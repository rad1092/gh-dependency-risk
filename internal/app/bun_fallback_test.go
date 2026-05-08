package app

import (
	"encoding/json"
	"strings"
	"testing"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"gh-dep-risk/internal/npm"
	"github.com/cli/go-gh/v2/pkg/api"
)

func TestRunPRBunListTargets(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		bunLock(""),
		bunLock(`"left-pad": ["left-pad@1.0.1", "", {}, "sha512-left"]`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"root [root, ecosystem=npm, manager=bun]",
		"manifest: package.json",
		"lockfile: bun.lock",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected list-targets to contain %q, got %q", expected, stdout)
		}
	}
}

func TestRunPRBunFallbackAddedScopes(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0",
			map[string]string{"runtime-lib": "^1.0.0"},
			map[string]string{"dev-lib": "^2.0.0"},
			map[string]string{"optional-lib": "^3.0.0"},
		),
		bunLock(""),
		bunLock(`
    "runtime-lib": ["runtime-lib@1.0.1", ""],
    "dev-lib": ["dev-lib@2.0.1", ""],
    "optional-lib": ["optional-lib@3.0.1", ""]
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if payload.DependencyReviewAvailable {
		t.Fatalf("expected local fallback, got %#v", payload)
	}
	for _, tc := range []struct {
		name  string
		scope analysis.DependencyScope
	}{
		{"runtime-lib", analysis.ScopeRuntime},
		{"dev-lib", analysis.ScopeDev},
		{"optional-lib", analysis.ScopeOptional},
	} {
		change := findChange(t, payload, tc.name)
		if change.ChangeType != analysis.ChangeAdded || change.Scope != tc.scope || !change.Direct {
			t.Fatalf("unexpected added Bun dependency for %s: %#v", tc.name, change)
		}
	}
	if !hasNote(payload.Notes, analysis.NoteBunLockfile) {
		t.Fatalf("expected Bun lockfile note, got %#v", payload.Notes)
	}
}

func TestRunPRBunFallbackRemovedAndUpdated(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", map[string]string{"removed-lib": "^1.0.0", "updated-lib": "^1.0.0"}, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"updated-lib": "^2.0.0"}, nil, nil),
		bunLock(`
    "removed-lib": ["removed-lib@1.0.1", ""],
    "updated-lib": ["updated-lib@1.0.1", ""]
`),
		bunLock(`"updated-lib": ["updated-lib@2.0.0", ""]`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	removed := findChange(t, payload, "removed-lib")
	if removed.ChangeType != analysis.ChangeRemoved || removed.FromVersion != "1.0.1" {
		t.Fatalf("unexpected removed Bun dependency: %#v", removed)
	}
	updated := findChange(t, payload, "updated-lib")
	if updated.ChangeType != analysis.ChangeUpdated || updated.FromRequirement != "^1.0.0" || updated.ToRequirement != "^2.0.0" || updated.ToVersion != "2.0.0" {
		t.Fatalf("unexpected updated Bun dependency: %#v", updated)
	}
}

func TestRunPRBunLockfileOnlyDirectVersionAndSourceUpdate(t *testing.T) {
	packageJSON := bunPackageJSON("bun@1.2.0", map[string]string{
		"left-pad":   "^1.0.0",
		"source-lib": "^1.0.0",
	}, nil, nil)
	client := newBunFakeGitHubClient(
		packageJSON,
		packageJSON,
		bunLock(`
    "left-pad": ["left-pad@1.0.1", "", {}, "sha512-left-old"],
    "source-lib": ["source-lib@1.0.0", "https://example.com/source-lib-v1.tgz", {}, "sha512-source-old"]
`),
		bunLock(`
    "left-pad": ["left-pad@2.0.0", "", {}, "sha512-left-new"],
    "source-lib": ["source-lib@1.0.0", "https://example.com/source-lib-v2.tgz", {}, "sha512-source-new"]
`),
	)
	client.files = []ghclient.PullRequestFile{{Filename: "bun.lock", Status: "modified"}}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	version := findChange(t, payload, "left-pad")
	if version.ChangeType != analysis.ChangeUpdated || version.FromVersion != "1.0.1" || version.ToVersion != "2.0.0" {
		t.Fatalf("unexpected lockfile-only version update: %#v", version)
	}
	source := findChange(t, payload, "source-lib")
	if source.ChangeType != analysis.ChangeUpdated || !strings.Contains(source.Resolved, "source-lib-v2") {
		t.Fatalf("unexpected lockfile-only source update: %#v", source)
	}
	if !hasNote(payload.Notes, analysis.NoteNonRegistrySource) || !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected non-registry note/action, got notes=%#v actions=%#v", payload.Notes, payload.RecommendedActions)
	}
	if !hasNote(payload.Notes, analysis.NoteBunChecksumChanged) {
		t.Fatalf("expected checksum evidence note for direct lockfile update, got %#v", payload.Notes)
	}
}

func TestRunPRBunTransitiveAndChecksumOnlyExit2(t *testing.T) {
	packageJSON := bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	for name, headLock := range map[string][]byte{
		"transitive": bunLock(`
    "left-pad": ["left-pad@1.0.1", "", {}, "sha512-left"],
    "transitive-lib": ["transitive-lib@2.0.0", ""]
`),
		"checksum": bunLock(`"left-pad": ["left-pad@1.0.1", "", {}, "sha512-new"]`),
		"transitive-source": bunLock(`
    "left-pad": ["left-pad@1.0.1", "", {}, "sha512-left"],
    "transitive-source": ["transitive-source@https://example.com/transitive.tgz", "https://example.com/transitive.tgz"]
`),
	} {
		t.Run(name, func(t *testing.T) {
			client := newBunFakeGitHubClient(
				packageJSON,
				packageJSON,
				bunLock(`"left-pad": ["left-pad@1.0.1", "", {}, "sha512-left"]`),
				headLock,
			)
			client.files = []ghclient.PullRequestFile{{Filename: "bun.lock", Status: "modified"}}
			_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
			assertExitCode(t, err, 2)
		})
	}
}

func TestRunPRBunRegistryAndSourceProtocols(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{
			"registry-lib":  "^1.0.0",
			"alias-lib":     "npm:left-pad@^1.0.0",
			"workspace-lib": "workspace:*",
			"file-lib":      "file:../file-lib.tgz",
			"link-lib":      "link:../link-lib",
			"git-lib":       "git:https://github.com/acme/git-lib.git",
			"github-lib":    "github:acme/github-lib",
			"http-lib":      "https://example.com/http-lib.tgz",
		}, nil, nil),
		bunLock(""),
		bunLock(`
    "registry-lib": ["registry-lib@1.0.1", "https://registry.npmjs.org/registry-lib/-/registry-lib-1.0.1.tgz"],
    "alias-lib": ["alias-lib@npm:left-pad@^1.0.0", "https://registry.npmjs.org/left-pad/-/left-pad-1.0.1.tgz"],
    "workspace-lib": ["workspace-lib@workspace:*", "workspace:*"],
    "file-lib": ["file-lib@file:../file-lib.tgz", "file:../file-lib.tgz"],
    "link-lib": ["link-lib@link:../link-lib", "link:../link-lib"],
    "git-lib": ["git-lib@git:https://github.com/acme/git-lib.git", "git:https://github.com/acme/git-lib.git"],
    "github-lib": ["github-lib@github:acme/github-lib", "github:acme/github-lib"],
    "http-lib": ["http-lib@https://example.com/http-lib.tgz", "https://example.com/http-lib.tgz"]
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !hasNote(payload.Notes, analysis.NoteBunWorkspaceProtocol) {
		t.Fatalf("expected workspace protocol note, got %#v", payload.Notes)
	}
	if !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected source validation action, got %#v", payload.RecommendedActions)
	}
	for _, note := range payload.Notes {
		if note.Code != analysis.NoteNonRegistrySource {
			continue
		}
		if note.Dependency == "registry-lib" || note.Dependency == "alias-lib" {
			t.Fatalf("did not expect registry-like Bun dependency %s to emit non-registry source note: %#v", note.Dependency, payload.Notes)
		}
	}
	for _, dependency := range []string{"workspace-lib", "file-lib", "link-lib", "git-lib", "github-lib", "http-lib"} {
		found := false
		for _, note := range payload.Notes {
			if note.Code == analysis.NoteNonRegistrySource && note.Dependency == dependency {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected non-registry source note for %s, got %#v", dependency, payload.Notes)
		}
	}
}

