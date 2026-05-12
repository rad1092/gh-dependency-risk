package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/rad1092/gh-dependency-risk/internal/analysis"
	ghclient "github.com/rad1092/gh-dependency-risk/internal/github"
	"github.com/rad1092/gh-dependency-risk/internal/npm"
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

func TestRunPRYarnBerryNodeLinkerOnlyChangeExits2(t *testing.T) {
	packageJSON := yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	lockfile := yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`)
	client := newYarnBerryFakeGitHubClient(packageJSON, packageJSON, lockfile, lockfile)
	client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], ".yarnrc.yml")
	client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], ".yarnrc.yml")
	client.filesByKey[fileKey(".yarnrc.yml", "base-sha")] = []byte("nodeLinker: node-modules\n")
	client.filesByKey[fileKey(".yarnrc.yml", "head-sha")] = []byte("nodeLinker: pnp\n")
	client.files = []ghclient.PullRequestFile{{Filename: ".yarnrc.yml", Status: "modified"}}

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

func TestRunPRYarnBerryNPMAliasDoesNotEmitNonRegistrySource(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"alias-name": "npm:real-name@^1.0.0"}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"alias-name@npm:real-name@^1.0.0":
  version: 1.0.1
  resolution: "alias-name@npm:1.0.1"
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "alias-name")
	if change.Resolved != "" {
		t.Fatalf("expected npm alias to stay registry-like, got %#v", change)
	}
	if hasNote(payload.Notes, analysis.NoteNonRegistrySource) {
		t.Fatalf("did not expect non-registry note for npm alias, got %#v", payload.Notes)
	}
}

func TestRunPRYarnBerryHTTPSourceEmitsNonRegistrySource(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"http-lib": "https://example.com/http-lib.tgz"}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"http-lib@https://example.com/http-lib.tgz":
  version: 1.0.0
  resolution: "http-lib@https://example.com/http-lib.tgz"
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !hasNote(payload.Notes, analysis.NoteNonRegistrySource) {
		t.Fatalf("expected non-registry note for http source, got %#v", payload.Notes)
	}
	if !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected validate source action, got %#v", payload.RecommendedActions)
	}
}

func TestRunPRYarnBerryAmbiguousDirectLockfileMatchDoesNotGuess(t *testing.T) {
	client := newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^3.0.0"}, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"

"left-pad@npm:^2.0.0":
  version: 2.0.0
  resolution: "left-pad@npm:2.0.0"
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "left-pad")
	if change.ToVersion != "" || change.Resolved != "" {
		t.Fatalf("expected ambiguous lockfile match not to guess version/source, got %#v", change)
	}
	note := findNote(t, payload.Notes, analysis.NoteUnsupportedDependency)
	if !strings.Contains(note.Detail, "ambiguous Yarn Berry lockfile entries for direct dependency") {
		t.Fatalf("expected ambiguous unsupported note, got %#v", note)
	}
}

func TestRunPRYarnBerryTransitiveOnlyProtocolEntryExits2(t *testing.T) {
	packageJSON := yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil)
	client := newYarnBerryFakeGitHubClient(
		packageJSON,
		packageJSON,
		yarnBerryLock(``),
		yarnBerryLock(`
"workspace-lib@workspace:*":
  version: 0.0.0-use.local
  resolution: "workspace-lib@workspace:*"
`),
	)
	client.files = []ghclient.PullRequestFile{{Filename: "yarn.lock", Status: "modified"}}

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
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
	change := findChange(t, payload, "left-pad")
	if change.ChangeType != analysis.ChangeAdded || change.ToVersion != "1.0.1" {
		t.Fatalf("expected Dependency Review Yarn change to be reported, got %#v", change)
	}
	for _, key := range client.getFileKeys {
		if strings.Contains(key, "yarn.lock@") || strings.Contains(key, ".yarnrc.yml@") {
			t.Fatalf("expected dependency review path not to load Yarn Berry fallback files, got keys %#v", client.getFileKeys)
		}
	}
}

func TestYarnBerryLocalInputGatesChecksumAndNodeLinkerNotes(t *testing.T) {
	target := analysis.AnalysisTarget{
		DisplayName:     "root",
		ManifestPath:    "package.json",
		LockfilePath:    "yarn.lock",
		PackageManager:  packageManagerYarnBerry,
		Ecosystem:       "yarn",
		OwningDirectory: "",
		LocalFallback:   true,
	}
	manifest := mustParsePackageManifest(t, yarnBerryPackageJSON("yarn@4.1.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil))
	baseLock := mustParseYarnBerryLock(t, yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
  checksum: 10c0
`))
	headChecksumOnly := mustParseYarnBerryLock(t, yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
  checksum: 20c0
`))
	input := buildYarnBerryLocalInput(
		target,
		manifest,
		manifest,
		baseLock,
		headChecksumOnly,
		npm.YarnRC{NodeLinker: "node-modules"},
		npm.YarnRC{NodeLinker: "pnp"},
		".yarnrc.yml",
	)
	if hasNote(input.Notes, analysis.NoteYarnChecksumChanged) || hasNote(input.Notes, analysis.NoteYarnNodeLinker) {
		t.Fatalf("did not expect checksum/nodeLinker notes without changed direct dependency, got %#v", input.Notes)
	}

	headUpdated := mustParseYarnBerryLock(t, yarnBerryLock(`
"left-pad@npm:^1.0.0":
  version: 2.0.0
  resolution: "left-pad@npm:2.0.0"
  checksum: 20c0
`))
	input = buildYarnBerryLocalInput(
		target,
		manifest,
		manifest,
		baseLock,
		headUpdated,
		npm.YarnRC{NodeLinker: "node-modules"},
		npm.YarnRC{NodeLinker: "pnp"},
		".yarnrc.yml",
	)
	for _, code := range []string{analysis.NoteYarnChecksumChanged, analysis.NoteYarnNodeLinker} {
		if !hasNote(input.Notes, code) {
			t.Fatalf("expected note %s for direct dependency update, got %#v", code, input.Notes)
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

	client = newYarnBerryFakeGitHubClient(
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryPackageJSON("yarn@4.1.0", nil, nil, nil),
		yarnBerryLock(``),
		yarnBerryLock(``),
	)
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "yarn.lock", ".yarnrc.yml", "apps/web/package.json"}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = yarnBerryWorkspaceRootPackageJSON()
	client.filesByKey[fileKey("package.json", "head-sha")] = yarnBerryWorkspaceRootPackageJSON()
	client.filesByKey[fileKey(".yarnrc.yml", "base-sha")] = []byte("nodeLinker: node-modules\n")
	client.filesByKey[fileKey(".yarnrc.yml", "head-sha")] = []byte("nodeLinker: pnp\n")
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
		{Filename: ".yarnrc.yml", Status: "modified"},
	}
	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"apps/web/package.json"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change = findChange(t, payload, "left-pad")
	if change.Target != "apps/web" || change.Manifest != "apps/web/package.json" {
		t.Fatalf("expected exact nested manifest target, got %#v", change)
	}
	if !hasNote(payload.Notes, analysis.NoteYarnNodeLinker) {
		t.Fatalf("expected root .yarnrc.yml nodeLinker note for nested target, got %#v", payload.Notes)
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

func mustParsePackageManifest(t *testing.T, data []byte) *npm.PackageManifest {
	t.Helper()
	manifest, err := npm.ParsePackageManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func mustParseYarnBerryLock(t *testing.T, data []byte) npm.YarnBerryLockfile {
	t.Helper()
	lockfile, err := npm.ParseYarnBerryLockfile(data)
	if err != nil {
		t.Fatal(err)
	}
	return lockfile
}
