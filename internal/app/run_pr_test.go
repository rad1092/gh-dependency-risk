package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"gh-dep-risk/internal/render"
	"github.com/cli/go-gh/v2/pkg/api"
)

func TestParsePRArg(t *testing.T) {
	t.Run("number", func(t *testing.T) {
		repo, number, repoFromArg, err := parsePRArg("123")
		if err != nil {
			t.Fatal(err)
		}
		if repo != (ghclient.Repo{}) {
			t.Fatalf("expected empty repo, got %#v", repo)
		}
		if number != 123 {
			t.Fatalf("expected PR 123, got %d", number)
		}
		if repoFromArg {
			t.Fatalf("expected repoFromArg=false")
		}
	})

	t.Run("url", func(t *testing.T) {
		repo, number, repoFromArg, err := parsePRArg("https://github.com/OWNER/REPO/pull/456")
		if err != nil {
			t.Fatal(err)
		}
		if number != 456 {
			t.Fatalf("expected PR 456, got %d", number)
		}
		if !repoFromArg {
			t.Fatalf("expected repoFromArg=true")
		}
		expected := ghclient.Repo{Host: "github.com", Owner: "OWNER", Name: "REPO"}
		if repo != expected {
			t.Fatalf("unexpected repo: %#v", repo)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, _, _, err := parsePRArg("github.com/OWNER/REPO/pull/123"); err == nil {
			t.Fatalf("expected invalid URL error")
		}
	})
}

func TestResolveTargetUsesCurrentBranchPRWhenArgMissing(t *testing.T) {
	client := newFakeGitHubClient()
	client.repo = testRepo()
	client.resolveCurrentPRNumber = 77

	repo, number, err := resolveTarget(context.Background(), client, RunPROptions{})
	if err != nil {
		t.Fatal(err)
	}
	if repo != client.repo {
		t.Fatalf("unexpected repo: %#v", repo)
	}
	if number != 77 {
		t.Fatalf("expected PR 77, got %d", number)
	}
	if client.resolveCurrentPRCalls != 1 {
		t.Fatalf("expected ResolveCurrentPR to be called once, got %d", client.resolveCurrentPRCalls)
	}
}

func TestResolveTargetCurrentBranchFailureIsActionable(t *testing.T) {
	client := newFakeGitHubClient()
	client.repo = testRepo()
	client.resolveCurrentPRErr = errors.New("no pull requests found for branch")

	_, _, err := resolveTarget(context.Background(), client, RunPROptions{})
	if err == nil {
		t.Fatalf("expected current-branch resolution error")
	}
	if !strings.Contains(err.Error(), "Pass a PR number, a full PR URL, or --repo OWNER/REPO explicitly") {
		t.Fatalf("expected actionable guidance, got %v", err)
	}
}

func TestRunPRExitCodeNoSupportedChange(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{{Filename: "README.md", Status: "modified"}}
	client.compareChanges = nil

	_, _, err := runPRWithClient(t, client, RunPROptions{})
	assertExitCode(t, err, 2)
}

func TestRunPRExitCodeFailLevel(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{FailLevel: analysis.RiskLevelHigh})
	assertExitCode(t, err, 3)
	if !strings.Contains(stdout, "owner/repo") {
		t.Fatalf("expected human output to include repo, got %q", stdout)
	}
}

func TestRunPRExitCodeAuth(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.getPullRequestErr = ghclient.AuthError{Op: "pull"}

	_, _, err := runPRWithClient(t, client, RunPROptions{})
	assertExitCode(t, err, 4)
}

func TestRunPRExitCodeGeneralError(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.getPullRequestErr = errors.New("boom")

	_, _, err := runPRWithClient(t, client, RunPROptions{})
	assertExitCode(t, err, 1)
}

