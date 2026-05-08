package app

import (
	"encoding/json"
	"sort"
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

func TestRunPRPoetryOnlyPyProjectIsLocalFallbackTarget(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(map[string]string{"requests": "^2.31"}, nil),
		poetryPyProject(map[string]string{"requests": "^2.32"}, nil),
		poetryLock(map[string]string{"requests": "2.31.0"}, nil),
		poetryLock(map[string]string{"requests": "2.32.3"}, nil),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "root [root, ecosystem=poetry, manager=poetry]") {
		t.Fatalf("expected Poetry target, got %q", stdout)
	}
	if !strings.Contains(stdout, "lockfile: poetry.lock") {
		t.Fatalf("expected Poetry lockfile listing, got %q", stdout)
	}
	if strings.Contains(stdout, "fallback:") {
		t.Fatalf("did not expect Poetry target to remain API-only, got %q", stdout)
	}
}

func TestRunPRPoetryFallbackAddedUsesLockVersion(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		poetryPyProject(map[string]string{"requests": "^2.32"}, nil),
		"",
		poetryLock(map[string]string{"requests": "2.32.3"}, nil),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "requests")
	if change.ChangeType != analysis.ChangeAdded || change.ToRequirement != "^2.32" || change.ToVersion != "2.32.3" || change.Scope != analysis.ScopeRuntime {
		t.Fatalf("unexpected Poetry added change: %#v", change)
	}
}

func TestRunPRPoetryFallbackDeclarationOnlyWithoutLockfile(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		poetryPyProject(map[string]string{"requests": "^2.32"}, nil),
		"",
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "requests")
	if change.ChangeType != analysis.ChangeAdded || change.ToRequirement != "^2.32" || change.ToVersion != "" {
		t.Fatalf("unexpected Poetry declaration-only change: %#v", change)
	}
}

func TestRunPRPoetryFallbackUpdatedUsesLockVersion(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(map[string]string{"django": "^4.2"}, nil),
		poetryPyProject(map[string]string{"django": "^5.0"}, nil),
		poetryLock(map[string]string{"django": "4.2.11"}, nil),
		poetryLock(map[string]string{"django": "5.0.1"}, nil),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "django")
	if change.ChangeType != analysis.ChangeUpdated || change.FromVersion != "4.2.11" || change.ToVersion != "5.0.1" {
		t.Fatalf("unexpected Poetry updated change: %#v", change)
	}
	if !containsString(change.RiskDrivers, analysis.DriverMajorVersionBump) {
		t.Fatalf("expected major version driver, got %#v", change.RiskDrivers)
	}
}

func TestRunPRPoetryLockfileOnlyDirectResolvedVersionUpdate(t *testing.T) {
	client := newPoetryLockOnlyFakeGitHubClient(
		poetryPyProject(map[string]string{"requests": "^2.32"}, nil),
		poetryLock(map[string]string{"requests": "2.32.2"}, nil),
		poetryLock(map[string]string{"requests": "2.32.3"}, nil),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "requests")
	if change.ChangeType != analysis.ChangeUpdated || change.FromRequirement != "^2.32" || change.ToRequirement != "^2.32" || change.FromVersion != "2.32.2" || change.ToVersion != "2.32.3" {
		t.Fatalf("expected direct lockfile resolved version update, got %#v", change)
	}
}

func TestRunPRPoetryLockfileOnlyTransitiveChangeIgnored(t *testing.T) {
	client := newPoetryLockOnlyFakeGitHubClient(
		poetryPyProject(map[string]string{"requests": "^2.32"}, nil),
		poetryLock(map[string]string{"requests": "2.32.3", "urllib3": "2.2.1"}, nil),
		poetryLock(map[string]string{"requests": "2.32.3", "urllib3": "2.2.2"}, nil),
	)

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
}

func TestRunPRPoetryFallbackRemovedUsesLockVersion(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(map[string]string{"flask": "^3.0"}, nil),
		poetryPyProject(nil, nil),
		poetryLock(map[string]string{"flask": "3.0.3"}, nil),
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "flask")
	if change.ChangeType != analysis.ChangeRemoved || change.FromVersion != "3.0.3" || change.ToVersion != "" {
		t.Fatalf("unexpected Poetry removed change: %#v", change)
	}
}

