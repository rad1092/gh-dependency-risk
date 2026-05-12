package review

import (
	"testing"

	ghclient "github.com/rad1092/gh-dependency-risk/internal/github"
)

func TestNormalizeChangesSupportsMixedEcosystems(t *testing.T) {
	raw := []ghclient.DependencyReviewChange{
		{Name: "serde", Manifest: "Cargo.toml", Ecosystem: "cargo", ChangeType: "added", Version: "1.0.204"},
		{Name: "axios", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "1.7.0"},
		{Name: "golang.org/x/text", Manifest: "go.mod", Ecosystem: "gomod", ChangeType: "added", Version: "0.16.0"},
		{Name: "requests", Manifest: "pyproject.toml", Ecosystem: "poetry", ChangeType: "added", Version: "2.32.3"},
	}

	changes := NormalizeChanges(raw)
	if len(changes) != 4 {
		t.Fatalf("expected 4 normalized changes, got %d", len(changes))
	}

	targets := TargetsFromChanges(changes)
	if len(targets) != 4 {
		t.Fatalf("expected 4 targets, got %d", len(targets))
	}

	want := map[string]struct {
		ecosystem Ecosystem
		manager   PackageManager
	}{
		"Cargo.toml":     {ecosystem: EcosystemCargo, manager: PackageManagerCargo},
		"go.mod":         {ecosystem: EcosystemGoModules, manager: PackageManagerGo},
		"package.json":   {ecosystem: EcosystemNPM, manager: PackageManagerNPM},
		"pyproject.toml": {ecosystem: EcosystemPoetry, manager: PackageManagerPoetry},
	}
	for _, target := range targets {
		expected, ok := want[target.ManifestPath]
		if !ok {
			t.Fatalf("unexpected target %q", target.ManifestPath)
		}
		if target.Ecosystem != expected.ecosystem || target.PackageManager != expected.manager {
			t.Fatalf("unexpected target metadata for %s: %+v", target.ManifestPath, target)
		}
	}
}

func TestNormalizeChangesSupportsYarnAndDedupesByManagerAwareTargetIdentity(t *testing.T) {
	raw := []ghclient.DependencyReviewChange{
		{Name: "react", Manifest: "package.json", Ecosystem: "yarn", ChangeType: "removed", Version: "18.2.0"},
		{Name: "react", Manifest: "package.json", Ecosystem: "yarn", ChangeType: "added", Version: "18.3.1"},
		{Name: "react", Manifest: "package.json", Ecosystem: "npm", ChangeType: "added", Version: "18.3.1"},
	}

	changes := NormalizeChanges(raw)
	grouped := ChangesByTarget(changes)
	if len(grouped) != 2 {
		t.Fatalf("expected manager-aware target grouping, got %d groups", len(grouped))
	}
	if _, ok := grouped[TargetIdentity("package.json", EcosystemYarn, PackageManagerYarn)]; !ok {
		t.Fatalf("expected yarn target group")
	}
	if _, ok := grouped[TargetIdentity("package.json", EcosystemNPM, PackageManagerNPM)]; !ok {
		t.Fatalf("expected npm target group")
	}
}

func TestNormalizeEcosystemSupportsCommonDependencyReviewAliases(t *testing.T) {
	cases := []struct {
		input     string
		ecosystem Ecosystem
		manager   PackageManager
	}{
		{input: "go-modules", ecosystem: EcosystemGoModules, manager: PackageManagerGo},
		{input: "go_modules", ecosystem: EcosystemGoModules, manager: PackageManagerGo},
		{input: "swift-package-manager", ecosystem: EcosystemSwiftPM, manager: PackageManagerSwiftPM},
		{input: "ruby_gems", ecosystem: EcosystemRubyGems, manager: PackageManagerBundler},
		{input: "rubygem", ecosystem: EcosystemRubyGems, manager: PackageManagerBundler},
	}

	for _, tc := range cases {
		gotEcosystem, gotManager, ok := NormalizeEcosystem(tc.input)
		if !ok {
			t.Fatalf("expected %q to normalize", tc.input)
		}
		if gotEcosystem != tc.ecosystem || gotManager != tc.manager {
			t.Fatalf("unexpected normalization for %q: got (%s, %s)", tc.input, gotEcosystem, gotManager)
		}
	}
}

func TestHasLocalFallbackIncludesPoetryAndGo(t *testing.T) {
	if !HasLocalFallback(PackageManagerPoetry) {
		t.Fatalf("expected Poetry to have local fallback")
	}
	if !HasLocalFallback(PackageManagerGo) {
		t.Fatalf("expected Go modules to have local fallback")
	}
}
