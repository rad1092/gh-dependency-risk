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

func TestAnalyzeLocalUnknownScopeDoesNotUseAddedRuntimeOrDevDrivers(t *testing.T) {
	result := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		HeadDependencies: []LocalDependency{
			{Name: "twine", Requirement: "^5", Scope: ScopeUnknown},
		},
	})

	if len(result.ChangedDependencies) != 1 || result.ChangedDependencies[0].Name != "twine" {
		t.Fatalf("expected unknown-scope dependency change, got %#v", result.ChangedDependencies)
	}
	if result.Score != 0 || len(result.RiskDrivers) != 0 {
		t.Fatalf("expected unknown scope not to use added runtime/dev scoring, got score=%d drivers=%#v", result.Score, result.RiskDrivers)
	}
}

func TestAnalyzeLocalTransitiveScopeIsNotDirectAndDoesNotScoreAsDirect(t *testing.T) {
	result := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		HeadDependencies: []LocalDependency{
			{Name: "golang.org/x/sys", Requirement: "v0.31.0", Version: "v0.31.0", Scope: ScopeTransitive},
		},
	})

	if len(result.ChangedDependencies) != 1 || result.ChangedDependencies[0].Name != "golang.org/x/sys" {
		t.Fatalf("expected transitive dependency change, got %#v", result.ChangedDependencies)
	}
	change := result.ChangedDependencies[0]
	if change.Direct || change.Scope != ScopeTransitive || change.Score != 0 || len(change.RiskDrivers) != 0 {
		t.Fatalf("expected transitive dependency not to score as direct, got %#v", change)
	}
}

func TestAnalyzeLocalScopeOnlyUpdateChangesDirectness(t *testing.T) {
	result := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		BaseDependencies: []LocalDependency{
			{Name: "example.com/lib", Requirement: "v1.0.0", Version: "v1.0.0", Scope: ScopeRuntime},
		},
		HeadDependencies: []LocalDependency{
			{Name: "example.com/lib", Requirement: "v1.0.0", Version: "v1.0.0", Scope: ScopeTransitive},
		},
	})

	if len(result.ChangedDependencies) != 1 {
		t.Fatalf("expected scope-only update to be reported, got %#v", result.ChangedDependencies)
	}
	change := result.ChangedDependencies[0]
	if change.ChangeType != ChangeUpdated || change.Scope != ScopeTransitive || change.Direct || change.Score != 0 {
		t.Fatalf("expected direct-to-transitive scope update to be conservative, got %#v", change)
	}
}

func TestAnalyzeLocalNonRegistrySourceIsInformational(t *testing.T) {
	withoutSource := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		HeadDependencies: []LocalDependency{
			{Name: "git-lib", Requirement: "^0.1", Version: "0.1.0", Scope: ScopeRuntime},
		},
	})
	withSource := AnalyzeLocalDirectDependencies(LocalInput{
		Target:                    pythonRequirementsTarget(),
		DependencyReviewAvailable: false,
		HeadDependencies: []LocalDependency{
			{Name: "git-lib", Requirement: "^0.1", Version: "0.1.0", Source: "git:https://github.com/example/git-lib.git#main", Scope: ScopeRuntime},
		},
	})

	if withoutSource.Score != withSource.Score {
		t.Fatalf("expected source note not to change score, got without=%d with=%d", withoutSource.Score, withSource.Score)
	}
	if !hasNoteCode(withSource.Notes, NoteNonRegistrySource) {
		t.Fatalf("expected non-registry source note, got %#v", withSource.Notes)
	}
	if !containsAction(withSource.RecommendedActions, ActionValidateSources) {
		t.Fatalf("expected validate-source action, got %#v", withSource.RecommendedActions)
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

func containsAction(actions []string, expected string) bool {
	for _, action := range actions {
		if action == expected {
			return true
		}
	}
	return false
}