func TestRunPRPoetryFallbackNonRegistrySourceUsesLockfileWhenPyProjectHasNoSource(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		poetryPyProject(map[string]string{"git-lib": "^0.1"}, nil),
		"",
		poetryLockWithSource("git-lib", "0.1.0", "git", "https://github.com/example/from-lock.git", "main"),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "git-lib")
	if change.Resolved != "git:https://github.com/example/from-lock.git#main" {
		t.Fatalf("expected lockfile source to enrich missing pyproject source, got %#v", change)
	}
	note := findNote(t, payload.Notes, analysis.NoteNonRegistrySource)
	if note.Dependency != "git-lib" || note.Detail != change.Resolved {
		t.Fatalf("expected non-registry note for lockfile source, got %#v", payload.Notes)
	}
	if !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected validate source recommendation, got %#v", payload.RecommendedActions)
	}
}

func TestRunPRPoetryFallbackNonRegistrySourceUsesPyProjectSourceFirst(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		poetryPyProject(nil, map[string]string{"git-lib": `{ git = "https://github.com/example/from-pyproject.git", branch = "main" }`}),
		"",
		poetryLockWithSource("git-lib", "0.1.0", "git", "https://github.com/example/from-lock.git", "main"),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "git-lib")
	if change.Resolved != "git:https://github.com/example/from-pyproject.git#branch=main" {
		t.Fatalf("expected pyproject source to win, got %#v", change)
	}
	note := findNote(t, payload.Notes, analysis.NoteNonRegistrySource)
	if note.Dependency != "git-lib" || note.Detail != change.Resolved {
		t.Fatalf("expected non-registry note for pyproject source, got %#v", payload.Notes)
	}
}

func TestRunPRPoetryUnsupportedOnlyKeepsExitCode2(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		"[tool.poetry]\nname = \"demo\"\nversion = \"0.1.0\"\n[tool.poetry.dependencies]\nbad = [\"not\", \"supported\"]\n",
		"",
		"",
	)

	_, stderr, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
	if !strings.Contains(stderr, "unsupported dependency entries were present") {
		t.Fatalf("expected unsupported warning on stderr, got %q", stderr)
	}
}

func TestRunPRPoetrySupportedAndUnsupportedIncludesNote(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		"[tool.poetry]\nname = \"demo\"\nversion = \"0.1.0\"\n[tool.poetry.dependencies]\nrequests = \"^2.32\"\nbad = [\"not\", \"supported\"]\n",
		"",
		poetryLock(map[string]string{"requests": "2.32.3"}, nil),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	_ = findChange(t, payload, "requests")
	if !hasNote(payload.Notes, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected unsupported note, got %#v", payload.Notes)
	}
}

func TestRunPRPoetryUsesDependencyReviewWhenAvailable(t *testing.T) {
	client := newPoetryFakeGitHubClient(
		poetryPyProject(nil, nil),
		poetryPyProject(map[string]string{"requests": "^2.32"}, nil),
		"",
		poetryLock(map[string]string{"requests": "2.32.3"}, nil),
	)
	client.compareErr = nil
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "requests", Manifest: "pyproject.toml", Ecosystem: "poetry", ChangeType: "added", Version: "2.32.3"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review primary path, got %#v", payload)
	}
	for _, key := range client.getFileKeys {
		if strings.Contains(key, "poetry.lock@") {
			t.Fatalf("expected dependency review path not to load poetry.lock, got file calls %#v", client.getFileKeys)
		}
	}
	_ = findChange(t, payload, "requests")
}

func TestRunPRPoetryNestedTargetAndPathFiltering(t *testing.T) {
	client := newNestedPoetryFakeGitHubClient()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"services/worker [standalone, ecosystem=poetry, manager=poetry]",
		"manifest: services/worker/pyproject.toml",
		"lockfile: services/worker/poetry.lock",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected nested Poetry target listing to contain %q, got %q", expected, stdout)
		}
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"services/worker/pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "services/worker/pyproject.toml" {
		t.Fatalf("expected nested Poetry target only, got %#v", payload.Targets)
	}
	_ = findChange(t, payload, "celery")
}

