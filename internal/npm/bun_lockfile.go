package npm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type BunLockfile struct {
	LockfileVersion int
	Entries         []BunEntry
	Unsupported     []BunUnsupportedEntry
}

type BunEntry struct {
	Key          string
	Name         string
	Descriptor   string
	Reference    string
	Protocol     string
	Range        string
	Version      string
	Resolution   string
	Source       string
	Checksum     string
	Dependencies map[string]string
}

type BunUnsupportedEntry struct {
	Descriptor string
	Reason     string
}

func IsBunPackageManager(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "bun@")
}

func ParseBunLockfile(data []byte) (BunLockfile, error) {
	result := BunLockfile{}
	if strings.TrimSpace(string(data)) == "" {
		return result, nil
	}
	jsonData, err := stripJSONC(data)
	if err != nil {
		return result, fmt.Errorf("parse bun.lock: %w", err)
	}
	jsonData = removeTrailingJSONCommas(jsonData)

	var raw struct {
		LockfileVersion int                        `json:"lockfileVersion"`
		Packages        map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return result, fmt.Errorf("parse bun.lock: %w", err)
	}
	result.LockfileVersion = raw.LockfileVersion
	if raw.Packages == nil {
		result.Unsupported = append(result.Unsupported, BunUnsupportedEntry{
			Descriptor: "packages",
			Reason:     "bun.lock has no packages mapping",
		})
		return result, nil
	}

	for key, value := range raw.Packages {
		entry, unsupported := parseBunPackageEntry(key, value)
		result.Unsupported = append(result.Unsupported, unsupported...)
		if strings.TrimSpace(entry.Name) == "" && len(unsupported) > 0 {
			continue
		}
		if strings.TrimSpace(entry.Name) == "" {
			result.Unsupported = append(result.Unsupported, BunUnsupportedEntry{
				Descriptor: key,
				Reason:     "bun.lock package entry has no stable package name",
			})
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		left := result.Entries[i]
		right := result.Entries[j]
		if left.Name == right.Name {
			if left.Descriptor == right.Descriptor {
				return left.Resolution < right.Resolution
			}
			return left.Descriptor < right.Descriptor
		}
		return left.Name < right.Name
	})
	sortBunUnsupported(result.Unsupported)
	return result, nil
}

func parseBunPackageEntry(key string, data json.RawMessage) (BunEntry, []BunUnsupportedEntry) {
	entry := BunEntry{
		Key:          key,
		Name:         bunNameFromKey(key),
		Dependencies: map[string]string{},
	}
	var unsupported []BunUnsupportedEntry

	var array []json.RawMessage
	if err := json.Unmarshal(data, &array); err == nil {
		if len(array) == 0 {
			return BunEntry{}, []BunUnsupportedEntry{{Descriptor: key, Reason: "bun.lock package array is empty"}}
		}
		if err := json.Unmarshal(array[0], &entry.Descriptor); err != nil || strings.TrimSpace(entry.Descriptor) == "" {
			return BunEntry{}, []BunUnsupportedEntry{{Descriptor: key, Reason: "bun.lock package descriptor must be a string"}}
		}
		if parsed := parseBunDescriptor(entry.Descriptor); parsed.Name != "" {
			if entry.Name == "" || strings.Contains(key, "@") {
				entry.Name = parsed.Name
			}
			entry.Reference = parsed.Reference
			entry.Protocol = parsed.Protocol
			entry.Range = parsed.Range
		}
		for index, item := range array[1:] {
			if parseBunPackageString(&entry, item, index+1) {
				continue
			}
			if parseBunPackageObject(&entry, item, key, &unsupported) {
				continue
			}
			unsupported = append(unsupported, BunUnsupportedEntry{Descriptor: key, Reason: fmt.Sprintf("unsupported bun.lock package array item at index %d", index+1)})
		}
		finalizeBunEntry(&entry)
		return entry, unsupported
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return BunEntry{}, []BunUnsupportedEntry{{Descriptor: key, Reason: "bun.lock package entry must be an array or object"}}
	}
	if descriptor, ok := stringField(object, "descriptor"); ok {
		entry.Descriptor = descriptor
	} else if descriptor, ok := stringField(object, "resolution"); ok {
		entry.Descriptor = descriptor
	}
	if name, ok := stringField(object, "name"); ok && strings.TrimSpace(name) != "" {
		entry.Name = strings.TrimSpace(name)
	}
	for _, keyName := range []string{"version", "resolution", "resolved", "source", "checksum", "integrity"} {
		value, ok := stringField(object, keyName)
		if !ok {
			continue
		}
		switch keyName {
		case "version":
			entry.Version = value
		case "resolution", "resolved":
			entry.Resolution = value
		case "source":
			entry.Source = value
		case "checksum", "integrity":
			entry.Checksum = value
		}
	}
	if deps, ok := stringMapField(object, "dependencies"); ok {
		entry.Dependencies = deps
	} else if _, ok := object["dependencies"]; ok {
		unsupported = append(unsupported, BunUnsupportedEntry{Descriptor: key, Reason: "dependencies must be a string mapping"})
	}
	finalizeBunEntry(&entry)
	return entry, unsupported
}

