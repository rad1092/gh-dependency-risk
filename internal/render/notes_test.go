package render

import (
	"strings"
	"testing"

	"gh-dep-risk/internal/analysis"
)

func TestRenderNotesUseReadableLocalFallbackLanguage(t *testing.T) {
	report := noteReport()

	englishHuman, err := Render(report, "human", "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Dependency review API was unavailable, so local fallback analysis was used.",
		"Some dependency entries were not analyzed by local fallback: requirements.txt:1: included requirement files are not supported (-r other.txt)",
	} {
		if !strings.Contains(englishHuman, expected) {
			t.Fatalf("expected English human output to contain %q, got %q", expected, englishHuman)
		}
	}

	englishMarkdown, err := Render(report, "markdown", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(englishMarkdown, "local fallback analysis") || !strings.Contains(englishMarkdown, "Some dependency entries were not analyzed by local fallback") {
		t.Fatalf("expected English markdown output to use readable local fallback notes, got %q", englishMarkdown)
	}
}

func TestRenderKoreanNotesDoNotExposeRawCodes(t *testing.T) {
	report := noteReport()

	koreanHuman, err := Render(report, "human", "ko")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Dependency Review API를 사용할 수 없어 local fallback 분석을 사용했습니다.",
		"일부 의존성 항목은 local fallback에서 분석되지 않았습니다: requirements.txt:1: included requirement files are not supported (-r other.txt)",
	} {
		if !strings.Contains(koreanHuman, expected) {
			t.Fatalf("expected Korean human output to contain %q, got %q", expected, koreanHuman)
		}
	}
	for _, unexpected := range []string{
		analysis.NoteUnsupportedDependency,
		"lockfile 기반 fallback",
	} {
		if strings.Contains(koreanHuman, unexpected) {
			t.Fatalf("did not expect Korean human output to contain %q, got %q", unexpected, koreanHuman)
		}
	}

	koreanMarkdown, err := Render(report, "markdown", "ko")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(koreanMarkdown, "일부 의존성 항목은 local fallback에서 분석되지 않았습니다") || strings.Contains(koreanMarkdown, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected Korean markdown output to localize unsupported note, got %q", koreanMarkdown)
	}
}

func noteReport() Report {
	return Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number: 123,
			URL:    "https://github.com/owner/repo/pull/123",
			Title:  "Update Python dependencies",
		},
		Analysis: analysis.AnalysisResult{
			DependencyReviewAvailable: false,
			Score:                     20,
			Level:                     analysis.RiskLevelMedium,
			BlastRadius:               analysis.BlastRadiusLow,
			ChangedDependencies: []analysis.DependencyChange{
				{
					Name:        "requests",
					Manifest:    "requirements.txt",
					Target:      "root",
					ChangeType:  analysis.ChangeAdded,
					Scope:       analysis.ScopeRuntime,
					Direct:      true,
					Score:       20,
					Level:       analysis.RiskLevelMedium,
					ToVersion:   "2.32.3",
					RiskDrivers: []string{analysis.DriverAddedDirectRuntime},
				},
			},
			RiskDrivers:        []string{analysis.DriverAddedDirectRuntime},
			RecommendedActions: []string{analysis.ActionRunTargetedTests},
			Notes: []analysis.Note{
				{Code: analysis.NoteDependencyReviewFallback},
				{Code: analysis.NoteUnsupportedDependency, Detail: "requirements.txt:1: included requirement files are not supported (-r other.txt)"},
			},
		},
	}
}