func TestRunPRMixedPEP621AndPoetryPrefersPEP621Target(t *testing.T) {
	client := newMixedPEP621PoetryFakeGitHubClient()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "manager=pyproject") || strings.Contains(stdout, "manager=poetry") || strings.Contains(stdout, "lockfile: poetry.lock") {
		t.Fatalf("expected mixed pyproject.toml to prefer PEP 621 target without Poetry ambiguity, got %q", stdout)
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	_ = findChange(t, payload, "requests")
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "pyproject.toml" || payload.Targets[0].Target.LockfilePath != "" {
		t.Fatalf("expected single pyproject manifest target without Poetry lockfile, got %#v", payload.Targets)
	}
	if !hasNote(payload.Notes, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected Poetry table unsupported note on PEP 621 path, got %#v", payload.Notes)
	}
	for _, change := range payload.Changes {
		if change.Name == "flask" {
			t.Fatalf("did not expect Poetry dependency to be analyzed through mixed PEP 621 target, got %#v", payload.Changes)
		}
	}
}

func TestRunPRMixedPEP621PoetryAndUvPrefersPyProjectWithUvLockfile(t *testing.T) {
	client := newMixedPEP621PoetryUvFakeGitHubClient()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "manager=pyproject") || strings.Contains(stdout, "manager=poetry") {
		t.Fatalf("expected mixed pyproject.toml to prefer PEP 621 target without Poetry ambiguity, got %q", stdout)
	}
	if !strings.Contains(stdout, "lockfile: uv.lock") || strings.Contains(stdout, "lockfile: poetry.lock") {
		t.Fatalf("expected mixed pyproject.toml to attach uv.lock only, got %q", stdout)
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "requests")
	if change.ToVersion != "2.32.3" {
		t.Fatalf("expected uv.lock to enrich mixed PEP 621 target, got %#v", change)
	}
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "pyproject.toml" || payload.Targets[0].Target.LockfilePath != "uv.lock" {
		t.Fatalf("expected single pyproject target with uv.lock, got %#v", payload.Targets)
	}
	if !hasNote(payload.Notes, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected Poetry table unsupported note on mixed PEP 621 path, got %#v", payload.Notes)
	}
	for _, change := range payload.Changes {
		if change.Name == "flask" {
			t.Fatalf("did not expect Poetry dependency to be analyzed through mixed PEP 621 target, got %#v", payload.Changes)
		}
	}
}

func TestRunPRUvPyProjectListTargetsShowsLockfile(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"requests>=2.32"}),
		"",
		uvLock(map[string]string{"requests": "2.32.3"}),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"root [root, ecosystem=pip, manager=pyproject]",
		"manifest: pyproject.toml",
		"lockfile: uv.lock",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected uv target listing to contain %q, got %q", expected, stdout)
		}
	}
	if strings.Contains(stdout, "manager=uv") {
		t.Fatalf("did not expect duplicate uv manager target, got %q", stdout)
	}
}

func TestRunPRUvUsesDependencyReviewWhenAvailable(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"requests>=2.32"}),
		"",
		uvLock(map[string]string{"requests": "2.32.3"}),
	)
	client.compareErr = nil
	client.compareChanges = []ghclient.DependencyReviewChange{
		{Name: "requests", Manifest: "pyproject.toml", Ecosystem: "pip", ChangeType: "added", Version: "2.32.3"},
	}

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if !payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review primary path, got %#v", payload)
	}
	for _, key := range client.getFileKeys {
		if strings.Contains(key, "uv.lock@") {
			t.Fatalf("expected dependency review path not to load uv.lock, got file calls %#v", client.getFileKeys)
		}
	}
	_ = findChange(t, payload, "requests")
}

func TestRunPRUvFallbackAddedUsesLockVersion(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"requests>=2.32"}),
		"",
		uvLock(map[string]string{"requests": "2.32.3"}),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "requests")
	if change.ChangeType != analysis.ChangeAdded || change.ToRequirement != ">=2.32" || change.ToVersion != "2.32.3" || change.Scope != analysis.ScopeRuntime {
		t.Fatalf("unexpected uv added change: %#v", change)
	}
}

func TestRunPRUvFallbackDeclarationOnlyWithoutLockfile(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"requests>=2.32"}),
		"",
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if payload.Targets[0].Target.LockfilePath != "" {
		t.Fatalf("did not expect uv.lock target without lockfile, got %#v", payload.Targets)
	}
	change := findChange(t, payload, "requests")
	if change.ChangeType != analysis.ChangeAdded || change.ToRequirement != ">=2.32" || change.ToVersion != "" {
		t.Fatalf("unexpected declaration-only pyproject change: %#v", change)
	}
}