func parseBunPackageString(entry *BunEntry, item json.RawMessage, index int) bool {
	var value string
	if err := json.Unmarshal(item, &value); err != nil {
		return false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	switch {
	case looksLikeIntegrity(value):
		entry.Checksum = value
	case entry.Resolution == "":
		entry.Resolution = value
	case entry.Checksum == "":
		entry.Checksum = value
	default:
		if index == 1 && entry.Source == "" {
			entry.Source = value
		}
	}
	return true
}

func parseBunPackageObject(entry *BunEntry, item json.RawMessage, descriptor string, unsupported *[]BunUnsupportedEntry) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(item, &object); err != nil {
		return false
	}
	if deps, ok := stringMapField(object, "dependencies"); ok {
		entry.Dependencies = deps
	} else if _, ok := object["dependencies"]; ok {
		*unsupported = append(*unsupported, BunUnsupportedEntry{Descriptor: descriptor, Reason: "dependencies must be a string mapping"})
	}
	for _, keyName := range []string{"version", "resolution", "resolved", "source", "checksum", "integrity"} {
		value, ok := stringField(object, keyName)
		if !ok {
			continue
		}
		switch keyName {
		case "version":
			entry.Version = value
		case "resolution", "resolved":
			entry.Resolution = value
		case "source":
			entry.Source = value
		case "checksum", "integrity":
			entry.Checksum = value
		}
	}
	return true
}

func finalizeBunEntry(entry *BunEntry) {
	if entry.Name == "" {
		entry.Name = bunNameFromKey(entry.Key)
	}
	if entry.Descriptor == "" {
		entry.Descriptor = entry.Key
	}
	parsed := parseBunDescriptor(entry.Descriptor)
	if entry.Reference == "" {
		entry.Reference = parsed.Reference
	}
	if entry.Protocol == "" {
		entry.Protocol = parsed.Protocol
	}
	if entry.Range == "" {
		entry.Range = parsed.Range
	}
	if entry.Version == "" {
		entry.Version = bunVersionFromDescriptor(entry.Descriptor)
	}
	if entry.Source == "" {
		entry.Source = bunSourceFromReference(entry.Resolution)
	}
	if entry.Source == "" {
		entry.Source = bunSourceFromReference(entry.Reference)
	}
}

func (l BunLockfile) ResolveDirectEntry(name, requirement string) (BunEntry, bool, []BunUnsupportedEntry) {
	requirement = strings.TrimSpace(requirement)
	matches := make([]BunEntry, 0)
	for _, entry := range l.Entries {
		if entry.Name != name {
			continue
		}
		if entry.MatchesRequirement(requirement) {
			return entry, true, nil
		}
		matches = append(matches, entry)
	}
	switch len(matches) {
	case 0:
		return BunEntry{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return BunEntry{}, false, []BunUnsupportedEntry{{
			Descriptor: name,
			Reason:     "ambiguous Bun lockfile entries for direct dependency",
		}}
	}
}

func (e BunEntry) MatchesRequirement(requirement string) bool {
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		return false
	}
	candidates := map[string]struct{}{}
	addBunRequirementCandidate(candidates, e.Reference)
	addBunRequirementCandidate(candidates, e.Range)
	addBunRequirementCandidate(candidates, e.Descriptor)
	addBunRequirementCandidate(candidates, e.Key)
	for _, descriptor := range []string{e.Descriptor, e.Key} {
		parsed := parseBunDescriptor(descriptor)
		addBunRequirementCandidate(candidates, parsed.Reference)
		addBunRequirementCandidate(candidates, parsed.Range)
		if parsed.Protocol != "" && parsed.Range != "" {
			addBunRequirementCandidate(candidates, parsed.Protocol+":"+parsed.Range)
		}
	}
	if e.Protocol != "" && e.Range != "" {
		addBunRequirementCandidate(candidates, e.Protocol+":"+e.Range)
	}
	if strings.HasPrefix(requirement, "npm:") {
		addBunRequirementCandidate(candidates, strings.TrimPrefix(requirement, "npm:"))
	}
	_, ok := candidates[requirement]
	return ok
}

func addBunRequirementCandidate(candidates map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	candidates[value] = struct{}{}
}

func (e BunEntry) SourceForRequirement(requirement string) string {
	for _, value := range []string{requirement, e.Source, e.Reference, e.Resolution} {
		if source := bunSourceFromReference(value); source != "" {
			return source
		}
	}
	return ""
}

func (e BunEntry) ProtocolSource(requirement string) (string, string) {
	for _, value := range []string{requirement, e.Source, e.Reference, e.Resolution} {
		protocol := bunProtocol(value)
		switch protocol {
		case "workspace", "file", "link", "git", "github", "http", "https":
			if source := bunSourceFromReference(value); source != "" {
				return protocol, source
			}
		}
	}
	return "", ""
}

