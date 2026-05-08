package app

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"gh-dep-risk/internal/analysis"
	"gh-dep-risk/internal/npm"
)

const packageManagerBun = "bun"

type bunDirectState struct {
	Dependencies []analysis.LocalDependency
	Entries      map[string]npm.BunEntry
	Requirements map[string]string
	Unsupported  []npm.BunUnsupportedEntry
}

func hasBunSignal(manifests [2]*npm.PackageManifest, dir string, bunLockfilePaths, bunBinaryLockfilePaths map[string]struct{}) bool {
	for _, manifest := range manifests {
		if manifest != nil && npm.IsBunPackageManager(manifest.PackageManager) {
			return true
		}
	}
	if _, ok := bunLockfilePaths[bunLockfilePathForDir(dir)]; ok {
		return true
	}
	_, ok := bunBinaryLockfilePaths[bunBinaryLockfilePathForDir(dir)]
	return ok
}

func isBunTarget(target analysis.AnalysisTarget) bool {
	return target.PackageManager == packageManagerBun
}

func shouldUseBunLocalFallback(target analysis.AnalysisTarget, dependencyReviewAvailable bool) bool {
	return !dependencyReviewAvailable && isBunTarget(target)
}

func loadBunLocalInput(ctx context.Context, cache *repoDataCache, baseSHA, headSHA string, target analysis.AnalysisTarget) (analysis.LocalInput, error) {
	if path.Base(target.ManifestPath) != "package.json" {
		return analysis.LocalInput{}, nil
	}
	if strings.TrimSpace(target.LockfilePath) == "" {
		return analysis.LocalInput{
			Target:                    target,
			DependencyReviewAvailable: false,
			Unsupported: []analysis.LocalUnsupportedEntry{{
				Manifest: target.ManifestPath,
				Reason:   "Bun local fallback requires a text bun.lock file",
			}},
		}, nil
	}
	if path.Base(target.LockfilePath) == "bun.lockb" {
		return analysis.LocalInput{
			Target:                    target,
			DependencyReviewAvailable: false,
			Unsupported: []analysis.LocalUnsupportedEntry{{
				Manifest: target.LockfilePath,
				Reason:   "binary bun.lockb is not supported by local fallback",
			}},
			Notes: []analysis.Note{{Code: analysis.NoteBunBinaryLockfile, Detail: target.LockfilePath}},
		}, nil
	}

	baseManifest, err := cache.manifest(ctx, baseSHA, target.ManifestPath)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	headManifest, err := cache.manifest(ctx, headSHA, target.ManifestPath)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	baseLockData, err := cache.file(ctx, baseSHA, target.LockfilePath)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	headLockData, err := cache.file(ctx, headSHA, target.LockfilePath)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	baseLockfile, err := npm.ParseBunLockfile(baseLockData)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	headLockfile, err := npm.ParseBunLockfile(headLockData)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	return buildBunLocalInput(target, baseManifest, headManifest, baseLockfile, headLockfile), nil
}

func buildBunLocalInput(target analysis.AnalysisTarget, baseManifest, headManifest *npm.PackageManifest, baseLockfile, headLockfile npm.BunLockfile) analysis.LocalInput {
	baseState := bunDirectDependencies(baseManifest, baseLockfile)
	headState := bunDirectDependencies(headManifest, headLockfile)
	changed := changedBunDependencies(baseState.Dependencies, headState.Dependencies)
	unsupported := convertBunUnsupported(target.LockfilePath, baseLockfile.Unsupported, headLockfile.Unsupported, baseState.Unsupported, headState.Unsupported)
	return analysis.LocalInput{
		Target:                    target,
		DependencyReviewAvailable: false,
		BaseDependencies:          baseState.Dependencies,
		HeadDependencies:          headState.Dependencies,
		Unsupported:               unsupported,
		Notes:                     bunNotes(target, baseState, headState, changed),
	}
}

