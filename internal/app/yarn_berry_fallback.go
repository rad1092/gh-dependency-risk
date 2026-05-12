package app

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
	"github.com/rad1092/gh-dependency-risk/internal/npm"
)

const packageManagerYarnBerry = "yarn-berry"

type yarnBerryDirectState struct {
	Dependencies []analysis.LocalDependency
	Entries      map[string]npm.YarnBerryEntry
	Requirements map[string]string
	Unsupported  []npm.YarnBerryUnsupportedEntry
}

func hasYarnBerrySignal(manifests [2]*npm.PackageManifest, dir string, yarnRCPaths map[string]struct{}) bool {
	for _, manifest := range manifests {
		if manifest != nil && npm.IsYarnBerryPackageManager(manifest.PackageManager) {
			return true
		}
	}
	_, ok := yarnRCPaths[yarnRCPathForDir(dir)]
	return ok
}

func isYarnBerryTarget(target analysis.AnalysisTarget) bool {
	return target.PackageManager == packageManagerYarnBerry
}

func loadYarnBerryLocalInputIfModern(ctx context.Context, cache *repoDataCache, baseSHA, headSHA string, target analysis.AnalysisTarget, dependencyReviewAvailable bool) (analysis.LocalInput, bool, error) {
	if dependencyReviewAvailable || path.Base(target.ManifestPath) != "package.json" {
		return analysis.LocalInput{}, false, nil
	}
	if target.PackageManager != "yarn" && target.PackageManager != packageManagerYarnBerry {
		return analysis.LocalInput{}, false, nil
	}
	if strings.TrimSpace(target.LockfilePath) == "" {
		return analysis.LocalInput{}, false, nil
	}

	baseManifest, err := cache.manifest(ctx, baseSHA, target.ManifestPath)
	if err != nil {
		return analysis.LocalInput{}, false, err
	}
	headManifest, err := cache.manifest(ctx, headSHA, target.ManifestPath)
	if err != nil {
		return analysis.LocalInput{}, false, err
	}
	baseLockData, err := cache.file(ctx, baseSHA, target.LockfilePath)
	if err != nil {
		return analysis.LocalInput{}, false, err
	}
	headLockData, err := cache.file(ctx, headSHA, target.LockfilePath)
	if err != nil {
		return analysis.LocalInput{}, false, err
	}

	modern := isYarnBerryTarget(target) ||
		npm.IsYarnBerryPackageManager(baseManifest.PackageManager) ||
		npm.IsYarnBerryPackageManager(headManifest.PackageManager) ||
		npm.LooksLikeYarnBerryLockfile(baseLockData) ||
		npm.LooksLikeYarnBerryLockfile(headLockData)
	if !modern {
		return analysis.LocalInput{}, false, nil
	}

	baseLockfile, err := npm.ParseYarnBerryLockfile(baseLockData)
	if err != nil {
		return analysis.LocalInput{}, true, err
	}
	headLockfile, err := npm.ParseYarnBerryLockfile(headLockData)
	if err != nil {
		return analysis.LocalInput{}, true, err
	}

	yarnRCPath := yarnRCPathForDir(yarnBerryProjectRootDir(target))
	baseYarnRCData, err := cache.file(ctx, baseSHA, yarnRCPath)
	if err != nil {
		return analysis.LocalInput{}, true, err
	}
	headYarnRCData, err := cache.file(ctx, headSHA, yarnRCPath)
	if err != nil {
		return analysis.LocalInput{}, true, err
	}
	baseYarnRC, err := npm.ParseYarnRC(baseYarnRCData)
	if err != nil {
		return analysis.LocalInput{}, true, err
	}
	headYarnRC, err := npm.ParseYarnRC(headYarnRCData)
	if err != nil {
		return analysis.LocalInput{}, true, err
	}

	return buildYarnBerryLocalInput(target, baseManifest, headManifest, baseLockfile, headLockfile, baseYarnRC, headYarnRC, yarnRCPath), true, nil
}

func buildYarnBerryLocalInput(target analysis.AnalysisTarget, baseManifest, headManifest *npm.PackageManifest, baseLockfile, headLockfile npm.YarnBerryLockfile, baseYarnRC, headYarnRC npm.YarnRC, yarnRCPath string) analysis.LocalInput {
	baseState := yarnBerryDirectDependencies(baseManifest, baseLockfile)
	headState := yarnBerryDirectDependencies(headManifest, headLockfile)
	changed := changedYarnBerryDependencies(baseState.Dependencies, headState.Dependencies)
	notes := yarnBerryNotes(target, baseState, headState, baseYarnRC, headYarnRC, changed)

	unsupported := convertYarnBerryUnsupported(target.LockfilePath, baseLockfile.Unsupported, headLockfile.Unsupported, baseState.Unsupported, headState.Unsupported)
	unsupported = append(unsupported, convertYarnBerryUnsupported(yarnRCPath, baseYarnRC.Unsupported, headYarnRC.Unsupported)...)
	return analysis.LocalInput{
		Target:                    target,
		DependencyReviewAvailable: false,
		BaseDependencies:          baseState.Dependencies,
		HeadDependencies:          headState.Dependencies,
		Unsupported:               unsupported,
		Notes:                     notes,
	}
}