func TestRunPRBunWorkspaceProtocolAndChecksumNoteAreGated(t *testing.T) {
	target := analysis.AnalysisTarget{
		DisplayName:     "root",
		ManifestPath:    "package.json",
		LockfilePath:    "bun.lock",
		PackageManager:  packageManagerBun,
		Ecosystem:       "npm",
		OwningDirectory: "",
		LocalFallback:   true,
	}
	manifest := mustParsePackageManifest(t, bunPackageJSON("bun@1.2.0", map[string]string{"workspace-lib": "workspace:*"}, nil, nil))
	baseLock := mustParseBunLock(t, bunLock(`"workspace-lib": ["workspace-lib@workspace:*", "", {}, "sha512-old"]`))
	headChecksumOnly := mustParseBunLock(t, bunLock(`"workspace-lib": ["workspace-lib@workspace:*", "", {}, "sha512-new"]`))
	input := buildBunLocalInput(target, manifest, manifest, baseLock, headChecksumOnly)
	if hasNote(input.Notes, analysis.NoteBunChecksumChanged) || hasNote(input.Notes, analysis.NoteBunWorkspaceProtocol) {
		t.Fatalf("did not expect Bun notes without changed direct dependency, got %#v", input.Notes)
	}

	headUpdated := mustParseBunLock(t, bunLock(`"workspace-lib": ["workspace-lib@workspace:^", "", {}, "sha512-new"]`))
	headManifest := mustParsePackageManifest(t, bunPackageJSON("bun@1.2.0", map[string]string{"workspace-lib": "workspace:^"}, nil, nil))
	input = buildBunLocalInput(target, manifest, headManifest, baseLock, headUpdated)
	if !hasNote(input.Notes, analysis.NoteBunChecksumChanged) || !hasNote(input.Notes, analysis.NoteBunWorkspaceProtocol) {
		t.Fatalf("expected Bun checksum/workspace notes for direct update, got %#v", input.Notes)
	}
}

