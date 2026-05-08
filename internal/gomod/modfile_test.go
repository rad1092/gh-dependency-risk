package gomod

import "testing"

func TestParseModFileRequirements(t *testing.T) {
	manifest, err := ParseModFile([]byte(`module example.com/app

go 1.22
toolchain go1.22.5

require example.com/direct v1.2.3

require (
	example.com/grouped v0.4.0
	example.com/indirect v0.5.0 // indirect
)
`))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ModulePath != "example.com/app" || manifest.GoVersion != "1.22" || manifest.Toolchain != "go1.22.5" {
		t.Fatalf("unexpected directives: %#v", manifest)
	}
	if len(manifest.Requirements) != 3 {
		t.Fatalf("expected 3 requirements, got %#v", manifest.Requirements)
	}
	assertRequirement(t, manifest.Requirements, "example.com/direct", "v1.2.3", false)
	assertRequirement(t, manifest.Requirements, "example.com/grouped", "v0.4.0", false)
	assertRequirement(t, manifest.Requirements, "example.com/indirect", "v0.5.0", true)
}

func TestParseModFileReplacements(t *testing.T) {
	manifest, err := ParseModFile([]byte(`module example.com/app

go 1.22

replace example.com/local => ../local/path
replace example.com/remote => example.com/fork/remote v1.2.3
replace example.com/versioned v1.0.0 => example.com/fork/versioned v1.0.1
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Replacements) != 3 {
		t.Fatalf("expected replacements, got %#v", manifest.Replacements)
	}
	local := findReplacement(t, manifest.Replacements, "example.com/local")
	if !local.Local || local.NewPath != "../local/path" {
		t.Fatalf("expected local replacement, got %#v", local)
	}
	remote := findReplacement(t, manifest.Replacements, "example.com/remote")
	if remote.Local || remote.NewPath != "example.com/fork/remote" || remote.NewVersion != "v1.2.3" {
		t.Fatalf("expected remote replacement, got %#v", remote)
	}
	versioned := findReplacement(t, manifest.Replacements, "example.com/versioned", "v1.0.0")
	if versioned.Local || versioned.NewPath != "example.com/fork/versioned" || versioned.NewVersion != "v1.0.1" {
		t.Fatalf("expected version-specific replacement, got %#v", versioned)
	}
}

func TestParseModFileMultipleVersionSpecificReplacements(t *testing.T) {
	manifest, err := ParseModFile([]byte(`module example.com/app

go 1.22

replace (
	example.com/lib v1.0.0 => example.com/fork/lib v1.0.1
	example.com/lib v2.0.0 => example.com/fork/lib/v2 v2.0.1
)
`))
	if err != nil {
		t.Fatal(err)
	}
	first := findReplacement(t, manifest.Replacements, "example.com/lib", "v1.0.0")
	second := findReplacement(t, manifest.Replacements, "example.com/lib", "v2.0.0")
	if first.NewVersion != "v1.0.1" || second.NewVersion != "v2.0.1" {
		t.Fatalf("expected distinct old-version replacements, got %#v", manifest.Replacements)
	}
}

func TestParseModFileMalformedReturnsError(t *testing.T) {
	_, err := ParseModFile([]byte("module example.com/app\nrequire (\nexample.com/lib v1.2.3\n"))
	if err == nil {
		t.Fatalf("expected malformed go.mod parse error")
	}
}

func assertRequirement(t *testing.T, requirements []Requirement, path, version string, indirect bool) {
	t.Helper()
	for _, requirement := range requirements {
		if requirement.Path == path {
			if requirement.Version != version || requirement.Indirect != indirect {
				t.Fatalf("unexpected requirement for %s: %#v", path, requirement)
			}
			return
		}
	}
	t.Fatalf("expected requirement %s, got %#v", path, requirements)
}

func findReplacement(t *testing.T, replacements []Replacement, path string, oldVersion ...string) Replacement {
	t.Helper()
	version := ""
	if len(oldVersion) > 0 {
		version = oldVersion[0]
	}
	for _, replacement := range replacements {
		if replacement.OldPath == path && replacement.OldVersion == version {
			return replacement
		}
	}
	t.Fatalf("expected replacement %s@%s, got %#v", path, version, replacements)
	return Replacement{}
}
