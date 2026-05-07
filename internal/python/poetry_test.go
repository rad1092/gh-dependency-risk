package python

import (
	"strings"
	"testing"
)

func TestParsePoetryPyProjectStringDependency(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry]
name = "demo"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
Requests = "^2.32"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 {
		t.Fatalf("expected one dependency with python ignored, got %#v", result.Dependencies)
	}
	dependency := result.Dependencies[0]
	if dependency.Name != "requests" || dependency.Requirement != "^2.32" || dependency.Scope != ScopeRuntime {
		t.Fatalf("unexpected Poetry dependency: %#v", dependency)
	}
}

func TestParsePoetryPyProjectTableDependency(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry.dependencies]
rich = { version = "13.7.1", extras = ["jupyter"], optional = true, markers = "python_version >= '3.10'" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 {
		t.Fatalf("expected one table dependency, got %#v", result.Dependencies)
	}
	dependency := result.Dependencies[0]
	if dependency.Name != "rich" || dependency.Version != "13.7.1" || dependency.Scope != ScopeOptional {
		t.Fatalf("expected optional table dependency with version, got %#v", dependency)
	}
	for _, expected := range []string{"13.7.1", "extras=jupyter", "markers=python_version >= '3.10'", "optional=true"} {
		if !strings.Contains(dependency.Requirement, expected) {
			t.Fatalf("expected requirement to preserve %q, got %#v", expected, dependency)
		}
	}
}

func TestParsePoetryPyProjectSourceDependencies(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry.dependencies]
git-lib = { git = "https://github.com/example/git-lib.git", branch = "main" }
path-lib = { path = "../path-lib", develop = true }
url-lib = { url = "https://example.com/url-lib.whl" }
private-lib = { version = "^1.0", source = "private-index" }
`))
	if err != nil {
		t.Fatal(err)
	}
	wantSources := map[string]string{
		"git-lib":     "git:https://github.com/example/git-lib.git#branch=main",
		"path-lib":    "path:../path-lib",
		"url-lib":     "url:https://example.com/url-lib.whl",
		"private-lib": "source:private-index",
	}
	if len(result.Dependencies) != len(wantSources) {
		t.Fatalf("expected source dependencies, got %#v", result.Dependencies)
	}
	for _, dependency := range result.Dependencies {
		if dependency.Source != wantSources[dependency.Name] {
			t.Fatalf("unexpected source for %s: got %#v", dependency.Name, dependency)
		}
	}
}

func TestParsePoetryPyProjectGroupDependencies(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry.dev-dependencies]
pytest = "^8"

[tool.poetry.group.docs.dependencies]
mkdocs = "^1.6"

[tool.poetry.group.release.dependencies]
twine = "^5"
`))
	if err != nil {
		t.Fatal(err)
	}
	wantScopes := map[string]Scope{
		"pytest": ScopeDev,
		"mkdocs": ScopeDev,
		"twine":  ScopeUnknown,
	}
	if len(result.Dependencies) != len(wantScopes) {
		t.Fatalf("expected group dependencies, got %#v", result.Dependencies)
	}
	for _, dependency := range result.Dependencies {
		if dependency.Scope != wantScopes[dependency.Name] {
			t.Fatalf("unexpected scope for %s: got %#v", dependency.Name, dependency)
		}
	}
	if !hasUnsupportedReason(result.Unsupported, "Poetry dependency group scope is not classified by the local fallback in this phase") {
		t.Fatalf("expected unclassified group note, got %#v", result.Unsupported)
	}
}

func TestParsePoetryPyProjectUnknownDependencyShape(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry.dependencies]
bad = ["not", "supported"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 0 {
		t.Fatalf("expected unknown shape to stay out of dependencies, got %#v", result.Dependencies)
	}
	if len(result.Unsupported) != 1 || !strings.Contains(result.Unsupported[0].Reason, "unsupported value shape") {
		t.Fatalf("expected unsupported unknown shape, got %#v", result.Unsupported)
	}
}

func TestParsePoetryLockfilePackagesAndSources(t *testing.T) {
	lockfile, err := ParsePoetryLockfile([]byte(`
[[package]]
name = "Requests"
version = "2.32.3"
category = "main"
optional = false

[[package]]
name = "git-lib"
version = "0.1.0"
groups = ["main"]
optional = true

[package.source]
type = "git"
url = "https://github.com/example/git-lib.git"
reference = "main"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 2 {
		t.Fatalf("expected lock packages, got %#v", lockfile.Packages)
	}
	requests := lockfilePackage(t, lockfile, "requests")
	if requests.Version != "2.32.3" || requests.Category != "main" || requests.Optional {
		t.Fatalf("unexpected requests lock package: %#v", requests)
	}
	gitLib := lockfilePackage(t, lockfile, "git-lib")
	if gitLib.Version != "0.1.0" || !gitLib.Optional || gitLib.Source != "git:https://github.com/example/git-lib.git#main" {
		t.Fatalf("unexpected git lock package: %#v", gitLib)
	}
}

func TestParsePoetryLockfileUnknownSchemaIsUnsupported(t *testing.T) {
	lockfile, err := ParsePoetryLockfile([]byte(`
[metadata]
lock-version = "2.1"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 0 {
		t.Fatalf("expected no packages, got %#v", lockfile.Packages)
	}
	if len(lockfile.Unsupported) != 1 {
		t.Fatalf("expected unsupported lockfile schema note, got %#v", lockfile.Unsupported)
	}
}

func TestApplyPoetryLockfileUsesResolvedVersionAndSourcePrecedence(t *testing.T) {
	result := ParseResult{Dependencies: []Dependency{
		{Name: "requests", Requirement: "^2.32", Scope: ScopeRuntime},
		{Name: "git-lib", Requirement: "source=git:https://github.com/example/from-pyproject.git", Source: "git:https://github.com/example/from-pyproject.git", Scope: ScopeRuntime},
	}}
	lockfile := Lockfile{Packages: []LockPackage{
		{Name: "requests", Version: "2.32.3", Source: "legacy:https://pypi.org/simple"},
		{Name: "git-lib", Version: "0.1.0", Source: "git:https://github.com/example/from-lock.git#main"},
	}}

	enriched := ApplyPoetryLockfile(result, lockfile)
	requests := dependencyByName(t, enriched, "requests")
	if requests.Version != "2.32.3" || requests.Source != "legacy:https://pypi.org/simple" {
		t.Fatalf("expected lockfile to enrich missing version/source, got %#v", requests)
	}
	gitLib := dependencyByName(t, enriched, "git-lib")
	if gitLib.Version != "0.1.0" || gitLib.Source != "git:https://github.com/example/from-pyproject.git" {
		t.Fatalf("expected pyproject source to beat lockfile source, got %#v", gitLib)
	}
}

func lockfilePackage(t *testing.T, lockfile Lockfile, name string) LockPackage {
	t.Helper()
	for _, item := range lockfile.Packages {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("expected lock package %s, got %#v", name, lockfile.Packages)
	return LockPackage{}
}

func dependencyByName(t *testing.T, result ParseResult, name string) Dependency {
	t.Helper()
	for _, dependency := range result.Dependencies {
		if dependency.Name == name {
			return dependency
		}
	}
	t.Fatalf("expected dependency %s, got %#v", name, result.Dependencies)
	return Dependency{}
}