func TestRunPRDependencyReviewFallback(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"dependency_review_available": false`) {
		t.Fatalf("expected fallback JSON output, got %q", stdout)
	}
}

func TestRunPRDependencyReviewUnexpectedError(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.compareErr = errors.New("dependency review transport error")

	_, _, err := runPRWithClient(t, client, RunPROptions{})
	assertExitCode(t, err, 1)
}

func TestRunPRCommentUpsertCreatesComment(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)

	_, _, err := runPRWithClient(t, client, RunPROptions{Comment: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.createdComments) != 1 {
		t.Fatalf("expected one created comment, got %d", len(client.createdComments))
	}
	if !strings.HasPrefix(client.createdComments[0], ghclient.MarkerComment) {
		t.Fatalf("expected marker comment body, got %q", client.createdComments[0])
	}
	if len(client.updatedComments) != 0 || len(client.deletedComments) != 0 {
		t.Fatalf("expected no update/delete on create path")
	}
}

func TestRunPRCommentUpsertUpdatesNewestAndDeletesOlderDuplicates(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	client.comments = []ghclient.IssueComment{
		{
			ID:        10,
			Body:      ghclient.MarkerComment + "\nold",
			UserLogin: "reviewer",
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        11,
			Body:      ghclient.MarkerComment + "\nforeign",
			UserLogin: "teammate",
			CreatedAt: now.Add(-90 * time.Minute),
		},
		{
			ID:        12,
			Body:      ghclient.MarkerComment + "\nnewest",
			UserLogin: "reviewer",
			CreatedAt: now.Add(-30 * time.Minute),
		},
	}

	_, stderr, err := runPRWithClient(t, client, RunPROptions{Comment: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.updatedComments[12]; !ok {
		t.Fatalf("expected newest own marker comment to be updated")
	}
	if _, ok := client.updatedComments[11]; ok {
		t.Fatalf("expected foreign marker comment to remain untouched")
	}
	if !reflect.DeepEqual(client.deletedComments, []int64{10}) {
		t.Fatalf("expected only older own duplicate to be deleted, got %v", client.deletedComments)
	}
	if !strings.Contains(stderr, "warning: found marker comment owned by teammate") {
		t.Fatalf("expected foreign marker warning, got %q", stderr)
	}
}

func TestRunPRWritesBundleFromSingleAnalysisPass(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	bundleDir := t.TempDir()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{
		Format:    "human",
		Lang:      "en",
		BundleDir: bundleDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "owner/repo") {
		t.Fatalf("expected stdout output, got %q", stdout)
	}
	for _, name := range []string{"dep-risk-human.txt", "dep-risk.json", "dep-risk.md", "metadata.json"} {
		if _, statErr := os.Stat(filepath.Join(bundleDir, name)); statErr != nil {
			t.Fatalf("expected bundle file %s: %v", name, statErr)
		}
	}
	if client.viewerLoginCalls != 0 || client.getPullRequestCalls != 1 || client.listPullRequestCalls != 1 || client.compareCalls != 1 || client.listRepositoryFileCalls != 2 {
		t.Fatalf("expected a single GitHub analysis pass, got viewer=%d pr=%d files=%d compare=%d trees=%d", client.viewerLoginCalls, client.getPullRequestCalls, client.listPullRequestCalls, client.compareCalls, client.listRepositoryFileCalls)
	}
	if client.getFileCalls != 4 {
		t.Fatalf("expected four manifest/lockfile fetches, got %d", client.getFileCalls)
	}
}

func TestRunPRWritesBundleBeforeFailLevelExit(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	bundleDir := t.TempDir()

	_, _, err := runPRWithClient(t, client, RunPROptions{
		Lang:      "en",
		BundleDir: bundleDir,
		FailLevel: analysis.RiskLevelHigh,
	})
	assertExitCode(t, err, 3)
	for _, name := range []string{"dep-risk-human.txt", "dep-risk.json", "dep-risk.md", "metadata.json"} {
		if _, statErr := os.Stat(filepath.Join(bundleDir, name)); statErr != nil {
			t.Fatalf("expected bundle file %s after fail-level exit: %v", name, statErr)
		}
	}
}

func TestRunPRListTargets(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Detected dependency targets:",
		"- root [root, ecosystem=npm, manager=npm]",
		"manifest: package.json",
		"- apps/web [workspace, ecosystem=npm, manager=npm]",
		"manifest: apps/web/package.json",
		"lockfile: package-lock.json",
		"- services/api [standalone, ecosystem=npm, manager=npm]",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected target listing to contain %q, got %q", expected, stdout)
		}
	}
}

func TestRunPRListTargetsSkipsAnalysisAndCommentPaths(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)
	registry := &fakeRegistryClient{}
	bundleDir := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunPR(context.Background(), RunPRDependencies{
		GitHub:   client,
		Registry: registry,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}, RunPROptions{
		ListTargets: true,
		Paths:       []string{"apps/web"},
		Comment:     true,
		BundleDir:   bundleDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "- apps/web [workspace, ecosystem=npm, manager=npm]") {
		t.Fatalf("expected filtered target output, got %q", stdout.String())
	}
	if client.listPullRequestCalls != 0 {
		t.Fatalf("expected no PR file listing, got %d", client.listPullRequestCalls)
	}
	if client.compareCalls != 0 {
		t.Fatalf("expected no dependency review calls, got %d", client.compareCalls)
	}
	if client.viewerLoginCalls != 0 {
		t.Fatalf("expected no viewer resolution, got %d", client.viewerLoginCalls)
	}
	if client.listCommentsCalls != 0 || client.createCommentCalls != 0 || client.updateCommentCalls != 0 || client.deleteCommentCalls != 0 {
		t.Fatalf("expected no comment upsert activity, got list=%d create=%d update=%d delete=%d", client.listCommentsCalls, client.createCommentCalls, client.updateCommentCalls, client.deleteCommentCalls)
	}
	if registry.calls != 0 {
		t.Fatalf("expected no registry lookups, got %d", registry.calls)
	}
	entries, readErr := os.ReadDir(bundleDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no bundle files, got %#v", entries)
	}
}

func TestRunPRPathFiltering(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{
		Format: "json",
		Paths:  []string{"apps/web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload render.JSONReport
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Targets) != 1 || payload.Targets[0].Target.DisplayName != "apps/web" {
		t.Fatalf("expected only apps/web target, got %#v", payload.Targets)
	}
	if client.compareCalls != 1 {
		t.Fatalf("expected a single unscoped dependency review call, got %d", client.compareCalls)
	}
}

func TestRunPRPathFilteringRejectsUnknownTarget(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)

	_, _, err := runPRWithClient(t, client, RunPROptions{Paths: []string{"apps/unknown"}})
	assertExitCode(t, err, 1)
	for _, expected := range []string{
		"unknown dependency target path",
		"Run --list-targets",
		"apps/web/package.json",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected helpful unknown target error containing %q, got %v", expected, err)
		}
	}
}

func TestRunPRListTargetsForPNPMIncludesManagerDistinction(t *testing.T) {
	client := newPNPMWorkspaceFakeGitHubClient(t)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Detected dependency targets:",
		"- root [root, ecosystem=pnpm, manager=pnpm]",
		"- apps/web [workspace, ecosystem=pnpm, manager=pnpm]",
		"- packages/ui [workspace, ecosystem=pnpm, manager=pnpm]",
		"- tools/cli [standalone, ecosystem=pnpm, manager=pnpm]",
		"lockfile: pnpm-lock.yaml",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected pnpm target listing to contain %q, got %q", expected, stdout)
		}
	}
}

func TestRunPRPathFilteringForPNPM(t *testing.T) {
	client := newPNPMWorkspaceFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "pnpm-lock.yaml", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{
		Format: "json",
		Paths:  []string{"apps/web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload render.JSONReport
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Targets) != 1 || payload.Targets[0].Target.DisplayName != "apps/web" {
		t.Fatalf("expected only pnpm apps/web target, got %#v", payload.Targets)
	}
	if !containsString(payload.QuickCommands, "cd apps/web && pnpm list --depth Infinity") {
		t.Fatalf("expected pnpm quick command, got %#v", payload.QuickCommands)
	}
}

func TestRunPRPNPMDependencyReviewFallback(t *testing.T) {
	client := newPNPMWorkspaceFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "pnpm-lock.yaml", Status: "modified"},
	}
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"apps/web"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"dependency_review_available": false`,
		`"quick_commands": [`,
		`"cd apps/web \u0026\u0026 pnpm why axios"`,
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected pnpm fallback output to contain %q, got %q", expected, stdout)
		}
	}
}

