package app

import (
	"strings"
	"testing"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"github.com/cli/go-gh/v2/pkg/api"
)

func TestRunPRGoModTargetDiscoveryShowsLocalFallback(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		baseGoMod("example.com/app", "example.com/lib v1.0.0"),
		baseGoMod("example.com/app", "example.com/lib v1.1.0"),
		"example.com/lib v1.0.0 h1:base\n",
		"example.com/lib v1.1.0 h1:head\n",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"root [root, ecosystem=go-modules, manager=go]",
		"manifest: go.mod",
		"lockfile: go.sum",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected Go target listing to contain %q, got %q", expected, stdout)
		}
	}
	if strings.Contains(stdout, "fallback:") {
		t.Fatalf("did not expect Go modules target to be API-only, got %q", stdout)
	}
}

func TestRunPRGoModFallbackAddedDependency(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		baseGoMod("example.com/app", ""),
		baseGoMod("example.com/app", "golang.org/x/text v0.16.0"),
		"",
		"golang.org/x/text v0.16.0 h1:head\n",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review fallback, got %#v", payload)
	}
	change := findChange(t, payload, "golang.org/x/text")
	if change.ChangeType != analysis.ChangeAdded || change.ToVersion != "v0.16.0" || change.Scope != analysis.ScopeRuntime || !change.Direct {
		t.Fatalf("unexpected Go added dependency change: %#v", change)
	}
}

func TestRunPRGoModFallbackIndirectDependencyIsTransitive(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		baseGoMod("example.com/app", ""),
		baseGoMod("example.com/app", "golang.org/x/sys v0.31.0 // indirect"),
		"",
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "golang.org/x/sys")
	if change.Scope != analysis.ScopeTransitive || change.Direct || change.Score != 0 {
		t.Fatalf("expected indirect Go requirement to be transitive and unscored as direct, got %#v", change)
	}
}

func TestRunPRGoModFallbackDirectToIndirectUpdate(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		baseGoMod("example.com/app", "example.com/lib v1.0.0"),
		baseGoMod("example.com/app", "example.com/lib v1.0.0 // indirect"),
		"",
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "example.com/lib")
	if change.ChangeType != analysis.ChangeUpdated || change.Scope != analysis.ScopeTransitive || change.Direct || change.Score != 0 {
		t.Fatalf("expected direct-to-indirect update to be represented conservatively, got %#v", change)
	}
}

func TestRunPRGoModFallbackReplaceOnlyIsReported(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		"module example.com/app\ngo 1.22\n",
		"module example.com/app\ngo 1.22\nreplace example.com/local => ../local/path\n",
		"",
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "example.com/local")
	if change.ChangeType != analysis.ChangeAdded || change.Resolved != "local-replace:../local/path" {
		t.Fatalf("expected local replace-only change, got %#v", change)
	}
	for _, code := range []string{analysis.NoteGoReplaceDirective, analysis.NoteGoLocalReplace, analysis.NoteNonRegistrySource} {
		if !hasNote(payload.Notes, code) {
			t.Fatalf("expected note %s, got %#v", code, payload.Notes)
		}
	}
	if !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected validate source action, got %#v", payload.RecommendedActions)
	}
}

func TestRunPRGoModFallbackGoSumOnlyKeepsExitCode2(t *testing.T) {
	client := newGoSumOnlyFakeGitHubClient(
		baseGoMod("example.com/app", "example.com/lib v1.0.0"),
		"example.com/lib v1.0.0 h1:base\n",
		"example.com/lib v1.0.0 h1:head\n",
	)

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
}

func TestRunPRGoModDirectiveOnlyKeepsExitCode2(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		"module example.com/app\ngo 1.22\n",
		"module example.com/app\ngo 1.23\n",
		"",
		"",
	)

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
}

