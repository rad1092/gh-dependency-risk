package python

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type uvLockDocument struct {
	Version  any             `toml:"version"`
	Revision any             `toml:"revision"`
	Packages []uvLockPackage `toml:"package"`
}

type uvLockPackage struct {
	Name    string         `toml:"name"`
	Version string         `toml:"version"`
	Source  map[string]any `toml:"source"`
}

func ParseUvLockfile(data []byte) (Lockfile, error) {
	lockfile := Lockfile{}
	if strings.TrimSpace(string(data)) == "" {
		return lockfile, nil
	}
	var document uvLockDocument
	if err := toml.Unmarshal(data, &document); err != nil {
		return Lockfile{}, fmt.Errorf("parse uv.lock: %w", err)
	}
	if len(document.Packages) == 0 {
		lockfile.Unsupported = append(lockfile.Unsupported, UnsupportedEntry{
			Text:   "uv.lock",
			Reason: "uv.lock does not contain supported package entries",
		})
		return lockfile, nil
	}

	for index, item := range document.Packages {
		name := NormalizeName(item.Name)
		if name == "" {
			lockfile.Unsupported = append(lockfile.Unsupported, UnsupportedEntry{
				Text:   fmt.Sprintf("package[%d]", index),
				Reason: "uv.lock package entry is missing a package name",
			})
			continue
		}
		source, sourceType, sourceURL, sourceReference, unsupported := parseUvPackageSource(name, item.Source)
		if unsupported.Reason != "" {
			lockfile.Unsupported = append(lockfile.Unsupported, unsupported)
		}
		lockfile.Packages = append(lockfile.Packages, LockPackage{
			Name:            name,
			Version:         strings.TrimSpace(item.Version),
			Source:          source,
			SourceType:      sourceType,
			SourceURL:       sourceURL,
			SourceReference: sourceReference,
		})
	}
	sort.SliceStable(lockfile.Packages, func(i, j int) bool {
		return lockfile.Packages[i].Name < lockfile.Packages[j].Name
	})
	return lockfile, nil
}

func ApplyUvLockfile(result ParseResult, lockfile Lockfile) ParseResult {
	packages := map[string]LockPackage{}
	for _, item := range lockfile.Packages {
		if _, ok := packages[item.Name]; ok {
			continue
		}
		packages[item.Name] = item
	}
	dependencies := make([]Dependency, 0, len(result.Dependencies))
	for _, dependency := range result.Dependencies {
		if item, ok := packages[dependency.Name]; ok {
			if item.Version != "" {
				dependency.Version = item.Version
			}
			if dependency.Source == "" && item.Source != "" {
				dependency.Source = item.Source
			}
		}
		dependencies = append(dependencies, dependency)
	}
	result.Dependencies = dependencies
	return result
}

func parseUvPackageSource(name string, source map[string]any) (string, string, string, string, UnsupportedEntry) {
	if len(source) == 0 {
		return "", "", "", "", UnsupportedEntry{}
	}
	keys := sortedAnyMapKeys(source)
	text := fmt.Sprintf("uv.lock package %q source", name)
	if len(keys) == 1 && isUvRegistryLikeSourceKey(keys[0]) {
		value, ok := uvSourceStringOrBool(source[keys[0]])
		if !ok {
			return "", "", "", "", UnsupportedEntry{Text: text, Reason: fmt.Sprintf("uv.lock package %q uses unsupported source shape", name)}
		}
		return "", keys[0], value, "", UnsupportedEntry{}
	}

	if git, ok := uvSourceString(source, "git"); ok && git != "" {
		if !uvSourceKeysAllowed(keys, map[string]struct{}{
			"git":                {},
			"rev":                {},
			"tag":                {},
			"branch":             {},
			"reference":          {},
			"resolved":           {},
			"resolved-reference": {},
			"resolved_reference": {},
			"commit":             {},
		}) {
			return "", "", "", "", UnsupportedEntry{Text: text, Reason: fmt.Sprintf("uv.lock package %q uses unsupported source shape", name)}
		}
		if !uvSourceKeysHaveStringValues(source, keys) {
			return "", "", "", "", UnsupportedEntry{Text: text, Reason: fmt.Sprintf("uv.lock package %q uses unsupported source shape", name)}
		}
		reference := uvSourceReference(source, []string{"rev", "tag", "branch", "reference", "resolved", "resolved-reference", "resolved_reference", "commit"})
		return formatPoetrySource("git", git, reference), "git", git, reference, UnsupportedEntry{}
	}

	if value, ok := uvSingleStringSource(source, keys, "url"); ok {
		return formatPoetrySource("url", value, ""), "url", value, "", UnsupportedEntry{}
	}
	if value, ok := uvPathLikeSource(source, keys, "path"); ok {
		reference := uvEditableReference(source)
		return formatPoetrySource("path", value, reference), "path", value, reference, UnsupportedEntry{}
	}
	if value, ok := uvPathLikeSource(source, keys, "directory"); ok {
		reference := uvEditableReference(source)
		return formatPoetrySource("directory", value, reference), "directory", value, reference, UnsupportedEntry{}
	}
	if value, ok := uvSingleStringSource(source, keys, "editable"); ok {
		return formatPoetrySource("editable", value, ""), "editable", value, "", UnsupportedEntry{}
	}

	return "", "", "", "", UnsupportedEntry{Text: text, Reason: fmt.Sprintf("uv.lock package %q uses unsupported source shape", name)}
}

func sortedAnyMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	sort.Strings(keys)
	return keys
}

func isUvRegistryLikeSourceKey(key string) bool {
	switch key {
	case "registry", "virtual", "workspace":
		return true
	default:
		return false
	}
}

func uvSourceString(source map[string]any, key string) (string, bool) {
	value, ok := source[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func uvSourceStringOrBool(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

func uvSourceKeysAllowed(keys []string, allowed map[string]struct{}) bool {
	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	return true
}

func uvSourceKeysHaveStringValues(source map[string]any, keys []string) bool {
	for _, key := range keys {
		if _, ok := uvSourceString(source, key); !ok {
			return false
		}
	}
	return true
}

func uvSourceReference(source map[string]any, keys []string) string {
	for _, key := range keys {
		value, ok := uvSourceString(source, key)
		if !ok || value == "" {
			continue
		}
		return key + "=" + value
	}
	return ""
}

func uvSingleStringSource(source map[string]any, keys []string, key string) (string, bool) {
	if len(keys) != 1 || keys[0] != key {
		return "", false
	}
	value, ok := uvSourceString(source, key)
	return value, ok && value != ""
}

func uvPathLikeSource(source map[string]any, keys []string, key string) (string, bool) {
	if !uvSourceKeysAllowed(keys, map[string]struct{}{key: {}, "editable": {}}) {
		return "", false
	}
	value, ok := uvSourceString(source, key)
	if !ok || value == "" {
		return "", false
	}
	if _, exists := source["editable"]; exists {
		if _, ok := source["editable"].(bool); !ok {
			return "", false
		}
	}
	return value, true
}

func uvEditableReference(source map[string]any) string {
	editable, ok := source["editable"].(bool)
	if !ok || !editable {
		return ""
	}
	return "editable=true"
}