func TestRunPRSupportsMixedNPMPNPMTargets(t *testing.T) {
	client := newMixedManagerFakeGitHubClient(t)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	var payload render.JSONReport
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Targets) != 2 {
		t.Fatalf("expected mixed npm + pnpm targets, got %#v", payload.Targets)
	}
	if !containsString(payload.QuickCommands, "npm ls --all") || !containsString(payload.QuickCommands, "cd tools/cli && pnpm list --depth Infinity") {
		t.Fatalf("expected mixed-manager quick commands, got %#v", payload.QuickCommands)
	}
}

func TestRunPRSupportsMixedReviewOnlyAndJSDependencyReviewTargets(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
		{Filename: "backend/go.mod", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], "backend/go.mod")
	client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], "backend/go.mod")
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "removed", Version: "1.0.0"},
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "2.0.0"},
		{Name: "golang.org/x/text", Manifest: "backend/go.mod", Ecosystem: "gomod", ChangeType: "added", Version: "0.16.0"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "human"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"backend [standalone, ecosystem=go-modules",
		"left-pad",
		"golang.org/x/text",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected mixed-ecosystem human output to contain %q, got %q", expected, stdout)
		}
	}
	if !strings.Contains(stdout, "npm ls --all") {
		t.Fatalf("expected JS quick command to remain present, got %q", stdout)
	}
	if strings.Contains(stdout, "go list") {
		t.Fatalf("did not expect speculative non-JS quick commands, got %q", stdout)
	}
}

func TestRunPRListTargetsIncludesEcosystemAwareNonJSTargets(t *testing.T) {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "List mixed targets",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.repositoryFilesByRef["base-sha"] = []string{"backend/go.mod", "pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"backend/go.mod", "pyproject.toml"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte("[tool.poetry]\nname = \"demo\"\nversion = \"0.1.0\"\n")
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte("[tool.poetry]\nname = \"demo\"\nversion = \"0.1.0\"\n")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Detected dependency targets:",
		"backend [standalone, ecosystem=go-modules, manager=go]",
		"root [root, ecosystem=poetry, manager=poetry]",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected target listing to contain %q, got %q", expected, stdout)
		}
	}
}