func TestRunPRUvFallbackUpdatedUsesLockVersion(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{"django>=4.2"}),
		pep621PyProject([]string{"django>=5.0"}),
		uvLock(map[string]string{"django": "4.2.11"}),
		uvLock(map[string]string{"django": "5.0.1"}),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "django")
	if change.ChangeType != analysis.ChangeUpdated || change.FromVersion != "4.2.11" || change.ToVersion != "5.0.1" {
		t.Fatalf("unexpected uv updated change: %#v", change)
	}
	if !containsString(change.RiskDrivers, analysis.DriverMajorVersionBump) {
		t.Fatalf("expected major version driver, got %#v", change.RiskDrivers)
	}
}

func TestRunPRUvFallbackRemovedUsesLockVersion(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{"flask>=3.0"}),
		pep621PyProject([]string{}),
		uvLock(map[string]string{"flask": "3.0.3"}),
		"",
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "flask")
	if change.ChangeType != analysis.ChangeRemoved || change.FromVersion != "3.0.3" || change.ToVersion != "" {
		t.Fatalf("unexpected uv removed change: %#v", change)
	}
}

func TestRunPRUvLockfileOnlyDirectResolvedVersionUpdate(t *testing.T) {
	client := newUvLockOnlyFakeGitHubClient(
		pep621PyProject([]string{"requests>=2.32"}),
		uvLock(map[string]string{"requests": "2.32.2"}),
		uvLock(map[string]string{"requests": "2.32.3"}),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, decodeJSONReport(t, stdout), "requests")
	if change.ChangeType != analysis.ChangeUpdated || change.FromRequirement != ">=2.32" || change.ToRequirement != ">=2.32" || change.FromVersion != "2.32.2" || change.ToVersion != "2.32.3" {
		t.Fatalf("expected direct uv.lock resolved version update, got %#v", change)
	}
}

func TestRunPRUvLockfileOnlyTransitiveChangeIgnored(t *testing.T) {
	client := newUvLockOnlyFakeGitHubClient(
		pep621PyProject([]string{"requests>=2.32"}),
		uvLock(map[string]string{"requests": "2.32.3", "urllib3": "2.2.1"}),
		uvLock(map[string]string{"requests": "2.32.3", "urllib3": "2.2.2"}),
	)

	_, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
}

func TestRunPRUvLockfileOnlyDirectSourceUpdate(t *testing.T) {
	client := newUvLockOnlyFakeGitHubClient(
		pep621PyProject([]string{"git-lib>=0.1"}),
		uvLockWithSource("git-lib", "0.1.0", `{ git = "https://github.com/example/from-base.git", rev = "abc123" }`),
		uvLockWithSource("git-lib", "0.1.0", `{ git = "https://github.com/example/from-head.git", rev = "def456" }`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "git-lib")
	if change.ChangeType != analysis.ChangeUpdated || change.Resolved != "git:https://github.com/example/from-head.git#rev=def456" {
		t.Fatalf("expected direct uv.lock source update, got %#v", change)
	}
	if !hasNote(payload.Notes, analysis.NoteNonRegistrySource) {
		t.Fatalf("expected non-registry note for updated source, got %#v", payload.Notes)
	}
}

func TestRunPRUvFallbackNonRegistrySourceUsesLockfileWhenPyProjectHasNoSource(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"git-lib>=0.1"}),
		"",
		uvLockWithSource("git-lib", "0.1.0", `{ git = "https://github.com/example/from-lock.git", rev = "abc123" }`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "git-lib")
	if change.Resolved != "git:https://github.com/example/from-lock.git#rev=abc123" {
		t.Fatalf("expected uv.lock source to enrich missing pyproject source, got %#v", change)
	}
	note := findNote(t, payload.Notes, analysis.NoteNonRegistrySource)
	if note.Dependency != "git-lib" || note.Detail != change.Resolved {
		t.Fatalf("expected non-registry note for uv.lock source, got %#v", payload.Notes)
	}
	if !containsString(payload.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected validate source recommendation, got %#v", payload.RecommendedActions)
	}
}

func TestRunPRUvFallbackPyProjectSourceWinsOverLockfile(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"git-lib @ https://github.com/example/from-pyproject.git"}),
		"",
		uvLockWithSource("git-lib", "0.1.0", `{ git = "https://github.com/example/from-lock.git", rev = "abc123" }`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	change := findChange(t, payload, "git-lib")
	if change.Resolved != "https://github.com/example/from-pyproject.git" {
		t.Fatalf("expected pyproject source to win, got %#v", change)
	}
	note := findNote(t, payload.Notes, analysis.NoteNonRegistrySource)
	if note.Dependency != "git-lib" || note.Detail != change.Resolved {
		t.Fatalf("expected non-registry note for pyproject source, got %#v", payload.Notes)
	}
}

