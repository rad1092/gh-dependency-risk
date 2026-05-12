package analysis

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rad1092/gh-dependency-risk/internal/npm"
)

func TestAnalyzeScoresAndCaps(t *testing.T) {
	baseManifestData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "base.package.json"))
	headManifestData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "head.package.json"))
	baseLockData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "base.package-lock.json"))
	headLockData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "head.package-lock.json"))

	baseManifest, _ := npm.ParsePackageManifest(baseManifestData)
	headManifest, _ := npm.ParsePackageManifest(headManifestData)
	baseLockfile, _ := npm.ParseLockfile(baseLockData)
	headLockfile, _ := npm.ParseLockfile(headLockData)

	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	result := Analyze(Input{
		Now:                       now,
		DependencyReviewAvailable: true,
		ReviewChanges: []ReviewChange{
			{Name: "left-pad", Manifest: "package.json", ChangeType: ChangeRemoved, Version: "1.0.0"},
			{Name: "left-pad", Manifest: "package.json", ChangeType: ChangeAdded, Version: "2.0.0", Vulnerabilities: []Vulnerability{{GHSAID: "GHSA-1", Severity: "high", Summary: "demo", URL: "https://example.com"}}},
			{Name: "chalk", Manifest: "package.json", ChangeType: ChangeAdded, Version: "5.0.0"},
		},
		BaseManifest: baseManifest,
		HeadManifest: headManifest,
		BaseLockfile: baseLockfile,
		HeadLockfile: headLockfile,
	}, map[PackageVersion]time.Time{
		{Name: "left-pad", Version: "2.0.0"}: now.Add(-48 * time.Hour),
		{Name: "chalk", Version: "5.0.0"}:    now.Add(-24 * time.Hour),
	})

	if result.Score < 70 {
		t.Fatalf("expected critical score, got %d", result.Score)
	}
	if result.Level != RiskLevelCritical {
		t.Fatalf("expected critical level, got %s", result.Level)
	}
	if len(result.ChangedDependencies) != 2 {
		t.Fatalf("expected 2 dependency changes, got %d", len(result.ChangedDependencies))
	}
	if result.ChangedDependencies[0].Score != 100 {
		t.Fatalf("expected score cap at 100, got %d", result.ChangedDependencies[0].Score)
	}
}

func TestAggregateResultsAcrossTargets(t *testing.T) {
	targets := []TargetAnalysisResult{
		{
			Target:               AnalysisTarget{DisplayName: "apps/web", ManifestPath: "apps/web/package.json", LockfilePath: "package-lock.json", Kind: TargetKindWorkspace},
			Score:                48,
			Level:                RiskLevelHigh,
			BlastRadius:          BlastRadiusMedium,
			ChangedDependencies:  []DependencyChange{{Name: "axios", Target: "apps/web", Score: 48, RiskDrivers: []string{DriverAddedDirectRuntime}}},
			RiskDrivers:          []string{DriverAddedDirectRuntime},
			RecommendedActions:   []string{ActionRunTargetedTests},
			QuickCommands:        []string{"cd apps/web && npm ls axios"},
			AddedTransitiveCount: 2,
			addedTransitiveKeys:  []string{"node_modules/follow-redirects", "node_modules/form-data"},
		},
		{
			Target:               AnalysisTarget{DisplayName: "packages/ui", ManifestPath: "packages/ui/package.json", LockfilePath: "package-lock.json", Kind: TargetKindWorkspace},
			Score:                22,
			Level:                RiskLevelMedium,
			BlastRadius:          BlastRadiusLow,
			ChangedDependencies:  []DependencyChange{{Name: "tailwind-merge", Target: "packages/ui", Score: 22, RiskDrivers: []string{DriverAddedDirectRuntime}}},
			RiskDrivers:          []string{DriverAddedDirectRuntime},
			RecommendedActions:   []string{ActionRunTargetedTests},
			QuickCommands:        []string{"cd packages/ui && npm ls tailwind-merge"},
			AddedTransitiveCount: 1,
			addedTransitiveKeys:  []string{"node_modules/tailwind-merge/node_modules/postcss"},
		},
	}

	result := AggregateResults(targets)
	if result.Score != 52 {
		t.Fatalf("expected aggregate score with deterministic bonus, got %d", result.Score)
	}
	if result.Level != RiskLevelHigh {
		t.Fatalf("expected aggregate high level, got %s", result.Level)
	}
	if result.AddedTransitiveCount != 3 {
		t.Fatalf("expected deduped transitive count, got %d", result.AddedTransitiveCount)
	}
	if len(result.Targets) != 2 || len(result.ChangedDependencies) != 2 {
		t.Fatalf("expected flattened aggregate result, got %#v", result)
	}
}

func TestAggregateResultsDedupesSharedTransitivePathsAcrossTargets(t *testing.T) {
	targets := []TargetAnalysisResult{
		{
			Target: AnalysisTarget{DisplayName: "apps/web", ManifestPath: "apps/web/package.json", LockfilePath: "package-lock.json", Kind: TargetKindWorkspace},
			Score:  18,
			Level:  RiskLevelLow,
			ChangedDependencies: []DependencyChange{
				{Name: "eslint-config-custom", Target: "apps/web", Scope: ScopeDev, Direct: true, ChangeType: ChangeAdded, Score: 18},
			},
			AddedTransitiveCount: 3,
			addedTransitiveKeys: []string{
				"node_modules/@eslint/js",
				"node_modules/eslint-plugin-import",
				"node_modules/globals",
			},
		},
		{
			Target: AnalysisTarget{DisplayName: "packages/ui", ManifestPath: "packages/ui/package.json", LockfilePath: "package-lock.json", Kind: TargetKindWorkspace},
			Score:  18,
			Level:  RiskLevelLow,
			ChangedDependencies: []DependencyChange{
				{Name: "eslint-config-custom", Target: "packages/ui", Scope: ScopeDev, Direct: true, ChangeType: ChangeAdded, Score: 18},
			},
			AddedTransitiveCount: 3,
			addedTransitiveKeys: []string{
				"node_modules/@eslint/js",
				"node_modules/eslint-plugin-import",
				"node_modules/globals",
			},
		},
	}

	result := AggregateResults(targets)
	if result.AddedTransitiveCount != 3 {
		t.Fatalf("expected shared transitive paths to count once, got %d", result.AddedTransitiveCount)
	}
	if result.BlastRadius != BlastRadiusLow {
		t.Fatalf("expected deduped aggregate blast radius to stay low, got %s", result.BlastRadius)
	}
}
