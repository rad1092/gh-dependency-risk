package render

import (
	"strings"
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func TestRenderMarkdown(t *testing.T) {
	report := Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number: 123,
			URL:    "https://github.com/owner/repo/pull/123",
			Title:  "Update dependencies",
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
					Score:       48,
					RiskDrivers: []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
					FromVersion: "1.0.0",
					ToVersion:   "2.0.0",
				},
			},
			RiskDrivers:        []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
			RecommendedActions: []string{analysis.ActionReviewChangelog, analysis.ActionInspectInstall},
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
							Score:       48,
							RiskDrivers: []string{analysis.DriverMajorVersionBump, analysis.DriverInstallScript},
							FromVersion: "1.0.0",
							ToVersion:   "2.0.0",
						},
					},
				},
			},
		},
	}

	output, err := Render(report, "markdown", "ko")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output, "<!-- gh-dep-risk -->") {
		t.Fatalf("expected marker comment prefix")
	}
	if !strings.Contains(output, "영향 범위") {
		t.Fatalf("expected korean labels in markdown output")
	}
	if !strings.Contains(output, "타깃 결과") {
		t.Fatalf("expected target section in markdown output")
	}
	if !strings.Contains(output, "`npm ls left-pad`") {
		t.Fatalf("expected quick command")
	}
}
