package render

import (
	"strings"
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
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

func TestRenderYarnBerryFallbackNotesAreReadable(t *testing.T) {
	report := noteReport()
	report.Analysis.Notes = []analysis.Note{
		{Code: analysis.NoteYarnBerryLockfile, Detail: "yarn.lock"},
		{Code: analysis.NoteYarnNodeLinker, Detail: "nodeLinker node-modules -> pnp"},
		{Code: analysis.NoteYarnWorkspaceProtocol, Dependency: "workspace-lib", Detail: "workspace:*"},
		{Code: analysis.NoteYarnPatchProtocol, Dependency: "patched", Detail: "patch:patched@npm%3A1.0.0#./patches/patched.patch"},
		{Code: analysis.NoteYarnPortalProtocol, Dependency: "portal-lib", Detail: "portal:../portal-lib"},
		{Code: analysis.NoteYarnLinkProtocol, Dependency: "linked-lib", Detail: "link:../linked-lib"},
		{Code: analysis.NoteYarnFileProtocol, Dependency: "file-lib", Detail: "file:../file-lib.tgz"},
		{Code: analysis.NoteYarnGitSource, Dependency: "git-lib", Detail: "git:https://github.com/acme/git-lib.git"},
		{Code: analysis.NoteYarnChecksumChanged, Dependency: "left-pad", Detail: "checksum 10c0 -> 20c0"},
	}

	englishMarkdown, err := Render(report, "markdown", "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Yarn Berry lockfile fallback was used",
		"Yarn nodeLinker setting was detected",
		"uses the workspace protocol",
		"uses the patch protocol",
		"uses the portal protocol",
		"uses the link protocol",
		"uses the file protocol",
		"uses a git source",
		"Yarn checksum evidence changed",
	} {
		if !strings.Contains(englishMarkdown, expected) {
			t.Fatalf("expected English Yarn note %q, got %q", expected, englishMarkdown)
		}
	}

	koreanMarkdown, err := Render(report, "markdown", "ko")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		analysis.NoteYarnBerryLockfile,
		analysis.NoteYarnNodeLinker,
		analysis.NoteYarnWorkspaceProtocol,
		analysis.NoteYarnPatchProtocol,
		analysis.NoteYarnPortalProtocol,
		analysis.NoteYarnLinkProtocol,
		analysis.NoteYarnFileProtocol,
		analysis.NoteYarnGitSource,
		analysis.NoteYarnChecksumChanged,
	} {
		if strings.Contains(koreanMarkdown, raw) {
			t.Fatalf("did not expect Korean output to expose raw note code %q, got %q", raw, koreanMarkdown)
		}
	}
	if !strings.Contains(koreanMarkdown, "Yarn") || !strings.Contains(koreanMarkdown, "workspace-lib") {
		t.Fatalf("expected readable Korean Yarn notes, got %q", koreanMarkdown)
	}
}

func TestRenderBunFallbackNotesAreReadable(t *testing.T) {
	report := noteReport()
	report.Analysis.Notes = []analysis.Note{
		{Code: analysis.NoteBunLockfile, Detail: "bun.lock"},
		{Code: analysis.NoteBunWorkspaceProtocol, Dependency: "workspace-lib", Detail: "workspace:*"},
		{Code: analysis.NoteBunChecksumChanged, Dependency: "left-pad", Detail: "checksum old -> new"},
		{Code: analysis.NoteBunBinaryLockfile, Detail: "bun.lockb"},
	}

	englishMarkdown, err := Render(report, "markdown", "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Bun lockfile fallback was used",
		"uses the workspace protocol",
		"Bun checksum evidence changed",
		"Bun binary lockfile is unsupported",
	} {
		if !strings.Contains(englishMarkdown, expected) {
			t.Fatalf("expected English Bun note %q, got %q", expected, englishMarkdown)
		}
	}

	koreanMarkdown, err := Render(report, "markdown", "ko")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		analysis.NoteBunLockfile,
		analysis.NoteBunWorkspaceProtocol,
		analysis.NoteBunChecksumChanged,
		analysis.NoteBunBinaryLockfile,
	} {
		if strings.Contains(koreanMarkdown, raw) {
			t.Fatalf("did not expect Korean output to expose raw note code %q, got %q", raw, koreanMarkdown)
		}
	}
	if !strings.Contains(koreanMarkdown, "Bun") || !strings.Contains(koreanMarkdown, "workspace-lib") {
		t.Fatalf("expected readable Korean Bun notes, got %q", koreanMarkdown)
	}
	if !strings.Contains(koreanMarkdown, "지원되지 않습니다") || !strings.Contains(koreanMarkdown, "checksum 근거") {
		t.Fatalf("expected readable Korean Bun note text, got %q", koreanMarkdown)
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
