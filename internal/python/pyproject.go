package python

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	poetryTablePattern           = regexp.MustCompile(`(?m)^\s*\[tool\.poetry(?:\]|\.)`)
	dependencyGroupsTablePattern = regexp.MustCompile(`(?m)^\s*\[dependency-groups(?:\]|\.)`)
)

type pyprojectDocument struct {
	Project struct {
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Dependencies    map[string]any `toml:"dependencies"`
			DevDependencies map[string]any `toml:"dev-dependencies"`
			Group           map[string]struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"group"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func ParsePyProject(data []byte) (ParseResult, error) {
	result := ParseResult{}
	if strings.TrimSpace(string(data)) == "" {
		return result, nil
	}
	var document pyprojectDocument
	if err := toml.Unmarshal(data, &document); err != nil {
		return ParseResult{}, fmt.Errorf("parse pyproject.toml: %w", err)
	}

	for index, value := range document.Project.Dependencies {
		appendPyProjectDependency(&result, value, ScopeRuntime, index+1)
	}

	groups := make([]string, 0, len(document.Project.OptionalDependencies))
	for group := range document.Project.OptionalDependencies {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		for index, value := range document.Project.OptionalDependencies[group] {
			appendPyProjectDependency(&result, value, ScopeOptional, index+1)
		}
	}

	if poetryTablePattern.Match(data) {
		result.Unsupported = append(result.Unsupported, UnsupportedEntry{
			Text:   "[tool.poetry]",
			Reason: "Poetry dependency tables are handled by Poetry local fallback targets, not the PEP 621 direct fallback",
		})
	}
	if dependencyGroupsTablePattern.Match(data) {
		result.Unsupported = append(result.Unsupported, UnsupportedEntry{
			Text:   "[dependency-groups]",
			Reason: "dependency groups are not supported by the Python direct local fallback in this phase",
		})
	}

	sortDependencies(result.Dependencies)
	return result, nil
}

func HasPEP621Dependencies(data []byte) (bool, error) {
	result, err := ParsePyProject(data)
	if err != nil {
		return false, err
	}
	return len(result.Dependencies) > 0, nil
}

func HasPoetryProject(data []byte) bool {
	if strings.TrimSpace(string(data)) == "" {
		return false
	}
	return poetryTablePattern.Match(data)
}

func ParsePoetryPyProject(data []byte) (ParseResult, error) {
	result := ParseResult{}
	if strings.TrimSpace(string(data)) == "" {
		return result, nil
	}
	var document pyprojectDocument
	if err := toml.Unmarshal(data, &document); err != nil {
		return ParseResult{}, fmt.Errorf("parse Poetry pyproject.toml: %w", err)
	}

	appendPoetryDependencies(&result, document.Tool.Poetry.Dependencies, ScopeRuntime, "[tool.poetry.dependencies]", "")
	appendPoetryDependencies(&result, document.Tool.Poetry.DevDependencies, ScopeDev, "[tool.poetry.dev-dependencies]", "")

	groups := make([]string, 0, len(document.Tool.Poetry.Group))
	for group := range document.Tool.Poetry.Group {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		scope, classified := poetryGroupScope(group)
		appendPoetryDependencies(&result, document.Tool.Poetry.Group[group].Dependencies, scope, fmt.Sprintf("[tool.poetry.group.%s.dependencies]", group), group)
		if !classified && len(document.Tool.Poetry.Group[group].Dependencies) > 0 {
			result.Unsupported = append(result.Unsupported, UnsupportedEntry{
				Text:   fmt.Sprintf("[tool.poetry.group.%s.dependencies]", group),
				Reason: "Poetry dependency group scope is not classified by the local fallback in this phase",
			})
		}
	}

	if dependencyGroupsTablePattern.Match(data) {
		result.Unsupported = append(result.Unsupported, UnsupportedEntry{
			Text:   "[dependency-groups]",
			Reason: "dependency groups are not supported by the Python direct local fallback in this phase",
		})
	}

	sortDependencies(result.Dependencies)
	return result, nil
}

func appendPyProjectDependency(result *ParseResult, value string, scope Scope, index int) {
	dependency, unsupported := parseRequirementString(value, scope, index)
	if unsupported.Reason != "" {
		result.Unsupported = append(result.Unsupported, unsupported)
		return
	}
	result.Dependencies = append(result.Dependencies, dependency)
}

func appendPoetryDependencies(result *ParseResult, dependencies map[string]any, scope Scope, section, group string) {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dependency, unsupported := parsePoetryDependency(name, dependencies[name], scope, section, group)
		if unsupported.Reason != "" {
			result.Unsupported = append(result.Unsupported, unsupported)
			continue
		}
		if strings.TrimSpace(dependency.Name) == "" {
			continue
		}
		result.Dependencies = append(result.Dependencies, dependency)
	}
}

func parsePoetryDependency(name string, value any, scope Scope, section, group string) (Dependency, UnsupportedEntry) {
	normalizedName := NormalizeName(name)
	if normalizedName == "" || normalizedName == "python" {
		return Dependency{}, UnsupportedEntry{}
	}
	text := fmt.Sprintf("%s.%s", section, name)
	switch typed := value.(type) {
	case string:
		requirement := strings.TrimSpace(typed)
		return Dependency{
			Name:        normalizedName,
			Requirement: requirement,
			Version:     poetryPinnedVersion(requirement),
			Scope:       scope,
			Raw:         fmt.Sprintf("%s = %q", name, typed),
		}, UnsupportedEntry{}
	case map[string]any:
		return parsePoetryDependencyTable(normalizedName, name, typed, scope, text)
	default:
		return Dependency{}, UnsupportedEntry{
			Text:   text,
			Reason: fmt.Sprintf("Poetry dependency %q uses an unsupported value shape", name),
		}
	}
}

func parsePoetryDependencyTable(normalizedName, originalName string, table map[string]any, scope Scope, text string) (Dependency, UnsupportedEntry) {
	for key, value := range table {
		if !isSupportedPoetryDependencyTableKey(key) {
			return Dependency{}, UnsupportedEntry{Text: text, Reason: fmt.Sprintf("Poetry dependency %q uses unsupported table key %q", originalName, key)}
		}
		if !poetryDependencyTableValueTypeSupported(key, value) {
			return Dependency{}, UnsupportedEntry{Text: text, Reason: fmt.Sprintf("Poetry dependency %q has unsupported value for key %q", originalName, key)}
		}
	}

	version := stringValue(table["version"])
	extras := stringSliceValue(table["extras"])
	markers := stringValue(table["markers"])
	optional := boolValue(table["optional"])
	if optional {
		scope = ScopeOptional
	}
	source := poetryDependencySource(table)

	parts := make([]string, 0, 5)
	if version != "" {
		parts = append(parts, version)
	}
	if len(extras) > 0 {
		parts = append(parts, "extras="+strings.Join(extras, ","))
	}
	if markers != "" {
		parts = append(parts, "markers="+markers)
	}
	if optional {
		parts = append(parts, "optional=true")
	}
	if source != "" {
		parts = append(parts, "source="+source)
	}
	if len(parts) == 0 {
		return Dependency{}, UnsupportedEntry{Text: text, Reason: fmt.Sprintf("Poetry dependency %q table does not contain a supported dependency declaration", originalName)}
	}

	return Dependency{
		Name:        normalizedName,
		Requirement: strings.Join(parts, "; "),
		Version:     poetryPinnedVersion(version),
		Source:      source,
		Scope:       scope,
		Raw:         text,
	}, UnsupportedEntry{}
}

func poetryGroupScope(group string) (Scope, bool) {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "dev", "test", "tests", "lint", "lints", "docs", "doc", "ci", "qa", "type", "types", "typing":
		return ScopeDev, true
	default:
		return ScopeUnknown, false
	}
}

func isSupportedPoetryDependencyTableKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "version", "extras", "optional", "markers", "source", "git", "path", "url", "branch", "tag", "rev", "reference", "develop", "python", "platform", "allow-prereleases":
		return true
	default:
		return false
	}
}

func poetryDependencyTableValueTypeSupported(key string, value any) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "optional", "develop", "allow-prereleases":
		_, ok := value.(bool)
		return ok
	case "extras":
		_, ok := toStringSlice(value)
		return ok
	default:
		_, ok := value.(string)
		return ok
	}
}

func poetryDependencySource(table map[string]any) string {
	if value := stringValue(table["git"]); value != "" {
		return formatPoetrySource("git", value, poetrySourceReference(table))
	}
	if value := stringValue(table["path"]); value != "" {
		return formatPoetrySource("path", value, "")
	}
	if value := stringValue(table["url"]); value != "" {
		return formatPoetrySource("url", value, "")
	}
	if value := stringValue(table["source"]); value != "" {
		return formatPoetrySource("source", value, "")
	}
	return ""
}

func poetrySourceReference(table map[string]any) string {
	for _, key := range []string{"branch", "tag", "rev", "reference"} {
		if value := stringValue(table[key]); value != "" {
			return key + "=" + value
		}
	}
	return ""
}

func formatPoetrySource(sourceType, value, reference string) string {
	sourceType = strings.TrimSpace(sourceType)
	value = strings.TrimSpace(value)
	reference = strings.TrimSpace(reference)
	source := value
	if sourceType != "" {
		source = sourceType + ":" + source
	}
	if reference != "" {
		source += "#" + reference
	}
	return source
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func boolValue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

func stringSliceValue(value any) []string {
	values, ok := toStringSlice(value)
	if !ok {
		return nil
	}
	return values
}

func toStringSlice(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		result := append([]string(nil), typed...)
		sort.Strings(result)
		return result, true
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			result = append(result, strings.TrimSpace(text))
		}
		sort.Strings(result)
		return result, true
	default:
		return nil, false
	}
}

func poetryPinnedVersion(value string) string {
	version := strings.TrimSpace(value)
	if version == "" || strings.ContainsAny(version, "<>~=!^*, []();") {
		return ""
	}
	return version
}
