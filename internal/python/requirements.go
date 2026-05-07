package python

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const packageNamePattern = `[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?`

var (
	directRequirementPattern = regexp.MustCompile(`^(` + packageNamePattern + `)(\[[^\]]+\])?\s+@\s+(.+)$`)
	namedRequirementPattern  = regexp.MustCompile(`^(` + packageNamePattern + `)(\[[^\]]+\])?\s*(.*)$`)
	eggNamePattern           = regexp.MustCompile(`(?i)(?:[#&?]egg=)(` + packageNamePattern + `)`)
	envReferencePattern      = regexp.MustCompile(`\$\{?[A-Za-z_][A-Za-z0-9_]*\}?|%[A-Za-z_][A-Za-z0-9_]*%`)
)

type logicalLine struct {
	Line int
	Text string
}

func ParseRequirements(data []byte) (ParseResult, error) {
	result := ParseResult{}
	for _, line := range joinContinuationLines(string(data)) {
		text := strings.TrimSpace(stripInlineComment(line.Text))
		if text == "" {
			continue
		}
		if reason := unsupportedRequirementReason(text); reason != "" {
			result.Unsupported = append(result.Unsupported, UnsupportedEntry{Line: line.Line, Text: text, Reason: reason})
			continue
		}
		dependency, unsupported := parseRequirementString(text, ScopeRuntime, line.Line)
		if unsupported.Reason != "" {
			result.Unsupported = append(result.Unsupported, unsupported)
			continue
		}
		result.Dependencies = append(result.Dependencies, dependency)
	}
	sortDependencies(result.Dependencies)
	return result, nil
}

func parseRequirementString(value string, scope Scope, line int) (Dependency, UnsupportedEntry) {
	text := strings.TrimSpace(value)
	if text == "" {
		return Dependency{}, UnsupportedEntry{}
	}
	if reason := unsupportedRequirementReason(text); reason != "" {
		return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: reason}
	}

	body, marker := splitMarker(text)
	body = strings.TrimSpace(body)
	lowerBody := strings.ToLower(body)
	if isBareSource(body) {
		name := extractEggName(body)
		if name == "" {
			return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: "source requirement does not expose a stable package name"}
		}
		if containsEnvReference(body) {
			return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: "source requirement contains an environment variable"}
		}
		return Dependency{
			Name:        NormalizeName(name),
			Requirement: appendMarker(body, marker),
			Source:      body,
			Scope:       scope,
			Raw:         text,
			Line:        line,
		}, UnsupportedEntry{}
	}
	if strings.HasPrefix(lowerBody, "http://") || strings.HasPrefix(lowerBody, "https://") || strings.HasPrefix(lowerBody, "file:") {
		return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: "archive or URL requirement is missing a stable package name"}
	}

	if matches := directRequirementPattern.FindStringSubmatch(body); matches != nil {
		source := strings.TrimSpace(matches[3])
		if containsEnvReference(source) {
			return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: "source requirement contains an environment variable"}
		}
		requirement := strings.TrimSpace(matches[2] + " @ " + source)
		return Dependency{
			Name:        NormalizeName(matches[1]),
			Requirement: appendMarker(requirement, marker),
			Source:      source,
			Scope:       scope,
			Raw:         text,
			Line:        line,
		}, UnsupportedEntry{}
	}

	matches := namedRequirementPattern.FindStringSubmatch(body)
	if matches == nil {
		return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: "requirement line is not in the supported direct dependency subset"}
	}
	name := NormalizeName(matches[1])
	extras := strings.TrimSpace(matches[2])
	specifier := strings.TrimSpace(matches[3])
	if specifier != "" && !isSupportedSpecifier(specifier) {
		return Dependency{}, UnsupportedEntry{Line: line, Text: text, Reason: "requirement specifier is not in the supported subset"}
	}
	requirement := strings.TrimSpace(extras + specifier)
	return Dependency{
		Name:        name,
		Requirement: appendMarker(requirement, marker),
		Version:     pinnedVersion(specifier),
		Scope:       scope,
		Raw:         text,
		Line:        line,
	}, UnsupportedEntry{}
}

