package app

import (
	"encoding/json"
	"strings"
	"testing"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"gh-dep-risk/internal/render"
	"github.com/cli/go-gh/v2/pkg/api"
)

func TestRunPRPythonRequirementsFallbackAdded(t *testing.T) {
	client := newPythonRequirementsFakeGitHubClient("", "fastapi==0.115.0\n")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review fallback, got %#v", payload)
	}
	if !hasNote(payload.Notes, analysis.NoteDependencyReviewFallback) {
		t.Fatalf("expected dependency review fallback note, got %#v", payload.Notes)
	}
	if client.getFileCalls == 0 {
		t.Fatalf("expected Python local fallback to load manifest contents")
	}
	change := findChange(t, payload, "fastapi")
	if change.ChangeType != analysis.ChangeAdded || change.ToVersion != "0.115.0" || change.Scope != analysis.ScopeRuntime || !change.Direct {
		t.Fatalf("unexpected fastapi change: %#v", change)
	}
}

func TestRunPRPythonUsesDependencyReviewWhenAvailable(t *testing.T) {
	client := newPythonRequirementsFakeGitHubClient("", "requests==2.32.3\n")
	client.compareErr = nil
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "requests", Manifest: "requirements.txt", Ecosystem: "pip", ChangeType: "added", Version: "2.32.3"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review to stay primary, got %#v", payload)
	}
	if hasNote(payload.Notes, analysis.NoteDependencyReviewFallback) {
		t.Fatalf("did not expect fallback note, got %#v", payload.Notes)
	}
	if client.getFileCalls != 0 {
		t.Fatalf("expected dependency review primary path not to load Python manifests, got %d file calls", client.getFileCalls)
	}
	_ = findChange(t, payload, "requests")
}

func TestRunPRPythonRequirementsFallbackUpdated(t *testing.T) {
	client := newPythonRequirementsFakeGitHubClient("django==4.2.0\n", "django==5.0.0\n")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "django")
	if change.ChangeType != analysis.ChangeUpdated || change.FromVersion != "4.2.0" || change.ToVersion != "5.0.0" {
		t.Fatalf("unexpected django change: %#v", change)
	}
	if !containsString(change.RiskDrivers, analysis.DriverMajorVersionBump) {
		t.Fatalf("expected major version driver, got %#v", change.RiskDrivers)
	}
}

func TestRunPRPythonRequirementsFallbackRemoved(t *testing.T) {
	client := newPythonRequirementsFakeGitHubClient("flask==3.0.0\n", "")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "flask")
	if change.ChangeType != analysis.ChangeRemoved || change.FromVersion != "3.0.0" || change.ToVersion != "" {
		t.Fatalf("unexpected flask change: %#v", change)
	}
}

func TestRunPRPythonPyProjectFallbackAdded(t *testing.T) {
	client := newPythonPyProjectFakeGitHubClient("[project]\ndependencies = []\n", "[project]\ndependencies = [\"requests==2.32.3\"]\n")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "requests")
	if change.ChangeType != analysis.ChangeAdded || change.ToVersion != "2.32.3" {
		t.Fatalf("unexpected requests change: %#v", change)
	}
}

func TestRunPRPythonPyProjectOptionalFallbackAdded(t *testing.T) {
	client := newPythonPyProjectFakeGitHubClient("[project.optional-dependencies]\ndev = []\n", "[project.optional-dependencies]\ndev = [\"pytest>=8\"]\n")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "pytest")
	if change.ChangeType != analysis.ChangeAdded || change.Scope != analysis.ScopeOptional || change.ToRequirement != ">=8" {
		t.Fatalf("unexpected pytest change: %#v", change)
	}
}

func TestRunPRPythonUnsupportedOnlyKeepsExitCode2(t *testing.T) {
	client := newPythonRequirementsFakeGitHubClient("", "-r other.txt\n")

	_, stderr, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
	if !strings.Contains(stderr, "unsupported dependency entries were present") {
		t.Fatalf("expected unsupported warning on stderr, got %q", stderr)
	}
}

func TestRunPRPythonSupportedAndUnsupportedIncludesNote(t *testing.T) {
	client := newPythonRequirementsFakeGitHubClient("", "-r other.txt\nrequests==2.32.3\n")

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !hasNote(payload.Notes, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected unsupported note, got %#v", payload.Notes)
	}
	if payload.Score != findChange(t, payload, "requests").Score {
		t.Fatalf("expected unsupported entry not to affect aggregate score, got report score=%d", payload.Score)
	}
	_ = findChange(t, payload, "requests")
}

