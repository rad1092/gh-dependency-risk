package python

import (
	"strings"
	"testing"
)

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
	if result.Dependencies[0].Name != "django" || result.Dependencies[1].Name != "requests" {
		t.Fatalf("expected normalized sorted runtime dependencies, got %#v", result.Dependencies)
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
	if result.Dependencies[0].Name != "mkdocs" || result.Dependencies[1].Name != "pytest" {
		t.Fatalf("expected normalized sorted optional dependencies, got %#v", result.Dependencies)
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
	if !hasUnsupportedReason(result.Unsupported, "Poetry dependency tables are not supported by the Python direct local fallback in this phase") {
		t.Fatalf("expected Poetry unsupported note, got %#v", result.Unsupported)
	}
	if !hasUnsupportedReason(result.Unsupported, "dependency groups are not supported by the Python direct local fallback in this phase") {
		t.Fatalf("expected dependency-groups unsupported note, got %#v", result.Unsupported)
	}
}

func TestHasPEP621DependenciesIgnoresPoetryOnlyPyProject(t *testing.T) {
	ok, err := HasPEP621Dependencies([]byte(`
[tool.poetry]
name = "demo"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
requests = "^2.32"
`))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("expected Poetry-only pyproject.toml not to be promoted to Python direct fallback")
	}
}

func TestParsePyProjectUnsupportedPEP621Entry(t *testing.T) {
	result, err := ParsePyProject([]byte(`
[project]
dependencies = [
  "requests==2.32.3 --hash=sha256:abc123",
]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 0 {
		t.Fatalf("expected unsupported entry to stay out of dependencies, got %#v", result.Dependencies)
	}
	if len(result.Unsupported) != 1 || !strings.Contains(result.Unsupported[0].Reason, "per-requirement options") {
		t.Fatalf("expected unsupported PEP 621 entry note, got %#v", result.Unsupported)
	}
}