func TestRunPRUvRegistryVirtualWorkspaceSourcesDoNotCreateNonRegistryNote(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"requests>=2.32"}),
		"",
		uvLockWithSource("requests", "2.32.3", `{ registry = "https://pypi.org/simple" }`)+
			uvLockWithSource("project", "0.1.0", `{ virtual = "." }`)+
			uvLockWithSource("workspace-lib", "0.1.0", `{ workspace = true }`),
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	_ = findChange(t, payload, "requests")
	if hasNote(payload.Notes, analysis.NoteNonRegistrySource) {
		t.Fatalf("did not expect non-registry note for registry/virtual/workspace sources, got %#v", payload.Notes)
	}
}

func TestRunPRUvUnsupportedOnlyKeepsExitCode2(t *testing.T) {
	client := newUvLockOnlyFakeGitHubClient(
		pep621PyProject([]string{"requests>=2.32"}),
		uvLock(map[string]string{"requests": "2.32.3"}),
		uvLockWithSource("requests", "2.32.3", `{ unknown = "value" }`),
	)

	_, stderr, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	assertExitCode(t, err, 2)
	if !strings.Contains(stderr, "unsupported dependency entries were present") {
		t.Fatalf("expected unsupported warning on stderr, got %q", stderr)
	}
}

func TestRunPRUvSupportedAndUnsupportedIncludesNote(t *testing.T) {
	client := newUvPyProjectFakeGitHubClient(
		pep621PyProject([]string{}),
		pep621PyProject([]string{"requests>=2.32"}),
		"",
		uvLock(map[string]string{"requests": "2.32.3"})+`
[[package]]
version = "1.0.0"
`,
	)

	stdout, _, err := runPRWithClient(t, client, RunPROptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	_ = findChange(t, payload, "requests")
	if !hasNote(payload.Notes, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected unsupported note, got %#v", payload.Notes)
	}
}

func TestRunPRUvNestedTargetAndPathFiltering(t *testing.T) {
	client := newNestedUvFakeGitHubClient()

	stdout, _, err := runPRWithClient(t, client, RunPROptions{ListTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"services/api [standalone, ecosystem=pip, manager=pyproject]",
		"manifest: services/api/pyproject.toml",
		"lockfile: services/api/uv.lock",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected nested uv target listing to contain %q, got %q", expected, stdout)
		}
	}

	stdout, _, err = runPRWithClient(t, client, RunPROptions{Format: "json", Paths: []string{"services/api/pyproject.toml"}})
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeJSONReport(t, stdout)
	if len(payload.Targets) != 1 || payload.Targets[0].Target.ManifestPath != "services/api/pyproject.toml" || payload.Targets[0].Target.LockfilePath != "services/api/uv.lock" {
		t.Fatalf("expected nested uv target only, got %#v", payload.Targets)
	}
	_ = findChange(t, payload, "fastapi")
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

func newPoetryFakeGitHubClient(basePyProject, headPyProject, baseLock, headLock string) *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "pyproject.toml", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(basePyProject)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(headPyProject)
	if baseLock != "" || headLock != "" {
		client.files = append(client.files, ghclient.PullRequestFile{Filename: "poetry.lock", Status: "modified"})
		client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], "poetry.lock")
		client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], "poetry.lock")
		client.filesByKey[fileKey("poetry.lock", "base-sha")] = []byte(baseLock)
		client.filesByKey[fileKey("poetry.lock", "head-sha")] = []byte(headLock)
	}
	return client
}

func newPoetryLockOnlyFakeGitHubClient(pyProject, baseLock, headLock string) *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "poetry.lock", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml", "poetry.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml", "poetry.lock"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(pyProject)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(pyProject)
	client.filesByKey[fileKey("poetry.lock", "base-sha")] = []byte(baseLock)
	client.filesByKey[fileKey("poetry.lock", "head-sha")] = []byte(headLock)
	return client
}