func TestRunPRBunAmbiguousDirectLockfileMatchDoesNotGuess(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^3.0.0"}, nil, nil),
		bunLock(""),
		bunLock(`
    "left-pad@npm:^1.0.0": ["left-pad@1.0.1", ""],
    "left-pad@npm:^2.0.0": ["left-pad@2.0.0", ""]
`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "left-pad")
	if change.ToVersion != "" || change.Resolved != "" {
		t.Fatalf("expected ambiguous Bun match not to guess, got %#v", change)
	}
	note := findNote(t, payload.Notes, analysis.NoteUnsupportedDependency)
	if !strings.Contains(note.Detail, "ambiguous Bun lockfile entries for direct dependency") {
		t.Fatalf("expected ambiguous unsupported note, got %#v", note)
	}
}

func TestRunPRBunDependencyReviewAvailableDoesNotLoadFallbackLockfile(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		[]byte("not parsed"),
		[]byte("not parsed"),
	)
	client.compareErr = nil
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "1.0.1"},
	}

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
		t.Fatalf("expected Dependency Review npm change to be reported, got %#v", change)
	}
	for _, key := range client.getFileKeys {
		if strings.Contains(key, "bun.lock@") {
			t.Fatalf("expected dependency review path not to load Bun fallback lockfile, got keys %#v", client.getFileKeys)
		}
	}
}

func TestRunPRBunPathFilteringAndNestedStandalone(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunLock(""),
		bunLock(""),
	)
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "bun.lock", "services/api/package.json", "services/api/bun.lock"}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("services/api/package.json", "base-sha")] = bunPackageJSON("bun@1.2.0", nil, nil, nil)
	client.filesByKey[fileKey("services/api/package.json", "head-sha")] = bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	client.filesByKey[fileKey("services/api/bun.lock", "base-sha")] = bunLock("")
	client.filesByKey[fileKey("services/api/bun.lock", "head-sha")] = bunLock(`"left-pad": ["left-pad@1.0.1", ""]`)
	client.files = []ghclient.PullRequestFile{
		{Filename: "services/api/package.json", Status: "modified"},
		{Filename: "services/api/bun.lock", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"services/api"}})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.Target != "services/api" || change.Manifest != "services/api/package.json" {
		t.Fatalf("expected nested Bun target, got %#v", change)
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"services/api/package.json"}})
	if err != nil {
		t.Fatal(err)
	}
	change = findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.Target != "services/api" || change.Manifest != "services/api/package.json" {
		t.Fatalf("expected exact manifest path to select nested Bun target, got %#v", change)
	}
}

