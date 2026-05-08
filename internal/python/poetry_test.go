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

func TestParsePoetryPyProjectStandardTableDependency(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry.dependencies.foo]
version = "^1.0"
markers = "python_version >= '3.10'"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 {
		t.Fatalf("expected one standard table dependency, got %#v", result.Dependencies)
	}
	dependency := result.Dependencies[0]
	if dependency.Name != "foo" || dependency.Scope != ScopeRuntime {
		t.Fatalf("unexpected standard table dependency: %#v", dependency)
	}
	for _, expected := range []string{"^1.0", "markers=python_version >= '3.10'"} {
		if !strings.Contains(dependency.Requirement, expected) {
			t.Fatalf("expected requirement to preserve %q, got %#v", expected, dependency)
		}
	}
}

func TestParsePoetryPyProjectSupportedMetadataKeys(t *testing.T) {
	result, err := ParsePoetryPyProject([]byte(`
[tool.poetry.dependencies]
rich = { version = "^13", python = ">=3.10", platform = "linux", allow-prereleases = true }
editable-lib = { path = "../editable-lib", develop = true }
`))
	if err != nil {
		t.Fatal(err)
	}
	rich := dependencyByName(t, result, "rich")
	for _, expected := range []string{"^13", "python=>=3.10", "platform=linux", "allow-prereleases=true"} {
		if !strings.Contains(rich.Requirement, expected) {
			t.Fatalf("expected rich requirement to preserve %q, got %#v", expected, rich)
		}
	}
	editable := dependencyByName(t, result, "editable-lib")
	for _, expected := range []string{"develop=true", "source=path:../editable-lib"} {
		if !strings.Contains(editable.Requirement, expected) {
			t.Fatalf("expected editable requirement to preserve %q, got %#v", expected, editable)
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

func TestParsePoetryPyProjectUnsupportedTableKeyAndValueType(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantReason string
	}{
		{
			name: "unknown key",
			data: `
[tool.poetry.dependencies]
bad = { version = "^1", unknown = "value" }
`,
			wantReason: "unsupported table key",
		},
		{
			name: "invalid value type",
			data: `
[tool.poetry.dependencies]
bad = { version = 1 }
`,
			wantReason: "unsupported value for key",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ParsePoetryPyProject([]byte(test.data))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Dependencies) != 0 {
				t.Fatalf("expected unsupported table not to become dependency, got %#v", result.Dependencies)
			}
			if len(result.Unsupported) != 1 || !strings.Contains(result.Unsupported[0].Reason, test.wantReason) {
				t.Fatalf("expected unsupported reason containing %q, got %#v", test.wantReason, result.Unsupported)
			}
		})
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

func TestParsePoetryLockfileEmptyIsAllowed(t *testing.T) {
	lockfile, err := ParsePoetryLockfile([]byte(" \n\t"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 0 || len(lockfile.Unsupported) != 0 {
		t.Fatalf("expected empty lockfile to parse as empty, got %#v", lockfile)
	}
}

func TestParsePoetryLockfileMalformedTOMLReturnsClearError(t *testing.T) {
	_, err := ParsePoetryLockfile([]byte("[[package]\nname = \"broken\"\n"))
	if err == nil || !strings.Contains(err.Error(), "parse poetry.lock") {
		t.Fatalf("expected clear poetry.lock parse error, got %v", err)
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

func TestParsePoetryLockfileMissingNameIsUnsupported(t *testing.T) {
	lockfile, err := ParsePoetryLockfile([]byte(`
[[package]]
version = "1.0.0"

[[package]]
name = "requests"
version = "2.32.3"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 1 || lockfile.Packages[0].Name != "requests" {
		t.Fatalf("expected named package to remain, got %#v", lockfile.Packages)
	}
	if len(lockfile.Unsupported) != 1 || !strings.Contains(lockfile.Unsupported[0].Reason, "missing a package name") {
		t.Fatalf("expected missing-name unsupported note, got %#v", lockfile.Unsupported)
	}
}

func TestParsePoetryLockfileResolvedReferenceFallback(t *testing.T) {
	lockfile, err := ParsePoetryLockfile([]byte(`
[[package]]
name = "git-lib"
version = "0.1.0"

[package.source]
type = "git"
url = "https://github.com/example/git-lib.git"
resolved_reference = "abc123"
`))
	if err != nil {
		t.Fatal(err)
	}
	gitLib := lockfilePackage(t, lockfile, "git-lib")
	if gitLib.Source != "git:https://github.com/example/git-lib.git#abc123" || gitLib.SourceReference != "abc123" {
		t.Fatalf("expected resolved_reference source fallback, got %#v", gitLib)
	}
}

func TestParsePoetryLockfileSortsPackagesAndKeepsFirstDuplicate(t *testing.T) {
	lockfile, err := ParsePoetryLockfile([]byte(`
[[package]]
name = "zeta"
version = "1.0.0"

[[package]]
name = "requests"
version = "2.32.2"

[[package]]
name = "requests"
version = "2.32.3"

[[package]]
name = "alpha"
version = "1.0.0"
`))
	if err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, 0, len(lockfile.Packages))
	for _, item := range lockfile.Packages {
		gotNames = append(gotNames, item.Name)
	}
	wantNames := []string{"alpha", "requests", "requests", "zeta"}
	for index, want := range wantNames {
		if gotNames[index] != want {
			t.Fatalf("expected sorted package names %#v, got %#v", wantNames, gotNames)
		}
	}

	enriched := ApplyPoetryLockfile(ParseResult{Dependencies: []Dependency{
		{Name: "requests", Requirement: "^2.32", Scope: ScopeRuntime},
	}}, lockfile)
	requests := dependencyByName(t, enriched, "requests")
	if requests.Version != "2.32.2" {
		t.Fatalf("expected first duplicate package to win, got %#v", requests)
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
