package app

import (
	"encoding/json"
	"strings"
	"testing"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"github.com/cli/go-gh/v2/pkg/api"
)

func TestRunPRYarnBerryListTargets(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`),
	)
	client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], ".yarnrc.yml")
	client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], ".yarnrc.yml")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"root [root, ecosystem=yarn, manager=yarn-berry]",
		"manifest: package.json",
		"lockfile: yarn.lock",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected list-targets to contain %q, got %q", expected, stdout)
		}
	}
}

func TestRunPRYarnBerryFallbackAddedDependency(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review fallback, got %#v", payload)
	}
	change := findChange(t, payload, "left-pad")
	if change.ChangeType != analysis.ChangeAdded || change.ToVersion != "1.0.1" || change.Scope != analysis.ScopeRuntime || !change.Direct {
		t.Fatalf("unexpected Yarn Berry added dependency: %#v", change)
	}
	if !hasNote(payload.Notes, analysis.NoteYarnBerryLockfile) {
		t.Fatalf("expected Yarn Berry lockfile note, got %#v", payload.Notes)
	}
}

func TestRunPRYarnBerryFallbackRemovedDependency(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`),
		yarnBerryLock(``),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.ChangeType != analysis.ChangeRemoved || change.FromVersion != "1.0.1" || change.ToVersion != "" {
		t.Fatalf("unexpected Yarn Berry removed dependency: %#v", change)
	}
}

func TestRunPRYarnBerryFallbackUpdatedDeclaration(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^2.0.0"}, nil, nil),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`),
		yarnBerryLock(`
"left-pad@npm:^2.0.0":
  version: 2.0.0
  resolution: "left-pad@npm:2.0.0"
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.ChangeType != analysis.ChangeUpdated || change.FromRequirement != "^1.0.0" || change.ToRequirement != "^2.0.0" {
		t.Fatalf("unexpected Yarn Berry updated declaration: %#v", change)
	}
}

func TestRunPRYarnBerryLockfileOnlyDirectVersionUpdate(t *testing.T) {
	packageJSON := yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	client := newYarnBerryFakeGitHubClient(
		packageJSON,
		packageJSON,
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 2.0.0
  resolution: "left-pad@npm:2.0.0"
`),
	)
	client.files = []ghclient.PullRequestFile{{Filename: "yarn.lock", Status: "modified"}}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.ChangeType != analysis.ChangeUpdated || change.FromVersion != "1.0.1" || change.ToVersion != "2.0.0" {
		t.Fatalf("expected direct lockfile version update, got %#v", change)
	}
	if !containsString(change.RiskDrivers, analysis.DriverMajorVersionBump) {
		t.Fatalf("expected major version driver, got %#v", change.RiskDrivers)
	}
}

func TestRunPRYarnBerryTransitiveOnlyLockfileChangeExits2(t *testing.T) {
	packageJSON := yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	client := newYarnBerryFakeGitHubClient(
		packageJSON,
		packageJSON,
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"

"transitive@npm:^1.0.0":
  version: 1.0.0
  resolution: "transitive@npm:1.0.0"
`),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"

"transitive@npm:^1.0.0":
  version: 2.0.0
  resolution: "transitive@npm:2.0.0"
`),
	)
	client.files = []ghclient.PullRequestFile{{Filename: "yarn.lock", Status: "modified"}}

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
}

func TestRunPRYarnBerryChecksumOnlyChangeExits2(t *testing.T) {
	packageJSON := yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	client := newYarnBerryFakeGitHubClient(
		packageJSON,
		packageJSON,
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
  checksum: 10c0
`),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
  checksum: 20c0
`),
	)
	client.files = []ghclient.PullRequestFile{{Filename: "yarn.lock", Status: "modified"}}

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
}