func newNestedPoetryFakeGitHubClient() *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{
		{Filename: "services/worker/pyproject.toml", Status: "modified"},
		{Filename: "services/worker/poetry.lock", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"services/worker/pyproject.toml", "services/worker/poetry.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"services/worker/pyproject.toml", "services/worker/poetry.lock"}
	client.filesByKey[fileKey("services/worker/pyproject.toml", "base-sha")] = []byte(poetryPyProject(nil, nil))
	client.filesByKey[fileKey("services/worker/pyproject.toml", "head-sha")] = []byte(poetryPyProject(map[string]string{"celery": "^5.4"}, nil))
	client.filesByKey[fileKey("services/worker/poetry.lock", "base-sha")] = []byte("")
	client.filesByKey[fileKey("services/worker/poetry.lock", "head-sha")] = []byte(poetryLock(map[string]string{"celery": "5.4.0"}, nil))
	return client
}

func newMixedPEP621PoetryFakeGitHubClient() *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	base := `[project]
dependencies = []

[tool.poetry]
name = "demo"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
`
	head := `[project]
dependencies = ["requests==2.32.3"]

[tool.poetry]
name = "demo"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
flask = "^3.0"
`
	client.files = []ghclient.PullRequestFile{
		{Filename: "pyproject.toml", Status: "modified"},
		{Filename: "poetry.lock", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml", "poetry.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml", "poetry.lock"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(base)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(head)
	client.filesByKey[fileKey("poetry.lock", "base-sha")] = []byte("")
	client.filesByKey[fileKey("poetry.lock", "head-sha")] = []byte(poetryLock(map[string]string{"flask": "3.0.3"}, nil))
	return client
}

func newMixedPEP621PoetryUvFakeGitHubClient() *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	base := `[project]
dependencies = []

[tool.poetry]
name = "demo"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
`
	head := `[project]
dependencies = ["requests>=2.32"]

[tool.poetry]
name = "demo"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
flask = "^3.0"
`
	client.files = []ghclient.PullRequestFile{
		{Filename: "pyproject.toml", Status: "modified"},
		{Filename: "poetry.lock", Status: "modified"},
		{Filename: "uv.lock", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml", "poetry.lock", "uv.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml", "poetry.lock", "uv.lock"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(base)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(head)
	client.filesByKey[fileKey("poetry.lock", "base-sha")] = []byte("")
	client.filesByKey[fileKey("poetry.lock", "head-sha")] = []byte(poetryLock(map[string]string{"flask": "3.0.3"}, nil))
	client.filesByKey[fileKey("uv.lock", "base-sha")] = []byte("")
	client.filesByKey[fileKey("uv.lock", "head-sha")] = []byte(uvLock(map[string]string{"requests": "2.32.3"}))
	return client
}

func newUvPyProjectFakeGitHubClient(basePyProject, headPyProject, baseLock, headLock string) *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "pyproject.toml", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(basePyProject)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(headPyProject)
	if baseLock != "" || headLock != "" {
		client.files = append(client.files, ghclient.PullRequestFile{Filename: "uv.lock", Status: "modified"})
		if baseLock != "" {
			client.repositoryFilesByRef["base-sha"] = append(client.repositoryFilesByRef["base-sha"], "uv.lock")
			client.filesByKey[fileKey("uv.lock", "base-sha")] = []byte(baseLock)
		}
		if headLock != "" {
			client.repositoryFilesByRef["head-sha"] = append(client.repositoryFilesByRef["head-sha"], "uv.lock")
			client.filesByKey[fileKey("uv.lock", "head-sha")] = []byte(headLock)
		}
	}
	return client
}

func newUvLockOnlyFakeGitHubClient(pyProject, baseLock, headLock string) *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{{Filename: "uv.lock", Status: "modified"}}
	client.repositoryFilesByRef["base-sha"] = []string{"pyproject.toml", "uv.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"pyproject.toml", "uv.lock"}
	client.filesByKey[fileKey("pyproject.toml", "base-sha")] = []byte(pyProject)
	client.filesByKey[fileKey("pyproject.toml", "head-sha")] = []byte(pyProject)
	client.filesByKey[fileKey("uv.lock", "base-sha")] = []byte(baseLock)
	client.filesByKey[fileKey("uv.lock", "head-sha")] = []byte(headLock)
	return client
}

func newNestedUvFakeGitHubClient() *fakeGitHubClient {
	client := newPythonBaseFakeGitHubClient()
	client.files = []ghclient.PullRequestFile{
		{Filename: "services/api/pyproject.toml", Status: "modified"},
		{Filename: "services/api/uv.lock", Status: "modified"},
	}
	client.repositoryFilesByRef["base-sha"] = []string{"services/api/pyproject.toml", "services/api/uv.lock"}
	client.repositoryFilesByRef["head-sha"] = []string{"services/api/pyproject.toml", "services/api/uv.lock"}
	client.filesByKey[fileKey("services/api/pyproject.toml", "base-sha")] = []byte(pep621PyProject([]string{}))
	client.filesByKey[fileKey("services/api/pyproject.toml", "head-sha")] = []byte(pep621PyProject([]string{"fastapi>=0.115"}))
	client.filesByKey[fileKey("services/api/uv.lock", "base-sha")] = []byte("")
	client.filesByKey[fileKey("services/api/uv.lock", "head-sha")] = []byte(uvLock(map[string]string{"fastapi": "0.115.0"}))
	return client
}

func pep621PyProject(dependencies []string) string {
	var b strings.Builder
	b.WriteString("[project]\nname = \"demo\"\nversion = \"0.1.0\"\ndependencies = [")
	for index, dependency := range dependencies {
		if index > 0 {
			b.WriteString(", ")
		}
		encoded, _ := json.Marshal(dependency)
		b.Write(encoded)
	}
	b.WriteString("]\n")
	return b.String()
}

func uvLock(packages map[string]string) string {
	var b strings.Builder
	for _, name := range sortedMapKeys(packages) {
		b.WriteString("[[package]]\n")
		b.WriteString("name = \"")
		b.WriteString(name)
		b.WriteString("\"\nversion = \"")
		b.WriteString(packages[name])
		b.WriteString("\"\n\n")
	}
	return b.String()
}

func uvLockWithSource(name, version, source string) string {
	var b strings.Builder
	b.WriteString("[[package]]\n")
	b.WriteString("name = \"")
	b.WriteString(name)
	b.WriteString("\"\nversion = \"")
	b.WriteString(version)
	b.WriteString("\"\nsource = ")
	b.WriteString(source)
	b.WriteString("\n\n")
	return b.String()
}

func poetryPyProject(dependencies map[string]string, tableDependencies map[string]string) string {
	var b strings.Builder
	b.WriteString("[tool.poetry]\nname = \"demo\"\nversion = \"0.1.0\"\n\n[tool.poetry.dependencies]\npython = \"^3.12\"\n")
	names := sortedMapKeys(dependencies)
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(" = \"")
		b.WriteString(dependencies[name])
		b.WriteString("\"\n")
	}
	names = sortedMapKeys(tableDependencies)
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(" = ")
		b.WriteString(tableDependencies[name])
		b.WriteString("\n")
	}
	return b.String()
}

