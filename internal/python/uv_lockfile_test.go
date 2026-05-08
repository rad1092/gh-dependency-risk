package python

import (
	"strings"
	"testing"
)

func TestParseUvLockfilePackageNameVersion(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(`
version = 1
revision = 2

[[package]]
name = "Requests"
version = "2.32.3"
source = { registry = "https://pypi.org/simple" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 1 {
		t.Fatalf("expected one uv package, got %#v", lockfile.Packages)
	}
	requests := lockfilePackage(t, lockfile, "requests")
	if requests.Version != "2.32.3" || requests.Source != "" || requests.SourceType != "registry" {
		t.Fatalf("unexpected uv package: %#v", requests)
	}
	if len(lockfile.Unsupported) != 0 {
		t.Fatalf("did not expect registry source to be unsupported, got %#v", lockfile.Unsupported)
	}
}

func TestParseUvLockfileSourceShapes(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(`
[[package]]
name = "git-lib"
version = "0.1.0"
source = { git = "https://github.com/example/git-lib.git", rev = "abc123" }

[[package]]
name = "url-lib"
version = "0.2.0"
source = { url = "https://example.com/url-lib.whl" }

[[package]]
name = "path-lib"
version = "0.3.0"
source = { path = "../path-lib", editable = true }

[[package]]
name = "dir-lib"
version = "0.4.0"
source = { directory = "../dir-lib" }

[[package]]
name = "editable-lib"
version = "0.5.0"
source = { editable = "../editable-lib" }

[[package]]
name = "virtual-project"
version = "0.0.0"
source = { virtual = "." }

[[package]]
name = "workspace-lib"
version = "0.6.0"
source = { workspace = true }
`))
	if err != nil {
		t.Fatal(err)
	}
	wantSources := map[string]string{
		"git-lib":      "git:https://github.com/example/git-lib.git#rev=abc123",
		"url-lib":      "url:https://example.com/url-lib.whl",
		"path-lib":     "path:../path-lib#editable=true",
		"dir-lib":      "directory:../dir-lib",
		"editable-lib": "editable:../editable-lib",
	}
	for name, want := range wantSources {
		if got := lockfilePackage(t, lockfile, name).Source; got != want {
			t.Fatalf("expected source %s for %s, got %#v", want, name, lockfilePackage(t, lockfile, name))
		}
	}
	for _, name := range []string{"virtual-project", "workspace-lib"} {
		if got := lockfilePackage(t, lockfile, name).Source; got != "" {
			t.Fatalf("did not expect non-registry source for %s, got %#v", name, lockfilePackage(t, lockfile, name))
		}
	}
}

func TestParseUvLockfileUnknownSourceShapeIsUnsupported(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(`
[[package]]
name = "bad-source"
version = "1.0.0"
source = { unknown = "value" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 1 || lockfile.Packages[0].Source != "" {
		t.Fatalf("expected package version to remain without guessed source, got %#v", lockfile.Packages)
	}
	if len(lockfile.Unsupported) != 1 || !strings.Contains(lockfile.Unsupported[0].Reason, "unsupported source shape") {
		t.Fatalf("expected unsupported source shape note, got %#v", lockfile.Unsupported)
	}
}

func TestParseUvLockfileEmptyIsAllowed(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(" \n\t"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 0 || len(lockfile.Unsupported) != 0 {
		t.Fatalf("expected empty uv.lock to parse as empty, got %#v", lockfile)
	}
}

func TestParseUvLockfileMalformedTOMLReturnsClearError(t *testing.T) {
	_, err := ParseUvLockfile([]byte("[[package]\nname = \"broken\"\n"))
	if err == nil || !strings.Contains(err.Error(), "parse uv.lock") {
		t.Fatalf("expected clear uv.lock parse error, got %v", err)
	}
}

func TestParseUvLockfileNoPackagesIsUnsupported(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(`
version = 1
revision = 2
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(lockfile.Packages) != 0 {
		t.Fatalf("expected no packages, got %#v", lockfile.Packages)
	}
	if len(lockfile.Unsupported) != 1 || !strings.Contains(lockfile.Unsupported[0].Reason, "does not contain supported package entries") {
		t.Fatalf("expected no-package unsupported note, got %#v", lockfile.Unsupported)
	}
}

func TestParseUvLockfileMissingNameIsUnsupported(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(`
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

func TestParseUvLockfileSortsPackagesAndKeepsFirstDuplicate(t *testing.T) {
	lockfile, err := ParseUvLockfile([]byte(`
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

	enriched := ApplyUvLockfile(ParseResult{Dependencies: []Dependency{
		{Name: "requests", Requirement: ">=2.32", Scope: ScopeRuntime},
	}}, lockfile)
	requests := dependencyByName(t, enriched, "requests")
	if requests.Version != "2.32.2" {
		t.Fatalf("expected first duplicate package to win, got %#v", requests)
	}
}

func TestApplyUvLockfileUsesResolvedVersionAndSourcePrecedence(t *testing.T) {
	result := ParseResult{Dependencies: []Dependency{
		{Name: "requests", Requirement: ">=2.32", Scope: ScopeRuntime},
		{Name: "git-lib", Requirement: "@ git:https://github.com/example/from-pyproject.git", Source: "git:https://github.com/example/from-pyproject.git", Scope: ScopeRuntime},
	}}
	lockfile := Lockfile{Packages: []LockPackage{
		{Name: "requests", Version: "2.32.3", Source: "git:https://github.com/example/from-lock.git#main"},
		{Name: "git-lib", Version: "0.1.0", Source: "git:https://github.com/example/from-lock.git#main"},
	}}

	enriched := ApplyUvLockfile(result, lockfile)
	requests := dependencyByName(t, enriched, "requests")
	if requests.Version != "2.32.3" || requests.Source != "git:https://github.com/example/from-lock.git#main" {
		t.Fatalf("expected uv.lock to enrich missing version/source, got %#v", requests)
	}
	gitLib := dependencyByName(t, enriched, "git-lib")
	if gitLib.Version != "0.1.0" || gitLib.Source != "git:https://github.com/example/from-pyproject.git" {
		t.Fatalf("expected pyproject source to beat uv.lock source, got %#v", gitLib)
	}
}