func TestRunPRGoModUsesDependencyReviewWhenAvailable(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		baseGoMod("example.com/app", ""),
		baseGoMod("example.com/app", "golang.org/x/text v0.16.0"),
		"",
		"golang.org/x/text v0.16.0 h1:head\n",
	)
	client.compareErr = nil
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "golang.org/x/text", Manifest: "go.mod", Ecosystem: "gomod", ChangeType: "added", Version: "0.16.0"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review primary path, got %#v", payload)
	}
	if client.getFileCalls != 0 {
		t.Fatalf("expected dependency review path not to load go.mod/go.sum, got file calls %#v", client.getFileKeys)
	}
	_ = findChange(t, payload, "golang.org/x/text")
}

func TestRunPRGoModNestedPathFiltering(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"services/api/go.mod",
		baseGoMod("example.com/api", ""),
		baseGoMod("example.com/api", "golang.org/x/text v0.16.0"),
		"",
		"golang.org/x/text v0.16.0 h1:head\n",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"services/api [standalone, ecosystem=go-modules, manager=go]",
		"manifest: services/api/go.mod",
		"lockfile: services/api/go.sum",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected nested Go target listing to contain %q, got %q", expected, stdout)
		}
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"services/api/go.mod"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "services/api/go.mod" {
		t.Fatalf("expected nested Go target only, got %#v", payload.Targets)
	}
	_ = findChange(t, payload, "golang.org/x/text")
}

func TestRunPRGoModMalformedReturnsClearError(t *testing.T) {
	client := newGoModFakeGitHubClient(
		"go.mod",
		"module example.com/app\ngo 1.22\n",
		"module example.com/app\nrequire (\nexample.com/lib v1.0.0\n",
		"",
		"",
	)

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "go.mod local fallback could not parse file safely") {
		t.Fatalf("expected clear malformed go.mod error, got %v", err)
	}
}

func TestRunPRGoSumUnsupportedOnlyKeepsExitCode2(t *testing.T) {
	client := newGoSumOnlyFakeGitHubClient(
		baseGoMod("example.com/app", "example.com/lib v1.0.0"),
		"example.com/lib v1.0.0 h1:base\n",
		"bad-checksum-line\n",
	)

	_, stderr, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
	if !strings.Contains(stderr, "unsupported dependency entries were present") {
		t.Fatalf("expected unsupported warning, got %q", stderr)
	}
}

func newGoModFakeGitHubClient(manifestPath, baseMod, headMod, baseSum, headSum string) *fakeGitHubClient {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update Go modules",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	sumPath := goSumPathForDir(manifestDir(manifestPath))
	client.files = []ghclient.PullRequestFile{{Filename: manifestPath, Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{manifestPath}
	client.repositoryFilesByRef["head-sha"] = []string{manifestPath}
	client.filesByKey[fileKey(manifestPath, "base-sha")] = []byte(baseMod)
	client.filesByKey[fileKey(manifestPath, "head-sha")] = []byte(headMod)
	if baseSum != "" || headSum != "" {
		client.files = append(client.files, ghclient.PullRequestFile{Filename: sumPath, Status: "modified"})
		client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], sumPath)
		client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], sumPath)
		client.filesByKey[fileKey(sumPath, "base-sha")] = []byte(baseSum)
		client.filesByKey[fileKey(sumPath, "head-sha")] = []byte(headSum)
	}
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}
	return client
}

func newGoSumOnlyFakeGitHubClient(goMod, baseSum, headSum string) *fakeGitHubClient {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update Go checksums",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{{Filename: "go.sum", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"go.mod", "go.sum"}
	client.repositoryFilesByRef["head-sha"] = []string{"go.mod", "go.sum"}
	client.filesByKey[fileKey("go.mod", "base-sha")] = []byte(goMod)
	client.filesByKey[fileKey("go.mod", "head-sha")] = []byte(goMod)
	client.filesByKey[fileKey("go.sum", "base-sha")] = []byte(baseSum)
	client.filesByKey[fileKey("go.sum", "head-sha")] = []byte(headSum)
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}
	return client
}

func baseGoMod(modulePath, requireLine string) string {
	if strings.TrimSpace(requireLine) == "" {
		return "module " + modulePath + "\n\ngo 1.22\n"
	}
	return "module " + modulePath + "\n\ngo 1.22\n\nrequire " + requireLine + "\n"
}