func poetryLock(packages map[string]string, sourcePackages map[string]string) string {
	var b strings.Builder
	for _, name := range sortedMapKeys(packages) {
		b.WriteString("[[package]]\n")
		b.WriteString("name = \"")
		b.WriteString(name)
		b.WriteString("\"\nversion = \"")
		b.WriteString(packages[name])
		b.WriteString("\"\ncategory = \"main\"\noptional = false\n\n")
	}
	for _, name := range sortedMapKeys(sourcePackages) {
		b.WriteString(poetryLockWithSource(name, sourcePackages[name], "git", "https://github.com/example/"+name+".git", "main"))
	}
	return b.String()
}

func poetryLockWithSource(name, version, sourceType, sourceURL, reference string) string {
	var b strings.Builder
	b.WriteString("[[package]]\n")
	b.WriteString("name = \"")
	b.WriteString(name)
	b.WriteString("\"\nversion = \"")
	b.WriteString(version)
	b.WriteString("\"\ngroups = [\"main\"]\noptional = false\n\n[package.source]\ntype = \"")
	b.WriteString(sourceType)
	b.WriteString("\"\nurl = \"")
	b.WriteString(sourceURL)
	b.WriteString("\"\nreference = \"")
	b.WriteString(reference)
	b.WriteString("\"\n\n")
	return b.String()
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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

func findNote(t *testing.T, notes []analysis.Note, code string) analysis.Note {
	t.Helper()
	for _, note := range notes {
		if note.Code == code {
			return note
		}
	}
	t.Fatalf("expected note %s, got %#v", code, notes)
	return analysis.Note{}
}
