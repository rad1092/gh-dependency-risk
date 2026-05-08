package gomod

import (
	"bufio"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

func ParseModFile(data []byte) (Manifest, error) {
	if strings.TrimSpace(string(data)) == "" {
		return Manifest{}, nil
	}

	file, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("go.mod local fallback could not parse file safely: %w", err)
	}

	manifest := Manifest{}
	if file.Module != nil {
		manifest.ModulePath = clean(file.Module.Mod.Path)
	}
	if file.Go != nil {
		manifest.GoVersion = clean(file.Go.Version)
	}
	if file.Toolchain != nil {
		manifest.Toolchain = clean(file.Toolchain.Name)
	}
	if manifest.Toolchain == "" {
		manifest.Toolchain = parseToolchainDirective(data)
	}

	for _, require := range file.Require {
		if require == nil || clean(require.Mod.Path) == "" {
			continue
		}
		manifest.Requirements = append(manifest.Requirements, Requirement{
			Path:     clean(require.Mod.Path),
			Version:  clean(require.Mod.Version),
			Indirect: require.Indirect,
		})
	}
	sort.Slice(manifest.Requirements, func(i, j int) bool {
		if manifest.Requirements[i].Path == manifest.Requirements[j].Path {
			return manifest.Requirements[i].Version < manifest.Requirements[j].Version
		}
		return manifest.Requirements[i].Path < manifest.Requirements[j].Path
	})

	replacements := replacementsFromModfile(file)
	rawReplacements, err := parseReplacementDirectives(data)
	if err != nil {
		return Manifest{}, fmt.Errorf("go.mod local fallback could not parse replace directives safely: %w", err)
	}
	if len(rawReplacements) > 0 {
		replacements = rawReplacements
	}
	for _, replace := range replacements {
		manifest.Replacements = append(manifest.Replacements, replace)
	}
	sort.Slice(manifest.Replacements, func(i, j int) bool {
		if manifest.Replacements[i].OldPath == manifest.Replacements[j].OldPath {
			if manifest.Replacements[i].OldVersion == manifest.Replacements[j].OldVersion {
				return manifest.Replacements[i].NewPath < manifest.Replacements[j].NewPath
			}
			return manifest.Replacements[i].OldVersion < manifest.Replacements[j].OldVersion
		}
		return manifest.Replacements[i].OldPath < manifest.Replacements[j].OldPath
	})

	return manifest, nil
}

func replacementsFromModfile(file *modfile.File) []Replacement {
	replacements := make([]Replacement, 0)
	for _, replace := range file.Replace {
		if replace == nil || clean(replace.Old.Path) == "" || clean(replace.New.Path) == "" {
			continue
		}
		item := Replacement{
			OldPath:    clean(replace.Old.Path),
			OldVersion: clean(replace.Old.Version),
			NewPath:    clean(replace.New.Path),
			NewVersion: clean(replace.New.Version),
		}
		item.Local = isLocalReplacePath(item.NewPath)
		replacements = append(replacements, item)
	}
	return replacements
}

func isLocalReplacePath(value string) bool {
	trimmed := clean(strings.ReplaceAll(value, "\\", "/"))
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "/") {
		return true
	}
	return filepath.IsAbs(trimmed)
}

func parseToolchainDirective(data []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(stripGoModComment(scanner.Text()))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "toolchain" {
			return unquoteToken(fields[1])
		}
	}
	return ""
}

func parseReplacementDirectives(data []byte) ([]Replacement, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	replacements := make([]Replacement, 0)
	inBlock := false
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(stripGoModComment(scanner.Text()))
		if line == "" {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			replacement, err := parseReplacementTokens(strings.Fields(line))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			replacements = append(replacements, replacement)
			continue
		}
		if line == "replace (" {
			inBlock = true
			continue
		}
		if !strings.HasPrefix(line, "replace ") {
			continue
		}
		replacement, err := parseReplacementTokens(strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "replace"))))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		replacements = append(replacements, replacement)
	}
	if inBlock {
		return nil, fmt.Errorf("unterminated replace block")
	}
	return replacements, nil
}

func parseReplacementTokens(tokens []string) (Replacement, error) {
	arrow := -1
	for i, token := range tokens {
		if token == "=>" {
			arrow = i
			break
		}
	}
	if arrow <= 0 || arrow == len(tokens)-1 {
		return Replacement{}, fmt.Errorf("replace directive must use 'old [version] => new [version]'")
	}
	left := tokens[:arrow]
	right := tokens[arrow+1:]
	if len(left) > 2 || len(right) > 2 {
		return Replacement{}, fmt.Errorf("replace directive has unsupported shape")
	}
	replacement := Replacement{
		OldPath: unquoteToken(left[0]),
		NewPath: unquoteToken(right[0]),
	}
	if len(left) == 2 {
		replacement.OldVersion = unquoteToken(left[1])
	}
	if len(right) == 2 {
		replacement.NewVersion = unquoteToken(right[1])
	}
	replacement.Local = isLocalReplacePath(replacement.NewPath)
	if replacement.OldPath == "" || replacement.NewPath == "" {
		return Replacement{}, fmt.Errorf("replace directive must include old and new module paths")
	}
	if !replacement.Local && replacement.NewVersion == "" {
		return Replacement{}, fmt.Errorf("replacement module without version must be a local directory path")
	}
	if replacement.Local && replacement.NewVersion != "" {
		return Replacement{}, fmt.Errorf("replacement local directory path cannot have a version")
	}
	return replacement, nil
}

func stripGoModComment(line string) string {
	inQuote := false
	escaped := false
	for i := 0; i < len(line)-1; i++ {
		switch {
		case escaped:
			escaped = false
		case line[i] == '\\':
			escaped = true
		case line[i] == '"':
			inQuote = !inQuote
		case !inQuote && line[i] == '/' && line[i+1] == '/':
			return line[:i]
		}
	}
	return line
}

func unquoteToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) >= 2 && token[0] == '"' && token[len(token)-1] == '"' {
		return strings.Trim(token, `"`)
	}
	return token
}