func TestRunPRPythonListTargetsAndPathFiltering(t *testing.T) {
	client := newPythonMixedFakeGitHubClient()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"manifest: requirements.txt", "manager=pip", "manifest: pyproject.toml", "manager=pyproject"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected list-targets to contain %q, got %q", expected, stdout)
		}
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"requirements.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "requirements.txt" {
		t.Fatalf("expected requirements target only, got %#v", payload.Targets)
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload = decodeJSONReport(t, stdout)
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "pyproject.toml" {
		t.Fatalf("expected pyproject target only, got %#v", payload.Targets)
	}
}

func TestRunPRPythonNestedTargetsAndPathFiltering(t *testing.T) {
	client := newPythonNestedFakeGitHubClient()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"services/api [standalone, ecosystem=pip, manager=pip]",
		"manifest: services/api/requirements.txt",
		"libs/pkg [standalone, ecosystem=pip, manager=pyproject]",
		"manifest: libs/pkg/pyproject.toml",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected nested target listing to contain %q, got %q", expected, stdout)
		}
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"libs/pkg/pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "libs/pkg/pyproject.toml" {
		t.Fatalf("expected nested pyproject target only, got %#v", payload.Targets)
	}
	_ = findChange(t, payload, "requests")
}

func TestRunPRPythonPoetryOnlyPyProjectIsNotDirectFallbackTarget(t *testing.T) {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "pyproject.toml", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml"}
	poetryOnly := []byte("[tool.poetry]\nname = \"demo\"\nversion = \"0.1.0\"\n[tool.poetry.dependencies]\npython = \"^3.12\"\nrequests = \"^2.32\"\n")
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = poetryOnly
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = poetryOnly

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "root [root, ecosystem=poetry, manager=poetry]") {
		t.Fatalf("expected Poetry API-only target, got %q", stdout)
	}
	if strings.Contains(stdout, "manager=pyproject") {
		t.Fatalf("did not expect Poetry-only pyproject.toml to be promoted to Python direct fallback, got %q", stdout)
	}
}

func newPythonRequirementsFakeGitHubClient(base, head string) *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "requirements.txt", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"requirements.txt"}
	client.repositoryFilesByRef["head-sha"] = []string{"requirements.txt"}
	client.filesByKey[fileKey("requirements.txt", "base-sha")] = []byte(base)
	client.filesByKey[fileKey("requirements.txt", "head-sha")] = []byte(head)
	return client
}

func newPythonPyProjectFakeGitHubClient(base, head string) *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "pyproject.toml", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(base)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(head)
	return client
}

func newPythonMixedFakeGitHubClient() *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{
		{Filename: "requirements.txt", Status: "modified"},
		{Filename: "pyproject.toml", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"requirements.txt", "pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"requirements.txt", "pyproject.toml"}
	client.filesByKey[fileKey("requirements.txt", "base-sha")] = []byte("")
	client.filesByKey[fileKey("requirements.txt", "head-sha")] = []byte("fastapi==0.115.0\n")
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte("[project]\ndependencies = []\n")
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte("[project]\ndependencies = [\"requests==2.32.3\"]\n")
	return client
}

func newPythonNestedFakeGitHubClient() *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{
		{Filename: "services/api/requirements.txt", Status: "modified"},
		{Filename: "libs/pkg/pyproject.toml", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"services/api/requirements.txt", "libs/pkg/pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"services/api/requirements.txt", "libs/pkg/pyproject.toml"}
	client.filesByKey[fileKey("services/api/requirements.txt", "base-sha")] = []byte("")
	client.filesByKey[fileKey("services/api/requirements.txt", "head-sha")] = []byte("fastapi==0.115.0\n")
	client.filesByKey[fileKey("libs/pkg/pyproject.toml", "base-sha")] = []byte("[project]\ndependencies = []\n")
	client.filesByKey[fileKey("libs/pkg/pyproject.toml", "head-sha")] = []byte("[project]\ndependencies = [\"requests==2.32.3\"]\n")
	return client
}

func newPythonBaseFakeGitHubClient() *fakeGitHubClient {
	client := newFakeGitHubClient()
	client.pr = ghclient.PullRequest{
		Title:       "Update Python dependencies",
		Draft:       false,
		Number:      123,
		BaseSHA:     "base-sha",
		HeadSHA:     "head-sha",
		URL:         "https://github.com/owner/repo/pull/123",
		AuthorLogin: "octocat",
	}
	client.compareErr = &api.HTTPError{StatusCode: 404, Message: "dependency review disabled"}
	return client
}

func decodeJSONReport(t *testing.T, stdout string) render.JSONReport {
	t.Helper()
	var payload render.JSONReport
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func findChange(t *testing.T, payload render.JSONReport, name string) analysis.DependencyChange {
	t.Helper()
	for _, change := range payload.Changes {
		if change.Name == name {
			return change
		}
	}
	t.Fatalf("expected change for %s, got %#v", name, payload.Changes)
	return analysis.DependencyChange{}
}

func hasNote(notes []analysis.Note, code string) bool {
	for _, note := range notes {
		if note.Code == code {
			return true
		}
	}
	return false
}