func TestRunPRPathMatchesManifestPathAndDirectoryForNonJSTarget(t *testing.T) {
	for _, pathValue := range []string{"backend", "backend/go.mod"} {
		t.Run(pathValue, func(t *testing.T) {
			client := newFakeGitHubClient()
			client.pr = ghclient.PullRequest{
				Title:       "Update go module",
				Draft:       false,
				Number:      123,
				BaseSHA:     "base-sha",
				HeadSHA:     "head-sha",
				URL:         "https://github.com/owner/repo/pull/123",
				AuthorLogin: "octocat",
			}
			client.files = []ghclient.PullRequestFile{{Filename: "backend/go.mod", Status: "modified"}}
			client.repositoryFilesByRef["base-sha"] = []string{"backend/go.mod"}
			client.repositoryFilesByRef["head-sha"] = []string{"backend/go.mod"}
			client.compareChanges = []ghclient.DependencyReviewChange{
				{Name: "golang.org/x/text", Manifest: "backend/go.mod", Ecosystem: "gomod", ChangeType: "added", Version: "0.16.0"},
			}

			stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "human", Paths: []string{pathValue}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout, "golang.org/x/text") {
				t.Fatalf("expected non-JS target selection to succeed for %q, got %q", pathValue, stdout)
			}
		})
	}
}

func TestRunPRDependencyReviewUnavailableForAPIOnlyNonJSTargetIsActionable(t *testing.T) {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update Rust dependency",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{{Filename: "backend/Cargo.toml", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"backend/Cargo.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"backend/Cargo.toml"}
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}

	_, _, err := runPRWithClient(t, client, RunPROptions{Paths: []string{"backend"}})
	assertExitCode(t, err, 1)
	for _, expected := range []string{
		"dependency review is unavailable",
		"backend/Cargo.toml",
		"npm/pnpm/yarn",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected actionable non-JS fallback error containing %q, got %v", expected, err)
		}
	}
}

func TestRunPRYarnBerryMetadataOnlyUsesModernFallback(t *testing.T) {
	client := newYarnRootFakeGitHubClient(t)
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}
	client.filesByKey[fileKey("yarn.lock", "base-sha")] = readFixture(t, "yarn.unsupported.lock")
	client.filesByKey[fileKey("yarn.lock", "head-sha")] = readFixture(t, "yarn.unsupported.lock")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if payload.DependencyReviewAvailable {
		t.Fatalf("expected local fallback for Yarn Berry metadata lockfile, got %#v", payload)
	}
	if !hasNote(payload.Notes, analysis.NoteYarnBerryLockfile) {
		t.Fatalf("expected Yarn Berry fallback note, got %#v", payload.Notes)
	}
}

func TestRunPRAmbiguousDualLockfileRequiresSingleChangedLockfile(t *testing.T) {
	client := newAmbiguousDualLockfileFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{{Filename: "package.json", Status: "modified"}}

	_, _, err := runPRWithClient(t, client, RunPROptions{})
	assertExitCode(t, err, 1)
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous dual-lockfile error, got %v", err)
	}
}

func TestRunPRAmbiguousDualLockfilePrefersSingleChangedManager(t *testing.T) {
	client := newAmbiguousDualLockfileFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{{Filename: "package.json", Status: "modified"}, {Filename: "pnpm-lock.yaml", Status: "modified"}}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"pnpm why axios"`) {
		t.Fatalf("expected pnpm manager to be selected, got %q", stdout)
	}
}

func TestRunPRCommentAuthErrorIncludesCommentModeGuidance(t *testing.T) {
	client := newConfiguredFakeGitHubClient(t)
	client.viewerLoginErr = ghclient.AuthError{Op: "viewer"}

	_, _, err := runPRWithClient(t, client, RunPROptions{Comment: true})
	assertExitCode(t, err, 4)
	for _, expected := range []string{
		"comment mode requires permission",
		"cross-repo workflow comment limits",
		"owner/repo",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected comment auth guidance containing %q, got %v", expected, err)
		}
	}
}

func TestRunPRAggregatesMultipleTargets(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}

	var payload render.JSONReport
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Targets) != 2 {
		t.Fatalf("expected two analyzed targets, got %#v", payload.Targets)
	}
	if payload.Targets[0].Target.DisplayName != "apps/web" {
		t.Fatalf("expected riskiest target first, got %#v", payload.Targets)
	}
	if payload.DependencyReviewAvailable != true {
		t.Fatalf("expected dependency review to be available")
	}
	if payload.Score <= payload.Targets[0].Score {
		t.Fatalf("expected aggregate score bonus above max target score, got %#v", payload)
	}
}

func TestRunPRDedupesSharedWorkspaceTransitiveCount(t *testing.T) {
	client := newSharedTransitiveWorkspaceFakeGitHubClient(t)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}

	var payload render.JSONReport
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	expectedSummary := "2 newly added transitive dependencies were detected."
	foundSummary := false
	for _, item := range payload.Summary {
		if item == expectedSummary {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("expected aggregate summary %q, got %#v", expectedSummary, payload.Summary)
	}
	if len(payload.Targets) != 2 {
		t.Fatalf("expected two targets, got %#v", payload.Targets)
	}
	for _, target := range payload.Targets {
		if target.AddedTransitiveCount != 2 {
			t.Fatalf("expected target %s to retain per-target transitive count 2, got %d", target.Target.DisplayName, target.AddedTransitiveCount)
		}
	}
}

func TestRunPRDependencyReviewFallbackForSingleTargetMarksAggregate(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"packages/ui"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"dependency_review_available": false`) {
		t.Fatalf("expected aggregate fallback output, got %q", stdout)
	}
}

