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
			Reason: "Poetry dependency tables are not supported by the Python direct local fallback in this phase",
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

func appendPyProjectDependency(result *ParseResult, value string, scope Scope, index int) {
	dependency, unsupported := parseRequirementString(value, scope, index)
	if unsupported.Reason != "" {
		result.Unsupported = append(result.Unsupported, unsupported)
		return
	}
	result.Dependencies = append(result.Dependencies, dependency)
}