func TestRunPRYarnBerryProtocolNotes(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{
			"workspace-lib": "workspace:*",
			"patched":       "patch:patched@npm%3A1.0.0#./patches/patched.patch",
			"git-lib":       "git:https://github.com/acme/git-lib.git#commit=abc",
		}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"workspace-lib@workspace:*":
  version: 0.0.0-use.local
  resolution: "workspace-lib@workspace:*"

"patched@patch:patched@npm%3A1.0.0#./patches/patched.patch":
  version: 1.0.0
  resolution: "patched@patch:patched@npm%3A1.0.0#./patches/patched.patch"

"git-lib@git:https://github.com/acme/git-lib.git#commit=abc":
  version: 1.0.0
  resolution: "git-lib@git:https://github.com/acme/git-lib.git#commit=abc"
`),
	)
	client.filesByKey[fileKey(".yarnrc.yml", "base-sha")] = []byte("nodeLinker: node-modules\n")
	client.filesByKey[fileKey(".yarnrc.yml", "head-sha")] = []byte("nodeLinker: pnp\n")
	client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], ".yarnrc.yml")
	client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], ".yarnrc.yml")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	for _, code := range []string{
		analysis.NoteYarnNodeLinker,
		analysis.NoteYarnWorkspaceProtocol,
		analysis.NoteYarnPatchProtocol,
		analysis.NoteYarnGitSource,
		analysis.NoteNonRegistrySource,
	} {
		if !hasNote(payload.Notes, code) {
			t.Fatalf("expected note %s, got %#v", code, payload.Notes)
		}
	}
	if !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected validate source action, got %#v", payload.RecommendedActions)
	}
}

func TestRunPRYarnBerryDependencyReviewAvailableDoesNotLoadFallbackFiles(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`),
	)
	client.compareErr = nil
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "yarn", ChangeType: "added", Version: "1.0.1"},
	}
	client.filesByKey[fileKey(".yarnrc.yml", "base-sha")] = []byte("nodeLinker: pnp\n")
	client.filesByKey[fileKey(".yarnrc.yml", "head-sha")] = []byte("nodeLinker: pnp\n")
	client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], ".yarnrc.yml")
	client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], ".yarnrc.yml")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review primary path, got %#v", payload)
	}
	for _, key := range client.getFileKeys {
		if strings.Contains(key, "yarn.lock@") || strings.Contains(key, ".yarnrc.yml@") {
			t.Fatalf("expected dependency review path not to load Yarn Berry fallback files, got keys %#v", client.getFileKeys)
		}
	}
}

func TestRunPRYarnBerryNestedPathFiltering(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(``),
	)
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "yarn.lock", "apps/web/package.json"}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = yarnBerryWorkspaceRootPackageJSON()
	client.filesByKey[fileKey("package.json", "head-sha")] = yarnBerryWorkspaceRootPackageJSON()
	client.filesByKey[fileKey("apps/web/package.json", "base-sha")] = yarnBerryPackageJSON("", nil, nil, nil)
	client.filesByKey[fileKey("apps/web/package.json", "head-sha")] = yarnBerryPackageJSON("", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	client.filesByKey[fileKey("yarn.lock", "base-sha")] = yarnBerryLock(``)
	client.filesByKey[fileKey("yarn.lock", "head-sha")] = yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "yarn.lock", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"apps/web"}})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.Target != "apps/web" || change.Manifest != "apps/web/package.json" {
		t.Fatalf("expected nested workspace target, got %#v", change)
	}
}

func newYarnBerryFakeGitHubClient(basePackageJSON, headPackageJSON, baseLock, headLock []byte) *fakeGitHubClient {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update Yarn Berry dependencies",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}
	client.files = []ghclient.PullRequestFile{
		{Filename: "package.json", Status: "modified"},
		{Filename: "yarn.lock", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "yarn.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"package.json", "yarn.lock"}
	client.filesByKey[fileKey("package.json", "base-sha")] = basePackageJSON
	client.filesByKey[fileKey("package.json", "head-sha")] = headPackageJSON
	client.filesByKey[fileKey("yarn.lock", "base-sha")] = baseLock
	client.filesByKey[fileKey("yarn.lock", "head-sha")] = headLock
	return client
}

func yarnBerryPackageJSON(packageManager string, dependencies, devDependencies, optionalDependencies map[string]string) []byte {
	value := map[string]any{
		"name":                 "demo",
		"dependencies":         dependencies,
		"devDependencies":      devDependencies,
		"optionalDependencies": optionalDependencies,
	}
	if packageManager != "" {
		value["packageManager"] = packageManager
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func yarnBerryWorkspaceRootPackageJSON() []byte {
	value := map[string]any{
		"name":           "root",
		"packageManager": "yarn@4.1.0",
		"workspaces":     []string{"apps/*"},
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func yarnBerryLock(entries string) []byte {
	return []byte("__metadata:\n  version: 8\n  cacheKey: 10\n" + entries)
}