func TestRunPRBundleIncludesPerTargetFiles(t *testing.T) {
	client := newWorkspaceFakeGitHubClient(t)
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}
	bundleDir := t.TempDir()

	_, _, err := runPRWithClient(t, client, RunPROptions{BundleDir: bundleDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		filepath.Join(bundleDir, "targets", "apps-web", "dep-risk.json"),
		filepath.Join(bundleDir, "targets", "apps-web", "dep-risk.md"),
		filepath.Join(bundleDir, "targets", "packages-ui", "dep-risk.json"),
		filepath.Join(bundleDir, "targets", "packages-ui", "dep-risk.md"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected per-target bundle file %s: %v", name, err)
		}
	}
}

type fakeGitHubClient struct {
	repo                     ghclient.Repo
	viewerLogin              string
	viewerLoginErr           error
	resolveRepoErr           error
	resolveCurrentPRNumber   int
	resolveCurrentPRErr      error
	resolveCurrentPRCalls    int
	viewerLoginCalls         int
	pr                       ghclient.PullRequest
	getPullRequestErr        error
	getPullRequestCalls      int
	files                    []ghclient.PullRequestFile
	listPullRequestErr       error
	listPullRequestCalls     int
	repositoryFilesByRef     map[string][]string
	listRepositoryFileCalls  int
	compareChanges           []ghclient.DependencyReviewChange
	compareChangesByManifest map[string][]ghclient.DependencyReviewChange
	compareErr               error
	compareCalls             int
	compareManifestErr       map[string]error
	compareManifestCalls     map[string]int
	filesByKey               map[string][]byte
	getFileErr               map[string]error
	getFileCalls             int
	getFileKeys              []string
	comments                 []ghclient.IssueComment
	listCommentsErr          error
	listCommentsCalls        int
	createCommentErr         error
	createCommentCalls       int
	updateCommentErr         error
	updateCommentCalls       int
	deleteCommentErr         error
	deleteCommentCalls       int
	createdComments          []string
	updatedComments          map[int64]string
	deletedComments          []int64
}

func newFakeGitHubClient() *fakeGitHubClient {
	return &fakeGitHubClient{
		repo:                     testRepo(),
		viewerLogin:              "reviewer",
		resolveCurrentPRNumber:   123,
		updatedComments:          map[int64]string{},
		repositoryFilesByRef:     map[string][]string{},
		compareChangesByManifest: map[string][]ghclient.DependencyReviewChange{},
		compareManifestErr:       map[string]error{},
		compareManifestCalls:     map[string]int{},
		filesByKey:               map[string][]byte{},
		getFileErr:               map[string]error{},
	}
}

func newConfiguredFakeGitHubClient(t *testing.T) *fakeGitHubClient {
	t.Helper()
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update dependencies",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{
		{Filename: "package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "removed", Version: "1.0.0"},
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "2.0.0"},
		{Name: "chalk", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "5.0.0"},
	}
	client.filesByKey[fileKey("package.json", "base-sha")] = readFixture(t, "base.package.json")
	client.filesByKey[fileKey("package.json", "head-sha")] = readFixture(t, "head.package.json")
	client.filesByKey[fileKey("package-lock.json", "base-sha")] = readFixture(t, "base.package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "head-sha")] = readFixture(t, "head.package-lock.json")
	client.repositoryFilesByRef["base-sha"] = []string{"package.json", "package-lock.json"}
	client.repositoryFilesByRef["head-sha"] = []string{"package.json", "package-lock.json"}
	return client
}

func newWorkspaceFakeGitHubClient(t *testing.T) *fakeGitHubClient {
	t.Helper()
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update workspace dependencies",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{
		"package.json",
		"package-lock.json",
		"apps/web/package.json",
		"packages/ui/package.json",
		"services/api/package.json",
		"services/api/package-lock.json",
	}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = readFixture(t, "workspace.root.package.json")
	client.filesByKey[fileKey("package.json", "head-sha")] = readFixture(t, "workspace.root.package.json")
	client.filesByKey[fileKey("package-lock.json", "base-sha")] = readFixture(t, "workspace.root.base.package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "head-sha")] = readFixture(t, "workspace.root.head.package-lock.json")
	client.filesByKey[fileKey("apps/web/package.json", "base-sha")] = readFixture(t, "workspace.apps-web.base.package.json")
	client.filesByKey[fileKey("apps/web/package.json", "head-sha")] = readFixture(t, "workspace.apps-web.head.package.json")
	client.filesByKey[fileKey("packages/ui/package.json", "base-sha")] = readFixture(t, "workspace.packages-ui.base.package.json")
	client.filesByKey[fileKey("packages/ui/package.json", "head-sha")] = readFixture(t, "workspace.packages-ui.head.package.json")
	client.filesByKey[fileKey("services/api/package.json", "base-sha")] = readFixture(t, "standalone.service.base.package.json")
	client.filesByKey[fileKey("services/api/package.json", "head-sha")] = readFixture(t, "standalone.service.head.package.json")
	client.filesByKey[fileKey("services/api/package-lock.json", "base-sha")] = readFixture(t, "standalone.service.base.package-lock.json")
	client.filesByKey[fileKey("services/api/package-lock.json", "head-sha")] = readFixture(t, "standalone.service.head.package-lock.json")
	client.compareChangesByManifest["apps/web/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "axios", Manifest: "apps/web/package.json", Ecosystem: "npm", ChangeType: "added", Version: "1.7.0"},
	}
	client.compareChangesByManifest["packages/ui/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "tailwind-merge", Manifest: "packages/ui/package.json", Ecosystem: "npm", ChangeType: "added", Version: "2.3.0"},
	}
	client.compareChangesByManifest["services/api/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "lodash", Manifest: "services/api/package.json", Ecosystem: "npm", ChangeType: "removed", Version: "4.17.20"},
		{Name: "lodash", Manifest: "services/api/package.json", Ecosystem: "npm", ChangeType: "added", Version: "4.17.21"},
	}
	return client
}

