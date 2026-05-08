package gomod

import (
	"testing"

	"gh-dep-risk/internal/analysis"
)

func TestBuildLocalInputRequireAddedUpdatedRemovedAndIndirect(t *testing.T) {
	target := goTarget()
	input, err := BuildLocalInput(target,
		[]byte(`module example.com/app
go 1.22
require example.com/old v1.0.0
require example.com/update v0.9.0
`),
		[]byte(`module example.com/app
go 1.22
require example.com/new v1.2.0
require example.com/update v1.0.0
require example.com/indirect v0.1.0 // indirect
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := analysis.AnalyzeLocalDirectDependencies(input)
	assertChange(t, result, "example.com/new", analysis.ChangeAdded, analysis.ScopeRuntime, true)
	update := assertChange(t, result, "example.com/update", analysis.ChangeUpdated, analysis.ScopeRuntime, true)
	if !contains(update.RiskDrivers, analysis.DriverMajorVersionBump) {
		t.Fatalf("expected major bump driver, got %#v", update.RiskDrivers)
	}
	indirect := assertChange(t, result, "example.com/indirect", analysis.ChangeAdded, analysis.ScopeTransitive, false)
	if indirect.Score != 0 || len(indirect.RiskDrivers) != 0 {
		t.Fatalf("expected indirect addition not to use direct scoring, got %#v", indirect)
	}
	assertChange(t, result, "example.com/old", analysis.ChangeRemoved, analysis.ScopeRuntime, true)
}

func TestBuildLocalInputDirectToIndirectScopeOnlyUpdate(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte(`module example.com/app
go 1.22
require example.com/lib v1.0.0
`),
		[]byte(`module example.com/app
go 1.22
require example.com/lib v1.0.0 // indirect
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := analysis.AnalyzeLocalDirectDependencies(input)
	change := assertChange(t, result, "example.com/lib", analysis.ChangeUpdated, analysis.ScopeTransitive, false)
	if change.Score != 0 || len(change.RiskDrivers) != 0 {
		t.Fatalf("expected direct to indirect update to stay conservative, got %#v", change)
	}
}

func TestBuildLocalInputIndirectToDirectScopeOnlyUpdate(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte(`module example.com/app
go 1.22
require example.com/lib v1.0.0 // indirect
`),
		[]byte(`module example.com/app
go 1.22
require example.com/lib v1.0.0
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := analysis.AnalyzeLocalDirectDependencies(input)
	change := assertChange(t, result, "example.com/lib", analysis.ChangeUpdated, analysis.ScopeRuntime, true)
	if change.Score != 0 || contains(change.RiskDrivers, analysis.DriverMajorVersionBump) {
		t.Fatalf("expected indirect to direct update without version change not to score as major bump, got %#v", change)
	}
}

func TestBuildLocalInputReplaceOnlyIsMeaningful(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\n"),
		[]byte(`module example.com/app
go 1.22
replace example.com/local => ../local/path
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := analysis.AnalyzeLocalDirectDependencies(input)
	change := assertChange(t, result, "example.com/local", analysis.ChangeAdded, analysis.ScopeUnknown, true)
	if change.Score != 0 {
		t.Fatalf("expected conservative replace-only score, got %#v", change)
	}
	if !hasNote(result.Notes, analysis.NoteGoReplaceDirective) || !hasNote(result.Notes, analysis.NoteGoLocalReplace) || !hasNote(result.Notes, analysis.NoteNonRegistrySource) {
		t.Fatalf("expected replace/local/non-registry notes, got %#v", result.Notes)
	}
	if !contains(result.RecommendedActions, analysis.ActionValidateSources) {
		t.Fatalf("expected validate source action, got %#v", result.RecommendedActions)
	}
}

func TestBuildLocalInputRemoteReplaceChanged(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte(`module example.com/app
go 1.22
replace example.com/lib => example.com/fork/lib v1.0.0
`),
		[]byte(`module example.com/app
go 1.22
replace example.com/lib => example.com/fork/lib v1.1.0
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.AnalyzeLocalDirectDependencies(input)
	change := assertChange(t, result, "example.com/lib", analysis.ChangeUpdated, analysis.ScopeUnknown, true)
	if change.Resolved != "replace:example.com/fork/lib@v1.1.0" {
		t.Fatalf("expected remote replace source, got %#v", change)
	}
	if !hasNote(result.Notes, analysis.NoteGoReplaceDirective) {
		t.Fatalf("expected replace note, got %#v", result.Notes)
	}
}

func TestBuildLocalInputVersionSpecificReplaceOnlyUsesStableIdentity(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\n"),
		[]byte(`module example.com/app
go 1.22
replace example.com/lib v1.0.0 => example.com/fork/lib v1.0.1
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := analysis.AnalyzeLocalDirectDependencies(input)
	change := assertChange(t, result, "example.com/lib@v1.0.0", analysis.ChangeAdded, analysis.ScopeUnknown, true)
	if change.Resolved != "replace:example.com/fork/lib@v1.0.1" {
		t.Fatalf("expected version-specific replace source, got %#v", change)
	}
	note := findNote(t, result.Notes, analysis.NoteGoReplaceDirective, "example.com/lib@v1.0.0")
	if note.Detail != "replace added: example.com/lib@v1.0.0 => example.com/fork/lib@v1.0.1" {
		t.Fatalf("expected deterministic version-specific replace note, got %#v", note)
	}
}

func TestBuildLocalInputMultipleVersionSpecificReplacesDoNotCollapse(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\n"),
		[]byte(`module example.com/app
go 1.22
replace (
	example.com/lib v1.0.0 => example.com/fork/lib v1.0.1
	example.com/lib v2.0.0 => example.com/fork/lib/v2 v2.0.1
)
`),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	result := analysis.AnalyzeLocalDirectDependencies(input)
	assertChange(t, result, "example.com/lib@v1.0.0", analysis.ChangeAdded, analysis.ScopeUnknown, true)
	assertChange(t, result, "example.com/lib@v2.0.0", analysis.ChangeAdded, analysis.ScopeUnknown, true)
	findNote(t, result.Notes, analysis.NoteGoReplaceDirective, "example.com/lib@v1.0.0")
	findNote(t, result.Notes, analysis.NoteGoReplaceDirective, "example.com/lib@v2.0.0")
}

func TestBuildLocalInputPseudoVersionAndDirectiveNotes(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\ntoolchain go1.22.0\n"),
		[]byte(`module example.com/app
go 1.23
toolchain go1.23.1
require example.com/pseudo v0.0.0-20240101120000-abcdefabcdef
`),
		[]byte("example.com/pseudo v0.0.0-20240101120000-abcdefabcdef h1:old\n"),
		[]byte("example.com/pseudo v0.0.0-20240101120000-abcdefabcdef h1:new\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.AnalyzeLocalDirectDependencies(input)
	assertChange(t, result, "example.com/pseudo", analysis.ChangeAdded, analysis.ScopeRuntime, true)
	for _, code := range []string{
		analysis.NoteGoPseudoVersion,
		analysis.NoteGoDirectiveChanged,
		analysis.NoteGoToolchainChanged,
		analysis.NoteGoChecksumChanged,
	} {
		if !hasNote(result.Notes, code) {
			t.Fatalf("expected note %s, got %#v", code, result.Notes)
		}
	}
}

func TestBuildLocalInputDirectiveAndChecksumOnlyHasNoMeaningfulChange(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\n"),
		[]byte("module example.com/app\ngo 1.23\n"),
		[]byte("example.com/lib v1.0.0 h1:old\n"),
		[]byte("example.com/lib v1.0.0 h1:new\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{analysis.NoteGoDirectiveChanged, analysis.NoteGoChecksumChanged} {
		if hasNote(input.Notes, code) {
			t.Fatalf("did not expect supplemental note %s without meaningful dependency change, got %#v", code, input.Notes)
		}
	}
	result := analysis.AnalyzeLocalDirectDependencies(input)
	if analysis.HasMeaningfulChange(result) {
		t.Fatalf("expected directive/checksum-only changes to have no meaningful dependency change, got %#v", result.ChangedDependencies)
	}
}

func TestBuildLocalInputToolchainOnlyHasNoSupplementalNote(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\ntoolchain go1.22.0\n"),
		[]byte("module example.com/app\ngo 1.22\ntoolchain go1.23.1\n"),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if hasNote(input.Notes, analysis.NoteGoToolchainChanged) {
		t.Fatalf("did not expect toolchain note without meaningful dependency change, got %#v", input.Notes)
	}
}

func TestBuildLocalInputGoSumUnsupportedOnlyDoesNotScore(t *testing.T) {
	input, err := BuildLocalInput(goTarget(),
		[]byte("module example.com/app\ngo 1.22\n"),
		[]byte("module example.com/app\ngo 1.22\n"),
		nil,
		[]byte("not-a-valid-checksum-line\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.AnalyzeLocalDirectDependencies(input)
	if analysis.HasMeaningfulChange(result) || result.Score != 0 {
		t.Fatalf("expected unsupported go.sum-only input not to score, got %#v", result)
	}
	if !hasNote(result.Notes, analysis.NoteUnsupportedDependency) {
		t.Fatalf("expected unsupported note, got %#v", result.Notes)
	}
}

func goTarget() analysis.AnalysisTarget {
	return analysis.AnalysisTarget{
		DisplayName:    "root",
		ManifestPath:   "go.mod",
		LockfilePath:   "go.sum",
		Kind:           analysis.TargetKindRoot,
		PackageManager: "go",
		Ecosystem:      "go-modules",
		LocalFallback:  true,
	}
}

func assertChange(t *testing.T, result analysis.AnalysisResult, name string, changeType analysis.ChangeType, scope analysis.DependencyScope, direct bool) analysis.DependencyChange {
	t.Helper()
	for _, change := range result.ChangedDependencies {
		if change.Name != name {
			continue
		}
		if change.ChangeType != changeType || change.Scope != scope || change.Direct != direct {
			t.Fatalf("unexpected change for %s: %#v", name, change)
		}
		return change
	}
	t.Fatalf("expected change %s, got %#v", name, result.ChangedDependencies)
	return analysis.DependencyChange{}
}

func hasNote(notes []analysis.Note, code string) bool {
	for _, note := range notes {
		if note.Code == code {
			return true
		}
	}
	return false
}

func findNote(t *testing.T, notes []analysis.Note, code, dependency string) analysis.Note {
	t.Helper()
	for _, note := range notes {
		if note.Code == code && note.Dependency == dependency {
			return note
		}
	}
	t.Fatalf("expected note %s for %s, got %#v", code, dependency, notes)
	return analysis.Note{}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
