package python

import "testing"

func TestParsePyProjectProjectDependencies(t *testing.T) {
	result, err := ParsePyProject([]byte(`
[project]
dependencies = [
  "requests==2.32.3",
  "django>=4.2,<5",
]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 2 {
		t.Fatalf("expected dependencies, got %#v", result.Dependencies)
	}
	for _, dependency := range result.Dependencies {
		if dependency.Scope != ScopeRuntime {
			t.Fatalf("expected runtime scope, got %#v", dependency)
		}
	}
}

func TestParsePyProjectOptionalDependencies(t *testing.T) {
	result, err := ParsePyProject([]byte(`
[project.optional-dependencies]
dev = ["pytest>=8"]
docs = ["mkdocs==1.6.1"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 2 {
		t.Fatalf("expected optional dependencies, got %#v", result.Dependencies)
	}
	for _, dependency := range result.Dependencies {
		if dependency.Scope != ScopeOptional {
			t.Fatalf("expected optional scope, got %#v", dependency)
		}
	}
}

func TestParsePyProjectRecordsUnsupportedFutureScopes(t *testing.T) {
	result, err := ParsePyProject([]byte(`
[project]
dependencies = ["requests"]

[tool.poetry.dependencies]
python = "^3.12"

[dependency-groups]
dev = ["pytest"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 {
		t.Fatalf("expected one PEP 621 dependency, got %#v", result.Dependencies)
	}
	if len(result.Unsupported) != 2 {
		t.Fatalf("expected Poetry and dependency-groups unsupported notes, got %#v", result.Unsupported)
	}
}
