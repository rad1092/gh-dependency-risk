package render

import (
	"encoding/json"
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func TestRenderJSONStableSchema(t *testing.T) {
	report := Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number:      123,
			URL:         "https://github.com/owner/repo/pull/123",
			Title:       "Update dependencies",
			BaseSHA:     "base",
			HeadSHA:     "head",
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
					ChangeType:  analysis.ChangeUpdated,
					Scope:       analysis.ScopeRuntime,
					Score:       48,
					RiskDrivers: []string{analysis.DriverMajorVersionBump},
					FromVersion: "1.0.0",
					ToVersion:   "2.0.0",
				},
			},
			RiskDrivers:        []string{analysis.DriverMajorVersionBump},
			RecommendedActions: []string{analysis.ActionReviewChangelog},
			QuickCommands:      []string{"npm ls left-pad"},
			Notes:              []analysis.Note{{Code: analysis.NoteDependencyReviewFallback}},
			Targets: []analysis.TargetAnalysisResult{
				{
					Target:                    analysis.AnalysisTarget{DisplayName: "root", ManifestPath: "package.json", LockfilePath: "package-lock.json", Kind: analysis.TargetKindRoot},
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
							RiskDrivers: []string{analysis.DriverMajorVersionBump},
							FromVersion: "1.0.0",
							ToVersion:   "2.0.0",
						},
					},
				},
			},
		},
	}

	output, err := Render(report, "json", "en")
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["analysis"]; ok {
		t.Fatalf("expected flattened JSON schema, found deprecated analysis key")
	}

	var payload JSONReport
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Repo != "owner/repo" || payload.PR.Number != 123 {
		t.Fatalf("unexpected top-level metadata: %#v", payload)
	}
	if payload.Level != analysis.RiskLevelHigh || payload.BlastRadius != analysis.BlastRadiusMedium {
		t.Fatalf("unexpected score metadata: %#v", payload)
	}
	if payload.DependencyReviewAvailable {
		t.Fatalf("expected dependency review fallback flag to be false")
	}
	if len(payload.Summary) == 0 || len(payload.Changes) != 1 {
		t.Fatalf("expected summary and detailed changes in payload: %#v", payload)
	}
	if len(payload.Targets) != 1 || payload.Targets[0].Target.DisplayName != "root" {
		t.Fatalf("expected stable targets array, got %#v", payload.Targets)
	}
	if len(payload.RecommendedActions) != 1 || payload.RecommendedActions[0] != analysis.ActionReviewChangelog {
		t.Fatalf("unexpected recommended actions: %#v", payload.RecommendedActions)
	}
}
