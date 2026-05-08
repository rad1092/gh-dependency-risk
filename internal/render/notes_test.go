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

func TestRenderNonRegistrySourceNoteIsReadable(t *testing.T) {
	report := noteReport()
	report.Analysis.Notes = append(report.Analysis.Notes, analysis.Note{
		Code:       analysis.NoteNonRegistrySource,
		Dependency: "git-lib",
		Detail:     "git:https://github.com/example/git-lib.git#branch=main",
	})

	englishMarkdown, err := Render(report, "markdown", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(englishMarkdown, "git-lib resolves from a non-default package source: git:https://github.com/example/git-lib.git#branch=main") {
		t.Fatalf("expected readable English non-registry note, got %q", englishMarkdown)
	}

	koreanMarkdown, err := Render(report, "markdown", "ko")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(koreanMarkdown, analysis.NoteNonRegistrySource) || !strings.Contains(koreanMarkdown, "git-lib") {
		t.Fatalf("expected localized Korean non-registry note without raw code, got %q", koreanMarkdown)
	}
}

func TestRenderGoFallbackNotesAreReadable(t *testing.T) {
	report := noteReport()
	report.Analysis.Notes = []analysis.Note{
		{Code: analysis.NoteGoReplaceDirective, Dependency: "example.com/lib", Detail: "replace added: example.com/lib => ../lib"},
		{Code: analysis.NoteGoLocalReplace, Dependency: "example.com/lib", Detail: "example.com/lib => ../lib"},
		{Code: analysis.NoteGoPseudoVersion, Dependency: "example.com/pseudo", Detail: "v0.0.0-20240101120000-abcdefabcdef"},
		{Code: analysis.NoteGoChecksumChanged, Detail: "go.sum checksum evidence changed: added=1 removed=0"},
		{Code: analysis.NoteGoDirectiveChanged, Detail: "go 1.22 -> 1.23"},
		{Code: analysis.NoteGoToolchainChanged, Detail: "toolchain go1.22.0 -> go1.23.1"},
	}

	englishMarkdown, err := Render(report, "markdown", "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Go replace directive changed",
		"uses a local replace target",
		"uses a pseudo-version",
		"Go checksum evidence changed",
		"Go language directive changed",
		"Go toolchain directive changed",
	} {
		if !strings.Contains(englishMarkdown, expected) {
			t.Fatalf("expected English Go note %q, got %q", expected, englishMarkdown)
		}
	}

	koreanMarkdown, err := Render(report, "markdown", "ko")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		analysis.NoteGoReplaceDirective,
		analysis.NoteGoLocalReplace,
		analysis.NoteGoPseudoVersion,
		analysis.NoteGoChecksumChanged,
		analysis.NoteGoDirectiveChanged,
		analysis.NoteGoToolchainChanged,
	} {
		if strings.Contains(koreanMarkdown, raw) {
			t.Fatalf("did not expect Korean output to expose raw note code %q, got %q", raw, koreanMarkdown)
		}
	}
	if !strings.Contains(koreanMarkdown, "Go") || !strings.Contains(koreanMarkdown, "example.com/lib") {
		t.Fatalf("expected readable Korean Go notes, got %q", koreanMarkdown)
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