func TestRunPRBunWorkspaceUsesSharedRootLockfile(t *testing.T) {
	rootPackageJSON := bunWorkspaceRootPackageJSON()
	client := newBunFakeGitHubClient(
		rootPackageJSON,
		rootPackageJSON,
		bunLock(""),
		bunLock(`"left-pad": ["left-pad@1.0.1", ""]`),
	)
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "bun.lock", "apps/web/package.json"}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("apps/web/package.json", "base-sha")] = bunPackageJSON("", nil, nil, nil)
	client.filesByKey[fileKey("apps/web/package.json", "head-sha")] = bunPackageJSON("", map[string]string{"left-pad": "^1.0.0"}, nil, nil)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "bun.lock", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"apps/web/package.json"}})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.Target != "apps/web" || change.Manifest != "apps/web/package.json" || change.ToVersion != "1.0.1" {
		t.Fatalf("expected Bun workspace target with shared root lockfile, got %#v", change)
	}
}

func TestRunPRBunAmbiguousWithMultipleJSLockfiles(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		bunLock(""),
		bunLock(`"left-pad": ["left-pad@1.0.1", ""]`),
	)
	client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], "package-lock.json")
	client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], "package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "base-sha")] = readFixture(t, "base.package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "head-sha")] = readFixture(t, "head.package-lock.json")
	client.files = []ghclient.PullRequestFile{{Filename: "package.json", Status: "modified"}}

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 1)
}

func TestRunPRBunBinaryLockfileOnlyUnsupportedExit2(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^2.0.0"}, nil, nil),
		nil,
		nil,
	)
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "bun.lockb"}
	client.repositoryFilesByRef["head-sha"] = []string{"package.json", "bun.lockb"}
	client.files = []ghclient.PullRequestFile{{Filename: "bun.lockb", Status: "modified"}}

	_, stderr, err := runPRWithClient(t, client, RunPROptions{Format: "human"})
	assertExitCode(t, err, 2)
	if !strings.Contains(stderr, "unsupported dependency entries") {
		t.Fatalf("expected unsupported-only warning, got %q", stderr)
	}
	if strings.Contains(stderr, analysis.NoteBunBinaryLockfile) {
		t.Fatalf("expected unsupported-only warning not to expose raw Bun note code, got %q", stderr)
	}
}

func TestRunPRBunTextLockPreferredOverBinaryLockfile(t *testing.T) {
	client := newBunFakeGitHubClient(
		bunPackageJSON("bun@1.2.0", nil, nil, nil),
		bunPackageJSON("bun@1.2.0", map[string]string{"left-pad": "^1.0.0"}, nil, nil),
		bunLock(""),
		bunLock(`"left-pad": ["left-pad@1.0.1", ""]`),
	)
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "bun.lock", "bun.lockb"}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("bun.lockb", "base-sha")] = []byte("binary lockfile should not be read")
	client.filesByKey[fileKey("bun.lockb", "head-sha")] = []byte("binary lockfile should not be read")
	client.files = []ghclient.PullRequestFile{
		{Filename: "package.json", Status: "modified"},
		{Filename: "bun.lock", Status: "modified"},
		{Filename: "bun.lockb", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "left-pad")
	if change.ToVersion != "1.0.1" {
		t.Fatalf("expected text bun.lock to be used, got %#v", change)
	}
	for _, key := range client.getFileKeys {
		if strings.Contains(key, "bun.lockb@") {
			t.Fatalf("expected text bun.lock to win without reading bun.lockb, got file calls %#v", client.getFileKeys)
		}
	}
}

func newBunFakeGitHubClient(basePackageJSON, headPackageJSON, baseLock, headLock []byte) *fakeGitHubClient {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update Bun dependencies",
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
		{Filename: "bun.lock", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "bun.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"package.json", "bun.lock"}
	client.filesByKey[fileKey("package.json", "base-sha")] = basePackageJSON
	client.filesByKey[fileKey("package.json", "head-sha")] = headPackageJSON
	client.filesByKey[fileKey("bun.lock", "base-sha")] = baseLock
	client.filesByKey[fileKey("bun.lock", "head-sha")] = headLock
	return client
}

func bunPackageJSON(packageManager string, dependencies, devDependencies, optionalDependencies map[string]string) []byte {
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

func bunWorkspaceRootPackageJSON() []byte {
	value := map[string]any{
		"name":           "root",
		"packageManager": "bun@1.2.0",
		"workspaces":     []string{"apps/*"},
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func bunLock(packages string) []byte {
	if strings.TrimSpace(packages) == "" {
		packages = ""
	}
	return []byte("{\n  \"lockfileVersion\": 1,\n  \"packages\": {\n" + packages + "\n  }\n}\n")
}

func mustParseBunLock(t *testing.T, data []byte) npm.BunLockfile {
	t.Helper()
	lockfile, err := npm.ParseBunLockfile(data)
	if err != nil {
		t.Fatal(err)
	}
	return lockfile
}