type bunDescriptor struct {
	Name      string
	Reference string
	Protocol  string
	Range     string
}

func parseBunDescriptor(value string) bunDescriptor {
	trimmed := strings.Trim(strings.TrimSpace(value), "\"")
	if trimmed == "" {
		return bunDescriptor{}
	}
	name := trimmed
	reference := ""
	if strings.HasPrefix(trimmed, "@") {
		if slash := strings.Index(trimmed, "/"); slash >= 0 {
			rest := trimmed[slash+1:]
			if at := strings.Index(rest, "@"); at >= 0 {
				name = trimmed[:slash+1+at]
				reference = rest[at+1:]
			}
		}
	} else if at := strings.Index(trimmed, "@"); at >= 0 {
		name = trimmed[:at]
		reference = trimmed[at+1:]
	}
	protocol := bunProtocol(reference)
	rangeValue := reference
	if protocol != "" {
		rangeValue = strings.TrimPrefix(reference, protocol+":")
	}
	return bunDescriptor{
		Name:      strings.TrimSpace(name),
		Reference: strings.TrimSpace(reference),
		Protocol:  protocol,
		Range:     strings.TrimSpace(rangeValue),
	}
}

func bunNameFromKey(key string) string {
	return parseBunDescriptor(key).Name
}

func bunVersionFromDescriptor(descriptor string) string {
	parsed := parseBunDescriptor(descriptor)
	value := parsed.Range
	if parsed.Protocol == "npm" {
		value = parsed.Range
	}
	if looksLikeResolvedVersion(value) {
		return strings.TrimPrefix(value, "v")
	}
	return ""
}

func bunProtocol(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") {
		return "http"
	}
	if strings.HasPrefix(trimmed, "https://") {
		return "https"
	}
	if strings.HasPrefix(trimmed, "git+") || strings.HasPrefix(trimmed, "git://") || strings.HasPrefix(trimmed, "ssh://") {
		return "git"
	}
	if strings.HasPrefix(trimmed, "github:") {
		return "github"
	}
	if index := strings.Index(trimmed, ":"); index > 0 {
		prefix := strings.ToLower(trimmed[:index])
		switch prefix {
		case "npm", "workspace", "file", "link", "git", "github", "http", "https":
			return prefix
		}
	}
	return ""
}

func bunSourceFromReference(value string) string {
	trimmed := strings.TrimSpace(value)
	switch bunProtocol(trimmed) {
	case "workspace", "file", "link", "git", "github":
		return trimmed
	case "http", "https":
		if isDefaultRegistryURL(trimmed) {
			return ""
		}
		return trimmed
	default:
		return ""
	}
}

func isDefaultRegistryURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "registry.npmjs.org/") || strings.Contains(lower, "registry.yarnpkg.com/")
}

func looksLikeIntegrity(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "sha1-") ||
		strings.HasPrefix(lower, "sha256-") ||
		strings.HasPrefix(lower, "sha384-") ||
		strings.HasPrefix(lower, "sha512-")
}

func looksLikeResolvedVersion(value string) bool {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "v")
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, "^~<>=*xX") {
		return false
	}
	first := rune(trimmed[0])
	return first >= '0' && first <= '9'
}

func stringField(object map[string]json.RawMessage, key string) (string, bool) {
	data, ok := object[key]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func stringMapField(object map[string]json.RawMessage, key string) (map[string]string, bool) {
	data, ok := object[key]
	if !ok {
		return nil, false
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}
	if result == nil {
		result = map[string]string{}
	}
	return result, true
}

func sortBunUnsupported(entries []BunUnsupportedEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Descriptor == entries[j].Descriptor {
			return entries[i].Reason < entries[j].Reason
		}
		return entries[i].Descriptor < entries[j].Descriptor
	})
}

func stripJSONC(data []byte) ([]byte, error) {
	var out bytes.Buffer
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/':
				i += 2
				for i < len(data) && data[i] != '\n' && data[i] != '\r' {
					i++
				}
				if i < len(data) {
					out.WriteByte(data[i])
				}
				continue
			case '*':
				i += 2
				closed := false
				for i+1 < len(data) {
					if data[i] == '\n' || data[i] == '\r' {
						out.WriteByte(data[i])
					}
					if data[i] == '*' && data[i+1] == '/' {
						i++
						closed = true
						break
					}
					i++
				}
				if !closed {
					return nil, fmt.Errorf("unterminated block comment")
				}
				continue
			}
		}
		out.WriteByte(ch)
	}
	if inString {
		return nil, fmt.Errorf("unterminated string")
	}
	return out.Bytes(), nil
}

func removeTrailingJSONCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(data) && unicode.IsSpace(rune(data[j])) {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
		}
		out = append(out, ch)
	}
	return out
}