func newSharedTransitiveWorkspaceFakeGitHubClient(t *testing.T) *fakeGitHubClient {
	t.Helper()
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Add shared workspace dependency",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{
		"package.json",
		"package-lock.json",
		"apps/web/package.json",
		"packages/ui/package.json",
	}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = readFixture(t, "workspace.shared.root.package.json")
	client.filesByKey[fileKey("package.json", "head-sha")] = readFixture(t, "workspace.shared.root.package.json")
	client.filesByKey[fileKey("package-lock.json", "base-sha")] = readFixture(t, "workspace.shared.root.base.package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "head-sha")] = readFixture(t, "workspace.shared.root.head.package-lock.json")
	client.filesByKey[fileKey("apps/web/package.json", "base-sha")] = readFixture(t, "workspace.shared.apps-web.base.package.json")
	client.filesByKey[fileKey("apps/web/package.json", "head-sha")] = readFixture(t, "workspace.shared.apps-web.head.package.json")
	client.filesByKey[fileKey("packages/ui/package.json", "base-sha")] = readFixture(t, "workspace.shared.packages-ui.base.package.json")
	client.filesByKey[fileKey("packages/ui/package.json", "head-sha")] = readFixture(t, "workspace.shared.packages-ui.head.package.json")
	client.compareChangesByManifest["apps/web/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "axios", Manifest: "apps/web/package.json", Ecosystem: "npm", ChangeType: "added", Version: "1.7.0"},
	}
	client.compareChangesByManifest["packages/ui/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "axios", Manifest: "packages/ui/package.json", Ecosystem: "npm", ChangeType: "added", Version: "1.7.0"},
	}
	return client
}