func joinContinuationLines(data string) []logicalLine {
	scanner := bufio.NewScanner(strings.NewReader(data))
	lines := []logicalLine{}
	var builder strings.Builder
	startLine := 1
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmedRight, "\\") {
			if builder.Len() == 0 {
				startLine = lineNumber
			}
			builder.WriteString(strings.TrimSpace(strings.TrimSuffix(trimmedRight, "\\")))
			builder.WriteString(" ")
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString(strings.TrimSpace(line))
			lines = append(lines, logicalLine{Line: startLine, Text: builder.String()})
			builder.Reset()
			continue
		}
		lines = append(lines, logicalLine{Line: lineNumber, Text: line})
	}
	if builder.Len() > 0 {
		lines = append(lines, logicalLine{Line: startLine, Text: builder.String()})
	}
	return lines
}

func stripInlineComment(line string) string {
	inSingle := false
	inDouble := false
	var previous rune
	for index, current := range line {
		switch current {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (index == 0 || isWhitespace(previous)) {
				return strings.TrimSpace(line[:index])
			}
		}
		previous = current
	}
	return line
}

func splitMarker(value string) (string, string) {
	inSingle := false
	inDouble := false
	for index, current := range value {
		switch current {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case ';':
			if !inSingle && !inDouble {
				return strings.TrimSpace(value[:index]), strings.TrimSpace(value[index+1:])
			}
		}
	}
	return value, ""
}

func unsupportedRequirementReason(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	first := fields[0]
	switch first {
	case "-r", "--requirement":
		return "included requirement files are not supported"
	case "-c", "--constraint":
		return "constraint files are not supported"
	case "-e", "--editable":
		return "editable installs are not supported"
	}
	if strings.HasPrefix(first, "-r") || strings.HasPrefix(first, "--requirement=") {
		return "included requirement files are not supported"
	}
	if strings.HasPrefix(first, "-c") || strings.HasPrefix(first, "--constraint=") {
		return "constraint files are not supported"
	}
	if strings.HasPrefix(first, "-e") || strings.HasPrefix(first, "--editable=") {
		return "editable installs are not supported"
	}
	if strings.HasPrefix(first, "-") {
		return "pip command options are not supported"
	}
	for _, token := range fields[1:] {
		if strings.HasPrefix(token, "--") {
			return "per-requirement options are not supported"
		}
	}
	return ""
}

func isBareSource(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "git+") || strings.HasPrefix(lower, "hg+") || strings.HasPrefix(lower, "svn+") || strings.HasPrefix(lower, "bzr+")
}

func extractEggName(value string) string {
	matches := eggNamePattern.FindStringSubmatch(value)
	if matches == nil {
		return ""
	}
	return matches[1]
}

func containsEnvReference(value string) bool {
	return envReferencePattern.MatchString(value)
}

func isSupportedSpecifier(value string) bool {
	specifier := strings.TrimSpace(value)
	if specifier == "" {
		return true
	}
	for _, prefix := range []string{"==", ">=", "<=", "~=", "!=", "===", ">", "<"} {
		if strings.HasPrefix(specifier, prefix) {
			return true
		}
	}
	return false
}

func pinnedVersion(value string) string {
	specifier := strings.TrimSpace(value)
	if !strings.HasPrefix(specifier, "==") || strings.HasPrefix(specifier, "===") {
		return ""
	}
	version := strings.TrimSpace(strings.TrimPrefix(specifier, "=="))
	if version == "" || strings.ContainsAny(version, ",<>~=! ") || strings.Contains(version, "*") {
		return ""
	}
	return version
}

func appendMarker(requirement, marker string) string {
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return strings.TrimSpace(requirement)
	}
	if strings.TrimSpace(requirement) == "" {
		return fmt.Sprintf("; %s", marker)
	}
	return fmt.Sprintf("%s; %s", strings.TrimSpace(requirement), marker)
}

func isWhitespace(value rune) bool {
	return value == ' ' || value == '\t'
}

func sortDependencies(dependencies []Dependency) {
	sort.SliceStable(dependencies, func(i, j int) bool {
		if dependencies[i].Name == dependencies[j].Name {
			return dependencies[i].Line < dependencies[j].Line
		}
		return dependencies[i].Name < dependencies[j].Name
	})
}
