package python

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type poetryLockDocument struct {
	Packages []poetryLockPackage `toml:"package"`
}

type poetryLockPackage struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	Category string   `toml:"category"`
	Groups   []string `toml:"groups"`
	Optional bool     `toml:"optional"`
	Source   struct {
		Type              string `toml:"type"`
		URL               string `toml:"url"`
		Reference         string `toml:"reference"`
		ResolvedReference string `toml:"resolved_reference"`
	} `toml:"source"`
}

func ParsePoetryLockfile(data []byte) (Lockfile, error) {
	lockfile := Lockfile{}
	if strings.TrimSpace(string(data)) == "" {
		return lockfile, nil
	}
	var document poetryLockDocument
	if err := toml.Unmarshal(data, &document); err != nil {
		return Lockfile{}, fmt.Errorf("parse poetry.lock: %w", err)
	}
	if len(document.Packages) == 0 {
		lockfile.Unsupported = append(lockfile.Unsupported, UnsupportedEntry{
			Text:   "poetry.lock",
			Reason: "poetry.lock does not contain supported package entries",
		})
		return lockfile, nil
	}

	for index, item := range document.Packages {
		name := NormalizeName(item.Name)
		if name == "" {
			lockfile.Unsupported = append(lockfile.Unsupported, UnsupportedEntry{
				Text:   fmt.Sprintf("package[%d]", index),
				Reason: "poetry.lock package entry is missing a package name",
			})
			continue
		}
		groups := append([]string(nil), item.Groups...)
		sort.Strings(groups)
		sourceReference := strings.TrimSpace(item.Source.Reference)
		if sourceReference == "" {
			sourceReference = strings.TrimSpace(item.Source.ResolvedReference)
		}
		lockfile.Packages = append(lockfile.Packages, LockPackage{
			Name:            name,
			Version:         strings.TrimSpace(item.Version),
			Category:        strings.TrimSpace(item.Category),
			Groups:          groups,
			Optional:        item.Optional,
			Source:          formatPoetrySource(item.Source.Type, item.Source.URL, sourceReference),
			SourceType:      strings.TrimSpace(item.Source.Type),
			SourceURL:       strings.TrimSpace(item.Source.URL),
			SourceReference: sourceReference,
		})
	}
	sort.Slice(lockfile.Packages, func(i, j int) bool {
		return lockfile.Packages[i].Name < lockfile.Packages[j].Name
	})
	return lockfile, nil
}

func ApplyPoetryLockfile(result ParseResult, lockfile Lockfile) ParseResult {
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
