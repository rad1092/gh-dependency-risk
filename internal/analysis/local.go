package analysis

import (
	"fmt"
	"sort"
	"strings"
)

type LocalDependency struct {
	Name        string
	Requirement string
	Version     string
	Source      string
	Scope       DependencyScope
}

type LocalUnsupportedEntry struct {
	Manifest string
	Line     int
	Text     string
	Reason   string
}

type LocalInput struct {
	Target                    AnalysisTarget
	DependencyReviewAvailable bool
	BaseDependencies          []LocalDependency
	HeadDependencies          []LocalDependency
	Unsupported               []LocalUnsupportedEntry
	Notes                     []Note
}

func AnalyzeLocalDirectDependencies(input LocalInput) AnalysisResult {
	base := localDependencyMap(input.BaseDependencies)
	head := localDependencyMap(input.HeadDependencies)
	names := localDependencyNames(base, head)

	changes := make([]DependencyChange, 0)
	notes := append([]Note(nil), input.Notes...)
	for _, name := range names {
		before, beforeOK := base[name]
		after, afterOK := head[name]
		if beforeOK && afterOK && !localDependencyChanged(before, after) {
			continue
		}

		change := localDependencyChange(input.Target, before, beforeOK, after, afterOK)
		change.Score, change.RiskDrivers = scoreLocalDependencyChange(change)
		change.Level = LevelForScore(change.Score)
		if afterOK && after.Source != "" {
			notes = append(notes, Note{
				Code:       NoteNonRegistrySource,
				Dependency: after.Name,
				Detail:     after.Source,
			})
		}
		changes = append(changes, change)
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Score == changes[j].Score {
			return changes[i].Name < changes[j].Name
		}
		return changes[i].Score > changes[j].Score
	})

	if !input.DependencyReviewAvailable {
		notes = append(notes, Note{Code: NoteDependencyReviewFallback})
	}
	if len(input.Unsupported) > 0 {
		notes = append(notes, Note{Code: NoteUnsupportedDependency, Detail: summarizeUnsupportedEntries(input.Unsupported)})
	}

	score := aggregateScore(changes)
	return AnalysisResult{
		DependencyReviewAvailable: input.DependencyReviewAvailable,
		Score:                     score,
		Level:                     LevelForScore(score),
		BlastRadius:               deriveBlastRadius(changes, 0),
		ChangedDependencies:       changes,
		RiskDrivers:               collectDrivers(changes),
		RecommendedActions:        recommendedActions(changes, notes),
		QuickCommands:             quickCommands(input.Target, changes),
		Notes:                     uniqueNotes(notes),
	}
}

func localDependencyMap(dependencies []LocalDependency) map[string]LocalDependency {
	result := map[string]LocalDependency{}
	for _, dependency := range dependencies {
		name := strings.TrimSpace(dependency.Name)
		if name == "" {
			continue
		}
		dependency.Name = name
		result[name] = dependency
	}
	return result
}

func localDependencyNames(base, head map[string]LocalDependency) []string {
	seen := map[string]struct{}{}
	for name := range base {
		seen[name] = struct{}{}
	}
	for name := range head {
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func localDependencyChanged(before, after LocalDependency) bool {
	return before.Requirement != after.Requirement || before.Version != after.Version || before.Source != after.Source
}

func localDependencyChange(target AnalysisTarget, before LocalDependency, beforeOK bool, after LocalDependency, afterOK bool) DependencyChange {
	change := DependencyChange{
		Manifest: target.ManifestPath,
		Target:   normalizeTargetDisplayName(target),
	}
	switch {
	case !beforeOK && afterOK:
		change.Name = after.Name
		change.ChangeType = ChangeAdded
		change.Scope = normalizeLocalScope(after.Scope)
		change.Direct = localDependencyDirect(after)
	case beforeOK && !afterOK:
		change.Name = before.Name
		change.ChangeType = ChangeRemoved
		change.Scope = normalizeLocalScope(before.Scope)
		change.Direct = localDependencyDirect(before)
	default:
		change.Name = after.Name
		change.ChangeType = ChangeUpdated
		change.Scope = normalizeLocalScope(after.Scope)
		change.Direct = localDependencyDirect(after)
		if !change.Direct {
			change.Direct = localDependencyDirect(before)
		}
	}
	if beforeOK {
		change.FromRequirement = before.Requirement
		change.FromVersion = before.Version
	}
	if afterOK {
		change.ToRequirement = after.Requirement
		change.ToVersion = after.Version
	}
	change.Resolved = after.Source
	if change.Resolved == "" {
		change.Resolved = before.Source
	}
	return change
}

func normalizeLocalScope(scope DependencyScope) DependencyScope {
	switch scope {
	case ScopeRuntime, ScopeOptional, ScopeDev, ScopeTransitive, ScopeUnknown:
		return scope
	default:
		return ScopeUnknown
	}
}

func localDependencyDirect(dependency LocalDependency) bool {
	return normalizeLocalScope(dependency.Scope) != ScopeTransitive
}

func scoreLocalDependencyChange(change DependencyChange) (int, []string) {
	score := 0
	drivers := make([]string, 0, 2)
	add := func(condition bool, points int, driver string) {
		if !condition {
			return
		}
		score += points
		drivers = append(drivers, driver)
	}

	add(change.ChangeType == ChangeAdded && change.Direct && (change.Scope == ScopeRuntime || change.Scope == ScopeOptional), scoreAddedDirectRuntime, DriverAddedDirectRuntime)
	add(change.ChangeType == ChangeAdded && change.Direct && change.Scope == ScopeDev, scoreAddedDirectDev, DriverAddedDirectDev)
	add(change.Direct && isMajorBump(change.FromVersion, change.ToVersion, change.FromRequirement, change.ToRequirement), scoreMajorVersionBump, DriverMajorVersionBump)

	if score > changeScoreCap {
		score = changeScoreCap
	}
	return score, uniqueStrings(drivers)
}

func summarizeUnsupportedEntries(entries []LocalUnsupportedEntry) string {
	if len(entries) == 0 {
		return ""
	}
	first := entries[0]
	location := first.Manifest
	if first.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, first.Line)
	}
	detail := fmt.Sprintf("%s: %s", location, first.Reason)
	if strings.TrimSpace(first.Text) != "" {
		detail = fmt.Sprintf("%s (%s)", detail, first.Text)
	}
	if len(entries) == 1 {
		return detail
	}
	return fmt.Sprintf("%d unsupported dependency entries; first: %s", len(entries), detail)
}