func newPNPMWorkspaceFakeGitHubClient(t *testing.T) *fakeGitHubClient {
	t.Helper()
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update pnpm workspace dependencies",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{
		{Filename: "apps/web/package.json", Status: "modified"},
		{Filename: "packages/ui/package.json", Status: "modified"},
		{Filename: "pnpm-lock.yaml", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{
		"package.json",
		"pnpm-workspace.yaml",
		"pnpm-lock.yaml",
		"apps/web/package.json",
		"packages/ui/package.json",
		"tools/cli/package.json",
		"tools/cli/pnpm-lock.yaml",
	}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = readFixture(t, "pnpm.workspace.root.package.json")
	client.filesByKey[fileKey("package.json", "head-sha")] = readFixture(t, "pnpm.workspace.root.package.json")
	client.filesByKey[fileKey("pnpm-workspace.yaml", "base-sha")] = readFixture(t, "pnpm-workspace.yaml")
	client.filesByKey[fileKey("pnpm-workspace.yaml", "head-sha")] = readFixture(t, "pnpm-workspace.yaml")
	client.filesByKey[fileKey("pnpm-lock.yaml", "base-sha")] = readFixture(t, "pnpm.workspace.base.lock.yaml")
	client.filesByKey[fileKey("pnpm-lock.yaml", "head-sha")] = readFixture(t, "pnpm.workspace.head.lock.yaml")
	client.filesByKey[fileKey("apps/web/package.json", "base-sha")] = readFixture(t, "pnpm.workspace.apps-web.base.package.json")
	client.filesByKey[fileKey("apps/web/package.json", "head-sha")] = readFixture(t, "pnpm.workspace.apps-web.head.package.json")
	client.filesByKey[fileKey("packages/ui/package.json", "base-sha")] = readFixture(t, "pnpm.workspace.packages-ui.base.package.json")
	client.filesByKey[fileKey("packages/ui/package.json", "head-sha")] = readFixture(t, "pnpm.workspace.packages-ui.head.package.json")
	client.filesByKey[fileKey("tools/cli/package.json", "base-sha")] = readFixture(t, "pnpm.standalone.base.package.json")
	client.filesByKey[fileKey("tools/cli/package.json", "head-sha")] = readFixture(t, "pnpm.standalone.head.package.json")
	client.filesByKey[fileKey("tools/cli/pnpm-lock.yaml", "base-sha")] = readFixture(t, "pnpm.standalone.base.lock.yaml")
	client.filesByKey[fileKey("tools/cli/pnpm-lock.yaml", "head-sha")] = readFixture(t, "pnpm.standalone.head.lock.yaml")
	client.compareChangesByManifest["apps/web/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "axios", Manifest: "apps/web/package.json", Ecosystem: "pnpm", ChangeType: "added", Version: "1.7.0"},
	}
	client.compareChangesByManifest["packages/ui/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "tailwind-merge", Manifest: "packages/ui/package.json", Ecosystem: "pnpm", ChangeType: "added", Version: "2.3.0"},
	}
	client.compareChangesByManifest["tools/cli/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "commander", Manifest: "tools/cli/package.json", Ecosystem: "pnpm", ChangeType: "added", Version: "12.1.0"},
	}
	return client
}

func newMixedManagerFakeGitHubClient(t *testing.T) *fakeGitHubClient {
	t.Helper()
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update mixed package managers",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{
		{Filename: "package.json", Status: "modified"},
		{Filename: "package-lock.json", Status: "modified"},
		{Filename: "tools/cli/package.json", Status: "modified"},
		{Filename: "tools/cli/pnpm-lock.yaml", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{
		"package.json",
		"package-lock.json",
		"tools/cli/package.json",
		"tools/cli/pnpm-lock.yaml",
	}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = readFixture(t, "base.package.json")
	client.filesByKey[fileKey("package.json", "head-sha")] = readFixture(t, "head.package.json")
	client.filesByKey[fileKey("package-lock.json", "base-sha")] = readFixture(t, "base.package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "head-sha")] = readFixture(t, "head.package-lock.json")
	client.filesByKey[fileKey("tools/cli/package.json", "base-sha")] = readFixture(t, "pnpm.standalone.base.package.json")
	client.filesByKey[fileKey("tools/cli/package.json", "head-sha")] = readFixture(t, "pnpm.standalone.head.package.json")
	client.filesByKey[fileKey("tools/cli/pnpm-lock.yaml", "base-sha")] = readFixture(t, "pnpm.standalone.base.lock.yaml")
	client.filesByKey[fileKey("tools/cli/pnpm-lock.yaml", "head-sha")] = readFixture(t, "pnpm.standalone.head.lock.yaml")
	client.compareChangesByManifest["package.json"] = []ghclient.DependencyReviewChange{
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "removed", Version: "1.0.0"},
		{Name: "left-pad", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "2.0.0"},
	}
	client.compareChangesByManifest["tools/cli/package.json"] = []ghclient.DependencyReviewChange{
		{Name: "commander", Manifest: "tools/cli/package.json", Ecosystem: "pnpm", ChangeType: "added", Version: "12.1.0"},
	}
	return client
}

func newAmbiguousDualLockfileFakeGitHubClient(t *testing.T) *fakeGitHubClient {
	t.Helper()
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Ambiguous root lockfiles",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.files = []ghclient.PullRequestFile{
		{Filename: "package.json", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{
		"package.json",
		"package-lock.json",
		"pnpm-lock.yaml",
	}
	client.repositoryFilesByRef["head-sha"] = append([]string(nil), client.repositoryFilesByRef["base-sha"]...)
	client.filesByKey[fileKey("package.json", "base-sha")] = readFixture(t, "pnpm.root.package.json")
	client.filesByKey[fileKey("package.json", "head-sha")] = readFixture(t, "pnpm.root.package.json")
	client.filesByKey[fileKey("package-lock.json", "base-sha")] = readFixture(t, "base.package-lock.json")
	client.filesByKey[fileKey("package-lock.json", "head-sha")] = readFixture(t, "head.package-lock.json")
	client.filesByKey[fileKey("pnpm-lock.yaml", "base-sha")] = readFixture(t, "pnpm.root.base.lock.yaml")
	client.filesByKey[fileKey("pnpm-lock.yaml", "head-sha")] = readFixture(t, "pnpm.root.head.lock.yaml")
	client.compareChangesByManifest["package.json"] = []ghclient.DependencyReviewChange{
		{Name: "axios", Manifest: "package.json", Ecosystem: "pnpm", ChangeType: "added", Version: "1.7.0"},
	}
	return client
}

func (f *fakeGitHubClient) ResolveRepo(_ context.Context, override string) (ghclient.Repo, error) {
	if f.resolveRepoErr != nil {
		return ghclient.Repo{}, f.resolveRepoErr
	}
	if override != "" {
		parts := strings.Split(override, "/")
		if len(parts) == 2 {
			return ghclient.Repo{Host: "github.com", Owner: parts[0], Name: parts[1]}, nil
		}
	}
	return f.repo, nil
}

func (f *fakeGitHubClient) ViewerLogin(context.Context, ghclient.Repo) (string, error) {
	f.viewerLoginCalls++
	return f.viewerLogin, f.viewerLoginErr
}

func (f *fakeGitHubClient) ResolveCurrentPR(context.Context, ghclient.Repo) (int, error) {
	f.resolveCurrentPRCalls++
	return f.resolveCurrentPRNumber, f.resolveCurrentPRErr
}

func (f *fakeGitHubClient) GetPullRequest(context.Context, ghclient.Repo, int) (ghclient.PullRequest, error) {
	f.getPullRequestCalls++
	return f.pr, f.getPullRequestErr
}

func (f *fakeGitHubClient) ListPullRequestFiles(context.Context, ghclient.Repo, int) ([]ghclient.PullRequestFile, error) {
	f.listPullRequestCalls++
	return append([]ghclient.PullRequestFile(nil), f.files...), f.listPullRequestErr
}

func (f *fakeGitHubClient) CompareDependencies(context.Context, ghclient.Repo, string, string) ([]ghclient.DependencyReviewChange, error) {
	f.compareCalls++
	return append([]ghclient.DependencyReviewChange(nil), f.compareChanges...), f.compareErr
}

func (f *fakeGitHubClient) CompareDependenciesForManifest(_ context.Context, _ ghclient.Repo, _, _ string, manifestPath string) ([]ghclient.DependencyReviewChange, error) {
	f.compareCalls++
	f.compareManifestCalls[manifestPath]++
	if err, ok := f.compareManifestErr[manifestPath]; ok {
		return nil, err
	}
	if changes, ok := f.compareChangesByManifest[manifestPath]; ok {
		return append([]ghclient.DependencyReviewChange(nil), changes...), nil
	}
	return append([]ghclient.DependencyReviewChange(nil), f.compareChanges...), f.compareErr
}

func (f *fakeGitHubClient) ListRepositoryFiles(_ context.Context, _ ghclient.Repo, ref string) ([]string, error) {
	f.listRepositoryFileCalls++
	return append([]string(nil), f.repositoryFilesByRef[ref]...), nil
}

func (f *fakeGitHubClient) GetRepositoryFile(_ context.Context, _ ghclient.Repo, path, ref string) ([]byte, error) {
	f.getFileCalls++
	f.getFileKeys = append(f.getFileKeys, fileKey(path, ref))
	if err, ok := f.getFileErr[fileKey(path, ref)]; ok {
		return nil, err
	}
	data, ok := f.filesByKey[fileKey(path, ref)]
	if !ok {
		return nil, ghclient.ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (f *fakeGitHubClient) ListIssueComments(context.Context, ghclient.Repo, int) ([]ghclient.IssueComment, error) {
	f.listCommentsCalls++
	if f.listCommentsErr != nil {
		return nil, f.listCommentsErr
	}
	return append([]ghclient.IssueComment(nil), f.comments...), nil
}

func (f *fakeGitHubClient) CreateIssueComment(_ context.Context, _ ghclient.Repo, _ int, body string) (ghclient.IssueComment, error) {
	f.createCommentCalls++
	if f.createCommentErr != nil {
		return ghclient.IssueComment{}, f.createCommentErr
	}
	f.createdComments = append(f.createdComments, body)
	return ghclient.IssueComment{ID: int64(100 + len(f.createdComments)), Body: body, UserLogin: f.viewerLogin}, nil
}

func (f *fakeGitHubClient) UpdateIssueComment(_ context.Context, _ ghclient.Repo, commentID int64, body string) error {
	f.updateCommentCalls++
	if f.updateCommentErr != nil {
		return f.updateCommentErr
	}
	f.updatedComments[commentID] = body
	return nil
}

func (f *fakeGitHubClient) DeleteIssueComment(_ context.Context, _ ghclient.Repo, commentID int64) error {
	f.deleteCommentCalls++
	if f.deleteCommentErr != nil {
		return f.deleteCommentErr
	}
	f.deletedComments = append(f.deletedComments, commentID)
	return nil
}

func runPRWithClient(t *testing.T, client *fakeGitHubClient, opts RunPROptions) (string, string, error) {
	t.Helper()
	if opts.Format == "" {
		opts.Format = "human"
	}
	if opts.Lang == "" {
		opts.Lang = "en"
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunPR(context.Background(), RunPRDependencies{
		GitHub: client,
		Stdout: &stdout,
		Stderr: &stderr,
	}, opts)
	return stdout.String(), stderr.String(), err
}

func assertExitCode(t *testing.T, err error, expected int) {
	t.Helper()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.Code != expected {
		t.Fatalf("expected exit code %d, got %d", expected, exitErr.Code)
	}
}

func testRepo() ghclient.Repo {
	return ghclient.Repo{Host: "github.com", Owner: "owner", Name: "repo"}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fileKey(path, ref string) string {
	return path + "@" + ref
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type fakeRegistryClient struct {
	calls int
}

func (f *fakeRegistryClient) PublishedAt(context.Context, string, string) (time.Time, error) {
	f.calls++
	return time.Time{}, errors.New("unexpected registry lookup")
}
