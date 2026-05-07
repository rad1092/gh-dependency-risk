package analysis

import "testing"

func TestAnalyzeLocalUnsupportedEntriesDoNotScore(t *testing.T) {
	result := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		Unsupported: []LocalUnsupportedEntry{
			{Manifest: "requirements.txt", Line: 1, Text: "-r other.txt", Reason: "included requirement files are not supported"},
		},
	})

	if HasMeaningfulChange(result) {
		t.Fatalf("expected unsupported-only input to have no meaningful change, got %#v", result.ChangedDependencies)
	}
	if result.Score != 0 || result.Level != RiskLevelLow {
		t.Fatalf("expected unsupported entries not to affect score, got score=%d level=%s", result.Score, result.Level)
	}
	if !hasNoteCode(result.Notes, NoteUnsupportedDependency) {
		t.Fatalf("expected unsupported note, got %#v", result.Notes)
	}
}

func TestAnalyzeLocalSupportedAndUnsupportedScoresOnlySupportedChange(t *testing.T) {
	result := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		HeadDependencies: []LocalDependency{
			{Name: "requests", Requirement: "==2.32.3", Version: "2.32.3", Scope: ScopeRuntime},
		},
		Unsupported: []LocalUnsupportedEntry{
			{Manifest: "requirements.txt", Line: 2, Text: "-r other.txt", Reason: "included requirement files are not supported"},
		},
	})

	if len(result.ChangedDependencies) != 1 || result.ChangedDependencies[0].Name != "requests" {
		t.Fatalf("expected only supported dependency change, got %#v", result.ChangedDependencies)
	}
	if result.Score != result.ChangedDependencies[0].Score {
		t.Fatalf("expected aggregate score to come only from supported change, got score=%d change=%d", result.Score, result.ChangedDependencies[0].Score)
	}
	if !hasNoteCode(result.Notes, NoteUnsupportedDependency) {
		t.Fatalf("expected unsupported note, got %#v", result.Notes)
	}
}

func pythonRequirementsTarget() AnalysisTarget {
	return AnalysisTarget{
		DisplayName:    "root",
		ManifestPath:   "requirements.txt",
		Kind:           TargetKindRoot,
		PackageManager: "pip",
		Ecosystem:      "pip",
		LocalFallback:  true,
	}
}

func hasNoteCode(notes []Note, expected string) bool {
	for _, note := range notes {
		if note.Code == expected {
			return true
		}
	}
	return false
}
