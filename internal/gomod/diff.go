package gomod

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gh-dep-risk/internal/analysis"
)

var pseudoVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\.0)?\.\d{14}-[0-9a-f]{12,}$|^v\d+\.\d+\.\d+-\d{14}-[0-9a-f]{12,}$`)

func BuildLocalInput(target analysis.AnalysisTarget, baseModData, headModData, baseSumData, headSumData []byte) (analysis.LocalInput, error) {
	baseManifest, err := ParseModFile(baseModData)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	headManifest, err := ParseModFile(headModData)
	if err != nil {
		return analysis.LocalInput{}, err
	}

	baseSum := ParseSumFile(baseSumData)
	headSum := ParseSumFile(headSumData)

	baseDependencies := dependenciesFromManifest(baseManifest)
	headDependencies := dependenciesFromManifest(headManifest)
	notes := diffNotes(target, baseManifest, headManifest, baseSum, headSum, baseDependencies, headDependencies)
	unsupported := convertUnsupported(target.LockfilePath, baseSum.Unsupported, headSum.Unsupported)

	return analysis.LocalInput{
		Target:                    target,
		DependencyReviewAvailable: false,
		BaseDependencies:          baseDependencies,
		HeadDependencies:          headDependencies,
		Unsupported:               unsupported,
		Notes:                     notes,
	}, nil
}

func dependenciesFromManifest(manifest Manifest) []analysis.LocalDependency {
	dependencies := map[string]analysis.LocalDependency{}
	for _, requirement := range manifest.Requirements {
		scope := analysis.ScopeRuntime
		if requirement.Indirect {
			scope = analysis.ScopeTransitive
		}
		dependencies[requirement.Path] = analysis.LocalDependency{
			Name:        requirement.Path,
			Requirement: requirement.Version,
			Version:     requirement.Version,
			Scope:       scope,
		}
	}

	for _, replacement := range manifest.Replacements {
		key := replacementDependencyKey(replacement, dependencies)
		dependency := dependencies[key]
		if dependency.Name == "" {
			dependency = analysis.LocalDependency{
				Name:  replacementIdentity(replacement),
				Scope: analysis.ScopeUnknown,
			}
			if replacement.OldVersion != "" {
				dependency.Requirement = replacement.OldVersion
				dependency.Version = replacement.OldVersion
			}
			if replacement.NewVersion != "" {
				dependency.Version = replacement.NewVersion
			}
		}
		dependency.Source = replacementSource(replacement)
		dependencies[key] = dependency
	}

	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]analysis.LocalDependency, 0, len(names))
	for _, name := range names {
		result = append(result, dependencies[name])
	}
	return result
}

func replacementSource(replacement Replacement) string {
	target := replacement.NewPath
	if replacement.NewVersion != "" {
		target += "@" + replacement.NewVersion
	}
	if replacement.Local {
		return "local-replace:" + target
	}
	return "replace:" + target
}

func replacementDependencyKey(replacement Replacement, dependencies map[string]analysis.LocalDependency) string {
	if dependency, ok := dependencies[replacement.OldPath]; ok {
		if replacement.OldVersion == "" || dependency.Version == replacement.OldVersion || dependency.Requirement == replacement.OldVersion {
			return replacement.OldPath
		}
	}
	return replacementIdentity(replacement)
}

func replacementIdentity(replacement Replacement) string {
	if strings.TrimSpace(replacement.OldVersion) == "" {
		return replacement.OldPath
	}
	return replacement.OldPath + "@" + replacement.OldVersion
}

func diffNotes(target analysis.AnalysisTarget, baseManifest, headManifest Manifest, baseSum, headSum SumFile, baseDependencies, headDependencies []analysis.LocalDependency) []analysis.Note {
	notes := make([]analysis.Note, 0)
	changed := changedDependencySet(baseDependencies, headDependencies)
	hasMeaningfulDependencyChange := len(changed) > 0

	for _, note := range replaceNotes(baseManifest, headManifest) {
		notes = append(notes, note)
	}

	for _, dependency := range headDependencies {
		if _, ok := changed[dependency.Name]; !ok {
			continue
		}
		if pseudoVersion(dependency.Version) || pseudoVersion(dependency.Source) {
			notes = append(notes, analysis.Note{
				Code:       analysis.NoteGoPseudoVersion,
				Dependency: dependency.Name,
				Detail:     pseudoVersionDetail(dependency),
			})
		}
	}

	if hasMeaningfulDependencyChange && baseManifest.GoVersion != headManifest.GoVersion {
		notes = append(notes, analysis.Note{
			Code:   analysis.NoteGoDirectiveChanged,
			Detail: fmt.Sprintf("go %s -> %s", displayValue(baseManifest.GoVersion), displayValue(headManifest.GoVersion)),
		})
	}
	if hasMeaningfulDependencyChange && baseManifest.Toolchain != headManifest.Toolchain {
		notes = append(notes, analysis.Note{
			Code:   analysis.NoteGoToolchainChanged,
			Detail: fmt.Sprintf("toolchain %s -> %s", displayValue(baseManifest.Toolchain), displayValue(headManifest.Toolchain)),
		})
	}
	if detail := checksumDetail(baseSum, headSum, target.LockfilePath); hasMeaningfulDependencyChange && detail != "" {
		notes = append(notes, analysis.Note{
			Code:   analysis.NoteGoChecksumChanged,
			Detail: detail,
		})
	}

	return notes
}

func replaceNotes(baseManifest, headManifest Manifest) []analysis.Note {
	base := replacementMap(baseManifest.Replacements)
	head := replacementMap(headManifest.Replacements)
	keys := map[string]struct{}{}
	for key := range base {
		keys[key] = struct{}{}
	}
	for key := range head {
		keys[key] = struct{}{}
	}

	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	notes := make([]analysis.Note, 0)
	for _, key := range sorted {
		before, beforeOK := base[key]
		after, afterOK := head[key]
		if beforeOK && afterOK && replacementSource(before) == replacementSource(after) {
			continue
		}

		dependency := key
		if afterOK {
			dependency = replacementIdentity(after)
		} else if beforeOK {
			dependency = replacementIdentity(before)
		}

		detail := ""
		switch {
		case !beforeOK && afterOK:
			detail = "replace added: " + replacementDescription(after)
		case beforeOK && !afterOK:
			detail = "replace removed: " + replacementDescription(before)
		default:
			detail = "replace changed: " + replacementDescription(before) + " -> " + replacementDescription(after)
		}
		notes = append(notes, analysis.Note{
			Code:       analysis.NoteGoReplaceDirective,
			Dependency: dependency,
			Detail:     detail,
		})
		if afterOK && after.Local {
			notes = append(notes, analysis.Note{
				Code:       analysis.NoteGoLocalReplace,
				Dependency: dependency,
				Detail:     replacementDescription(after),
			})
		}
	}
	return notes
}

func replacementMap(replacements []Replacement) map[string]Replacement {
	result := map[string]Replacement{}
	for _, replacement := range replacements {
		result[replacementIdentity(replacement)] = replacement
	}
	return result
}

func replacementDescription(replacement Replacement) string {
	left := replacement.OldPath
	if replacement.OldVersion != "" {
		left += "@" + replacement.OldVersion
	}
	right := replacement.NewPath
	if replacement.NewVersion != "" {
		right += "@" + replacement.NewVersion
	}
	return left + " => " + right
}

func changedDependencySet(baseDependencies, headDependencies []analysis.LocalDependency) map[string]struct{} {
	base := dependencyMap(baseDependencies)
	head := dependencyMap(headDependencies)
	keys := map[string]struct{}{}
	for key := range base {
		keys[key] = struct{}{}
	}
	for key := range head {
		keys[key] = struct{}{}
	}
	changed := map[string]struct{}{}
	for key := range keys {
		before, beforeOK := base[key]
		after, afterOK := head[key]
		if !beforeOK ||
			!afterOK ||
			before.Requirement != after.Requirement ||
			before.Version != after.Version ||
			before.Source != after.Source ||
			before.Scope != after.Scope {
			changed[key] = struct{}{}
		}
	}
	return changed
}

func dependencyMap(dependencies []analysis.LocalDependency) map[string]analysis.LocalDependency {
	result := map[string]analysis.LocalDependency{}
	for _, dependency := range dependencies {
		if dependency.Name == "" {
			continue
		}
		result[dependency.Name] = dependency
	}
	return result
}

func pseudoVersion(value string) bool {
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '@' || r == '#' || r == ':' || r == '/' || r == '\\'
	}) {
		if pseudoVersionPattern.MatchString(part) {
			return true
		}
	}
	return pseudoVersionPattern.MatchString(value)
}

func pseudoVersionDetail(dependency analysis.LocalDependency) string {
	if pseudoVersion(dependency.Version) {
		return dependency.Version
	}
	return dependency.Source
}

func checksumDetail(baseSum, headSum SumFile, lockfilePath string) string {
	if strings.TrimSpace(lockfilePath) == "" {
		return ""
	}
	base := sumEntrySet(baseSum)
	head := sumEntrySet(headSum)
	added := 0
	removed := 0
	for entry := range head {
		if _, ok := base[entry]; !ok {
			added++
		}
	}
	for entry := range base {
		if _, ok := head[entry]; !ok {
			removed++
		}
	}
	if added == 0 && removed == 0 {
		return ""
	}
	return fmt.Sprintf("%s checksum evidence changed: added=%d removed=%d", lockfilePath, added, removed)
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func convertUnsupported(manifestPath string, groups ...[]UnsupportedEntry) []analysis.LocalUnsupportedEntry {
	if strings.TrimSpace(manifestPath) == "" {
		return nil
	}
	converted := []analysis.LocalUnsupportedEntry{}
	for _, group := range groups {
		for _, entry := range group {
			converted = append(converted, analysis.LocalUnsupportedEntry{
				Manifest: manifestPath,
				Line:     entry.Line,
				Text:     entry.Text,
				Reason:   entry.Reason,
			})
		}
	}
	return converted
}