func yarnBerryDirectDependencies(manifest *npm.PackageManifest, lockfile npm.YarnBerryLockfile) yarnBerryDirectState {
	state := yarnBerryDirectState{
		Entries:      map[string]npm.YarnBerryEntry{},
		Requirements: map[string]string{},
	}
	if manifest == nil {
		return state
	}
	names := manifest.DirectNames()
	for _, name := range names {
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
			Scope:       yarnBerryScope(scopeText),
		}
		if entryOK {
			dependency.Version = entry.Version
			dependency.Source = entry.Source(requirement)
			state.Entries[name] = entry
		} else {
			dependency.Source = (npm.YarnBerryEntry{}).Source(requirement)
		}
		state.Requirements[name] = requirement
		state.Dependencies = append(state.Dependencies, dependency)
	}
	sort.Slice(state.Dependencies, func(i, j int) bool {
		return state.Dependencies[i].Name < state.Dependencies[j].Name
	})
	return state
}

func yarnBerryScope(scope string) analysis.DependencyScope {
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

func changedYarnBerryDependencies(base, head []analysis.LocalDependency) map[string]struct{} {
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

func yarnBerryNotes(target analysis.AnalysisTarget, baseState, headState yarnBerryDirectState, baseYarnRC, headYarnRC npm.YarnRC, changed map[string]struct{}) []analysis.Note {
	if len(changed) == 0 {
		return nil
	}
	notes := []analysis.Note{{
		Code:   analysis.NoteYarnBerryLockfile,
		Detail: target.LockfilePath,
	}}
	if baseYarnRC.NodeLinker != "" || headYarnRC.NodeLinker != "" {
		notes = append(notes, analysis.Note{
			Code:   analysis.NoteYarnNodeLinker,
			Detail: fmt.Sprintf("nodeLinker %s -> %s", displayYarnValue(baseYarnRC.NodeLinker), displayYarnValue(headYarnRC.NodeLinker)),
		})
	}

	names := make([]string, 0, len(changed))
	for name := range changed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, requirement := yarnBerryChangedEntry(name, baseState, headState)
		protocol, detail := entry.ProtocolSource(requirement)
		switch protocol {
		case "workspace":
			notes = append(notes, analysis.Note{Code: analysis.NoteYarnWorkspaceProtocol, Dependency: name, Detail: detail})
		case "patch":
			notes = append(notes, analysis.Note{Code: analysis.NoteYarnPatchProtocol, Dependency: name, Detail: detail})
		case "portal":
			notes = append(notes, analysis.Note{Code: analysis.NoteYarnPortalProtocol, Dependency: name, Detail: detail})
		case "link":
			notes = append(notes, analysis.Note{Code: analysis.NoteYarnLinkProtocol, Dependency: name, Detail: detail})
		case "file":
			notes = append(notes, analysis.Note{Code: analysis.NoteYarnFileProtocol, Dependency: name, Detail: detail})
		case "git", "github":
			notes = append(notes, analysis.Note{Code: analysis.NoteYarnGitSource, Dependency: name, Detail: detail})
		}
		if checksumChanged(baseState.Entries[name], headState.Entries[name]) {
			notes = append(notes, analysis.Note{
				Code:       analysis.NoteYarnChecksumChanged,
				Dependency: name,
				Detail:     fmt.Sprintf("checksum %s -> %s", displayYarnValue(baseState.Entries[name].Checksum), displayYarnValue(headState.Entries[name].Checksum)),
			})
		}
	}
	return notes
}

func yarnBerryChangedEntry(name string, baseState, headState yarnBerryDirectState) (npm.YarnBerryEntry, string) {
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
	return npm.YarnBerryEntry{}, requirement
}

func checksumChanged(baseEntry, headEntry npm.YarnBerryEntry) bool {
	return (baseEntry.Checksum != "" || headEntry.Checksum != "") && baseEntry.Checksum != headEntry.Checksum
}

func convertYarnBerryUnsupported(manifest string, groups ...[]npm.YarnBerryUnsupportedEntry) []analysis.LocalUnsupportedEntry {
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

func yarnBerryProjectRootDir(target analysis.AnalysisTarget) string {
	if target.Kind == analysis.TargetKindWorkspace {
		return target.WorkspaceRootPath
	}
	return target.Directory()
}

func yarnRCPathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return ".yarnrc.yml"
	}
	return cleaned + "/.yarnrc.yml"
}

func displayYarnValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}
