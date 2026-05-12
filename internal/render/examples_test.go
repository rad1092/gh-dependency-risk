package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func TestCheckedInExamplesStayInSync(t *testing.T) {
	testCases := []struct {
		name   string
		report Report
		files  map[string]string
	}{
		{
			name:   "single-target",
			report: sampleSingleTargetReport(),
			files: map[string]string{
				"human":    filepath.Join("..", "..", "docs", "examples", "single-target-human.txt"),
				"markdown": filepath.Join("..", "..", "docs", "examples", "single-target.md"),
				"json":     filepath.Join("..", "..", "docs", "examples", "single-target.json"),
			},
		},
		{
			name:   "multi-target",
			report: sampleMultiTargetReport(),
			files: map[string]string{
				"human":    filepath.Join("..", "..", "docs", "examples", "multi-target-human.txt"),
				"markdown": filepath.Join("..", "..", "docs", "examples", "multi-target.md"),
				"json":     filepath.Join("..", "..", "docs", "examples", "multi-target.json"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for format, examplePath := range tc.files {
				got, err := Render(tc.report, format, "en")
				if err != nil {
					t.Fatal(err)
				}
				wantBytes, err := os.ReadFile(filepath.Clean(examplePath))
				if err != nil {
					t.Fatal(err)
				}
				if got != string(wantBytes) {
					t.Fatalf("example %s (%s) is out of sync", examplePath, format)
				}
			}
		})
	}
}

func sampleSingleTargetReport() Report {
	return Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number:      123,
			URL:         "https://github.com/owner/repo/pull/123",
			Title:       "Update dependencies",
			Draft:       false,
			BaseSHA:     "base-sha",
			HeadSHA:     "head-sha",
			AuthorLogin: "octocat",
		},
		Analysis: analysis.AnalysisResult{
			DependencyReviewAvailable: false,
			Score:                     48,
			Level:                     analysis.RiskLevelHigh,
			BlastRadius:               analysis.BlastRadiusMedium,
			ChangedDependencies: []analysis.DependencyChange{
				{
					Name:        "left-pad",
					Target:      "root",
					ChangeType:  analysis.ChangeUpdated,
					Scope:       analysis.ScopeRuntime,
					Direct:      true,
					Score:       48,
					Level:       analysis.RiskLevelHigh,
					RiskDrivers: []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
					FromVersion: "1.0.0",
					ToVersion:   "2.0.0",
				},
			},
			RiskDrivers:        []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
			RecommendedActions: []string{analysis.ActionInspectInstall, analysis.ActionReviewChangelog},
			QuickCommands:      []string{"npm ls left-pad"},
			Notes:              []analysis.Note{{Code: analysis.NoteDependencyReviewFallback}},
			Targets: []analysis.TargetAnalysisResult{
				{
					Target:                    analysis.AnalysisTarget{DisplayName: "root", ManifestPath: "package.json", LockfilePath: "package-lock.json", Kind: analysis.TargetKindRoot, PackageManager: "npm", Ecosystem: "npm"},
					DependencyReviewAvailable: false,
					Score:                     48,
					Level:                     analysis.RiskLevelHigh,
					BlastRadius:               analysis.BlastRadiusMedium,
					ChangedDependencies: []analysis.DependencyChange{
						{
							Name:        "left-pad",
							Target:      "root",
							ChangeType:  analysis.ChangeUpdated,
							Scope:       analysis.ScopeRuntime,
							Direct:      true,
							Score:       48,
							Level:       analysis.RiskLevelHigh,
							RiskDrivers: []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
							FromVersion: "1.0.0",
							ToVersion:   "2.0.0",
						},
					},
					RiskDrivers:        []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
					RecommendedActions: []string{analysis.ActionInspectInstall, analysis.ActionReviewChangelog},
					QuickCommands:      []string{"npm ls left-pad"},
					Notes:              []analysis.Note{{Code: analysis.NoteDependencyReviewFallback}},
				},
			},
		},
	}
}

