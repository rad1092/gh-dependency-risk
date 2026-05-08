package npm

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
)

type PackageManifest struct {
	Name                 string            `json:"name"`
	PackageManager       string            `json:"packageManager"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Workspaces           []string          `json:"-"`
}

func ParsePackageManifest(data []byte) (*PackageManifest, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var raw struct {
		Name                 string            `json:"name"`
		PackageManager       string            `json:"packageManager"`
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
		Workspaces           json.RawMessage   `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse package.json: %w", err)
	}
	manifest := &PackageManifest{
		Name:                 raw.Name,
		PackageManager:       raw.PackageManager,
		Dependencies:         raw.Dependencies,
		DevDependencies:      raw.DevDependencies,
		OptionalDependencies: raw.OptionalDependencies,
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = map[string]string{}
	}
	if manifest.DevDependencies == nil {
		manifest.DevDependencies = map[string]string{}
	}
	if manifest.OptionalDependencies == nil {
		manifest.OptionalDependencies = map[string]string{}
	}
	workspaces, err := parseWorkspaces(raw.Workspaces)
	if err != nil {
		return nil, err
	}
	manifest.Workspaces = workspaces
	return manifest, nil
}

func (m *PackageManifest) Scope(name string) (string, bool) {
	if m == nil {
		return "", false
	}
	if _, ok := m.Dependencies[name]; ok {
		return "runtime", true
	}
	if _, ok := m.OptionalDependencies[name]; ok {
		return "optional", true
	}
	if _, ok := m.DevDependencies[name]; ok {
		return "dev", true
	}
	return "", false
}

func (m *PackageManifest) Requirement(name string) string {
	if m == nil {
		return ""
	}
	if value, ok := m.Dependencies[name]; ok {
		return value
	}
	if value, ok := m.OptionalDependencies[name]; ok {
		return value
	}
	if value, ok := m.DevDependencies[name]; ok {
		return value
	}
	return ""
}

func (m *PackageManifest) DirectNames() []string {
	if m == nil {
		return nil
	}
	set := map[string]struct{}{}
	for name := range m.Dependencies {
		set[name] = struct{}{}
	}
	for name := range m.DevDependencies {
		set[name] = struct{}{}
	}
	for name := range m.OptionalDependencies {
		set[name] = struct{}{}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func parseWorkspaces(data json.RawMessage) ([]string, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var array []string
	if err := json.Unmarshal(data, &array); err == nil {
		return cleanWorkspacePatterns(array), nil
	}

	var object struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("parse package.json workspaces: %w", err)
	}
	return cleanWorkspacePatterns(object.Packages), nil
}

func cleanWorkspacePatterns(patterns []string) []string {
	return uniqueWorkspacePatterns(patterns)
}

func cleanWorkspacePattern(pattern string) string {
	cleaned := path.Clean(pattern)
	switch cleaned {
	case ".", "/":
		return ""
	default:
		return cleaned
	}
}
