package python

import "testing"

func TestParseRequirementsSupportedSubset(t *testing.T) {
	data := []byte(`
# comment
Requests
flask==3.0.2
django>=4.2,<5
uvicorn[standard]>=0.29
typing-extensions; python_version < "3.12"
wheelhouse @ https://example.com/pkg.whl
git+https://github.com/example/demo.git#egg=Demo_Pkg
`)

	result, err := ParseRequirements(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Unsupported) != 0 {
		t.Fatalf("expected no unsupported entries, got %#v", result.Unsupported)
	}
	want := map[string]Dependency{
		"requests":          {Name: "requests"},
		"flask":             {Name: "flask", Requirement: "==3.0.2", Version: "3.0.2"},
		"django":            {Name: "django", Requirement: ">=4.2,<5"},
		"uvicorn":           {Name: "uvicorn", Requirement: "[standard]>=0.29"},
		"typing-extensions": {Name: "typing-extensions", Requirement: `; python_version < "3.12"`},
		"wheelhouse":        {Name: "wheelhouse", Requirement: "@ https://example.com/pkg.whl", Source: "https://example.com/pkg.whl"},
		"demo-pkg":          {Name: "demo-pkg", Requirement: "git+https://github.com/example/demo.git#egg=Demo_Pkg", Source: "git+https://github.com/example/demo.git#egg=Demo_Pkg"},
	}
	if len(result.Dependencies) != len(want) {
		t.Fatalf("expected %d dependencies, got %#v", len(want), result.Dependencies)
	}
	for _, dependency := range result.Dependencies {
		expected, ok := want[dependency.Name]
		if !ok {
			t.Fatalf("unexpected dependency %#v", dependency)
		}
		if dependency.Requirement != expected.Requirement || dependency.Version != expected.Version || dependency.Source != expected.Source {
			t.Fatalf("unexpected dependency for %s: got %#v want %#v", dependency.Name, dependency, expected)
		}
	}
}

func TestParseRequirementsUnsupportedInclude(t *testing.T) {
	result, err := ParseRequirements([]byte("-r other.txt\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 0 {
		t.Fatalf("expected no dependencies, got %#v", result.Dependencies)
	}
	if len(result.Unsupported) != 1 || result.Unsupported[0].Reason == "" {
		t.Fatalf("expected unsupported include note, got %#v", result.Unsupported)
	}
}

func TestParseRequirementsEdgeCases(t *testing.T) {
	data := []byte(`
My_Package.Name==1.0.0  # inline comment is ignored
typing-extensions; python_version < "3.12 # marker keeps hash" # trailing comment
flask \
  ==3.0.2
requests==2.32.3 --hash=sha256:abc123
wheelhouse @ https://${HOST}/pkg.whl
git+https://github.com/example/Demo.git#egg=Demo_Pkg
https://example.com/archive/pkg.whl
`)

	result, err := ParseRequirements(data)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]Dependency{
		"my-package-name":   {Name: "my-package-name", Requirement: "==1.0.0", Version: "1.0.0"},
		"typing-extensions": {Name: "typing-extensions", Requirement: `; python_version < "3.12 # marker keeps hash"`},
		"flask":             {Name: "flask", Requirement: "==3.0.2", Version: "3.0.2"},
		"demo-pkg":          {Name: "demo-pkg", Requirement: "git+https://github.com/example/Demo.git#egg=Demo_Pkg", Source: "git+https://github.com/example/Demo.git#egg=Demo_Pkg"},
	}
	if len(result.Dependencies) != len(want) {
		t.Fatalf("expected %d dependencies, got %#v", len(want), result.Dependencies)
	}
	for _, dependency := range result.Dependencies {
		expected, ok := want[dependency.Name]
		if !ok {
			t.Fatalf("unexpected dependency %#v", dependency)
		}
		if dependency.Requirement != expected.Requirement || dependency.Version != expected.Version || dependency.Source != expected.Source {
			t.Fatalf("unexpected dependency for %s: got %#v want %#v", dependency.Name, dependency, expected)
		}
	}

	if len(result.Unsupported) != 3 {
		t.Fatalf("expected hash, env URL, and bare URL unsupported entries, got %#v", result.Unsupported)
	}
	for _, expected := range []string{
		"per-requirement options are not supported",
		"source requirement contains an environment variable",
		"archive or URL requirement is missing a stable package name",
	} {
		if !hasUnsupportedReason(result.Unsupported, expected) {
			t.Fatalf("expected unsupported reason %q, got %#v", expected, result.Unsupported)
		}
	}
}

func hasUnsupportedReason(entries []UnsupportedEntry, expected string) bool {
	for _, entry := range entries {
		if entry.Reason == expected {
			return true
		}
	}
	return false
}