func bunDirectDependencies(manifest *npm.PackageManifest, lockfile npm.BunLockfile) bunDirectState {
	state := bunDirectState{
		Entries:      map[string]npm.BunEntry{},
		Requirements: map[string]string{},
	}
	if manifest == nil {
		return state
	}
	for _, name := range manifest.DirectNames() {
		scopeText, ok := manifest.Scope(name)
		if !ok {
			continue
		}
		requirement := manifest.Requirement(name)
		entry, entryOK, unsupported := lockfile.ResolveDirectEntry(name, requirement)
		state.Unsupported = append(state.Unsupported, unsupported...)
		dependency := analysis.LocalDependency{
			Name:        name,
			Requirement: requirement,
			Scope:       bunScope(scopeText),
		}
		if entryOK {
			dependency.Version = entry.Version
			dependency.Source = entry.SourceForRequirement(requirement)
			state.Entries[name] = entry
		} else {
			dependency.Source = (npm.BunEntry{}).SourceForRequirement(requirement)
		}
		state.Requirements[name] = requirement
		state.Dependencies = append(state.Dependencies, dependency)
	}
	sort.Slice(state.Dependencies, func(i, j int) bool {
		return state.Dependencies[i].Name < state.Dependencies[j].Name
	})
	return state
}

func bunScope(scope string) analysis.DependencyScope {
	switch scope {
	case "runtime":
		return analysis.ScopeRuntime
	case "dev":
		return analysis.ScopeDev
	case "optional":
		return analysis.ScopeOptional
	default:
		return analysis.ScopeUnknown
	}
}

func changedBunDependencies(base, head []analysis.LocalDependency) map[string]struct{} {
	baseMap := map[string]analysis.LocalDependency{}
	headMap := map[string]analysis.LocalDependency{}
	for _, dependency := range base {
		baseMap[dependency.Name] = dependency
	}
	for _, dependency := range head {
		headMap[dependency.Name] = dependency
	}
	seen := map[string]struct{}{}
	for name := range baseMap {
		seen[name] = struct{}{}
	}
	for name := range headMap {
		seen[name] = struct{}{}
	}
	changed := map[string]struct{}{}
	for name := range seen {
		before, beforeOK := baseMap[name]
		after, afterOK := headMap[name]
		if !beforeOK || !afterOK ||
			before.Requirement != after.Requirement ||
			before.Version != after.Version ||
			before.Source != after.Source ||
			before.Scope != after.Scope {
			changed[name] = struct{}{}
		}
	}
	return changed
}

func bunNotes(target analysis.AnalysisTarget, baseState, headState bunDirectState, changed map[string]struct{}) []analysis.Note {
	if len(changed) == 0 {
		return nil
	}
	notes := []analysis.Note{{Code: analysis.NoteBunLockfile, Detail: target.LockfilePath}}
	names := make([]string, 0, len(changed))
	for name := range changed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, requirement := bunChangedEntry(name, baseState, headState)
		protocol, detail := entry.ProtocolSource(requirement)
		if protocol == "workspace" {
			notes = append(notes, analysis.Note{Code: analysis.NoteBunWorkspaceProtocol, Dependency: name, Detail: detail})
		}
		if bunChecksumChanged(baseState.Entries[name], headState.Entries[name]) {
			notes = append(notes, analysis.Note{
				Code:       analysis.NoteBunChecksumChanged,
				Dependency: name,
				Detail:     fmt.Sprintf("checksum %s -> %s", displayBunValue(baseState.Entries[name].Checksum), displayBunValue(headState.Entries[name].Checksum)),
			})
		}
	}
	return notes
}

func bunChangedEntry(name string, baseState, headState bunDirectState) (npm.BunEntry, string) {
	if entry, ok := headState.Entries[name]; ok {
		return entry, headState.Requirements[name]
	}
	if entry, ok := baseState.Entries[name]; ok {
		return entry, baseState.Requirements[name]
	}
	requirement := headState.Requirements[name]
	if requirement == "" {
		requirement = baseState.Requirements[name]
	}
	return npm.BunEntry{}, requirement
}

func bunChecksumChanged(baseEntry, headEntry npm.BunEntry) bool {
	return (baseEntry.Checksum != "" || headEntry.Checksum != "") && baseEntry.Checksum != headEntry.Checksum
}

func convertBunUnsupported(manifest string, groups ...[]npm.BunUnsupportedEntry) []analysis.LocalUnsupportedEntry {
	entries := make([]analysis.LocalUnsupportedEntry, 0)
	for _, group := range groups {
		for _, unsupported := range group {
			entries = append(entries, analysis.LocalUnsupportedEntry{
				Manifest: manifest,
				Text:     unsupported.Descriptor,
				Reason:   unsupported.Reason,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Text == entries[j].Text {
			return entries[i].Reason < entries[j].Reason
		}
		return entries[i].Text < entries[j].Text
	})
	return entries
}

func displayBunValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}