func sampleMultiTargetReport() Report {
	return Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number:      456,
			URL:         "https://github.com/owner/repo/pull/456",
			Title:       "Update workspace dependencies",
			Draft:       false,
			BaseSHA:     "base-workspace",
			HeadSHA:     "head-workspace",
			AuthorLogin: "octocat",
		},
		Analysis: analysis.AnalysisResult{
			DependencyReviewAvailable: true,
			Score:                     52,
			Level:                     analysis.RiskLevelHigh,
			BlastRadius:               analysis.BlastRadiusMedium,
			ChangedDependencies: []analysis.DependencyChange{
				{
					Name:        "axios",
					Target:      "apps/web",
					ChangeType:  analysis.ChangeAdded,
					Scope:       analysis.ScopeRuntime,
					Direct:      true,
					Score:       48,
					Level:       analysis.RiskLevelHigh,
					RiskDrivers: []string{analysis.DriverAddedDirectRuntime},
					ToVersion:   "1.7.0",
				},
				{
					Name:        "tailwind-merge",
					Target:      "packages/ui",
					ChangeType:  analysis.ChangeAdded,
					Scope:       analysis.ScopeRuntime,
					Direct:      true,
					Score:       22,
					Level:       analysis.RiskLevelMedium,
					RiskDrivers: []string{analysis.DriverAddedDirectRuntime},
					ToVersion:   "2.3.0",
				},
			},
			RiskDrivers:        []string{analysis.DriverAddedDirectRuntime},
			RecommendedActions: []string{analysis.ActionRunTargetedTests},
			QuickCommands: []string{
				"cd apps/web && npm ls --all",
				"cd apps/web && npm ls axios",
				"cd packages/ui && npm ls --all",
			},
			Targets: []analysis.TargetAnalysisResult{
				{
					Target:                    analysis.AnalysisTarget{DisplayName: "apps/web", ManifestPath: "apps/web/package.json", LockfilePath: "package-lock.json", Kind: analysis.TargetKindWorkspace, PackageManager: "npm", Ecosystem: "npm"},
					DependencyReviewAvailable: true,
					Score:                     48,
					Level:                     analysis.RiskLevelHigh,
					BlastRadius:               analysis.BlastRadiusMedium,
					ChangedDependencies: []analysis.DependencyChange{
						{
							Name:        "axios",
							Target:      "apps/web",
							ChangeType:  analysis.ChangeAdded,
							Scope:       analysis.ScopeRuntime,
							Direct:      true,
							Score:       48,
							Level:       analysis.RiskLevelHigh,
							RiskDrivers: []string{analysis.DriverAddedDirectRuntime},
							ToVersion:   "1.7.0",
						},
					},
					RiskDrivers:        []string{analysis.DriverAddedDirectRuntime},
					RecommendedActions: []string{analysis.ActionRunTargetedTests},
					QuickCommands:      []string{"cd apps/web && npm ls --all", "cd apps/web && npm ls axios"},
				},
				{
					Target:                    analysis.AnalysisTarget{DisplayName: "packages/ui", ManifestPath: "packages/ui/package.json", LockfilePath: "package-lock.json", Kind: analysis.TargetKindWorkspace, PackageManager: "npm", Ecosystem: "npm"},
					DependencyReviewAvailable: true,
					Score:                     22,
					Level:                     analysis.RiskLevelMedium,
					BlastRadius:               analysis.BlastRadiusLow,
					ChangedDependencies: []analysis.DependencyChange{
						{
							Name:        "tailwind-merge",
							Target:      "packages/ui",
							ChangeType:  analysis.ChangeAdded,
							Scope:       analysis.ScopeRuntime,
							Direct:      true,
							Score:       22,
							Level:       analysis.RiskLevelMedium,
							RiskDrivers: []string{analysis.DriverAddedDirectRuntime},
							ToVersion:   "2.3.0",
						},
					},
					RiskDrivers:        []string{analysis.DriverAddedDirectRuntime},
					RecommendedActions: []string{analysis.ActionRunTargetedTests},
					QuickCommands:      []string{"cd packages/ui && npm ls --all"},
				},
			},
		},
	}
}
