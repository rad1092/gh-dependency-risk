package npm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type YarnBerryLockfile struct {
	Entries     []YarnBerryEntry
	Unsupported []YarnBerryUnsupportedEntry
	HasMetadata bool
}

type YarnBerryEntry struct {
	Descriptor       string
	Descriptors      []string
	Name             string
	Reference        string
	Protocol         string
	Range            string
	Version          string
	Resolution       string
	Checksum         string
	Dependencies     map[string]string
	PeerDependencies map[string]string
	LinkType         string
	LanguageName     string
}

type YarnBerryUnsupportedEntry struct {
	Descriptor string
	Reason     string
}

type YarnRC struct {
	NodeLinker  string
	Unsupported []YarnBerryUnsupportedEntry
}

func LooksLikeYarnBerryLockfile(data []byte) bool {
	return strings.Contains(string(data), "__metadata:")
}

func IsYarnBerryPackageManager(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(lower, "yarn@") {
		return false
	}
	version := strings.TrimPrefix(lower, "yarn@")
	majorText := version
	if index := strings.IndexAny(majorText, ".-+"); index >= 0 {
		majorText = majorText[:index]
	}
	major, err := strconv.Atoi(majorText)
	return err == nil && major >= 2
}

func ParseYarnBerryLockfile(data []byte) (YarnBerryLockfile, error) {
	result := YarnBerryLockfile{}
	if strings.TrimSpace(string(data)) == "" {
		return result, nil
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return result, fmt.Errorf("parse Yarn Berry yarn.lock: %w", err)
	}
	root := &document
	if document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		root = document.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return result, fmt.Errorf("parse Yarn Berry yarn.lock: expected top-level mapping")
	}

	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		descriptor := strings.TrimSpace(keyNode.Value)
		if descriptor == "" {
			result.Unsupported = append(result.Unsupported, YarnBerryUnsupportedEntry{Reason: "empty Yarn Berry lockfile descriptor"})
			continue
		}
		if descriptor == "__metadata" {
			result.HasMetadata = true
			continue
		}
		if valueNode.Kind != yaml.MappingNode {
			result.Unsupported = append(result.Unsupported, YarnBerryUnsupportedEntry{
				Descriptor: descriptor,
				Reason:     "Yarn Berry lockfile entry must be a mapping",
			})
			continue
		}
		entry, unsupported := parseYarnBerryEntry(descriptor, valueNode)
		result.Unsupported = append(result.Unsupported, unsupported...)
		if strings.TrimSpace(entry.Name) == "" {
			result.Unsupported = append(result.Unsupported, YarnBerryUnsupportedEntry{
				Descriptor: descriptor,
				Reason:     "Yarn Berry lockfile entry has no stable package name",
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
	sort.Slice(result.Unsupported, func(i, j int) bool {
		if result.Unsupported[i].Descriptor == result.Unsupported[j].Descriptor {
			return result.Unsupported[i].Reason < result.Unsupported[j].Reason
		}
		return result.Unsupported[i].Descriptor < result.Unsupported[j].Descriptor
	})
	return result, nil
}

func parseYarnBerryEntry(descriptor string, node *yaml.Node) (YarnBerryEntry, []YarnBerryUnsupportedEntry) {
	entry := YarnBerryEntry{
		Descriptor:       descriptor,
		Descriptors:      splitYarnBerryDescriptors(descriptor),
		Dependencies:     map[string]string{},
		PeerDependencies: map[string]string{},
	}
	if len(entry.Descriptors) == 0 {
		entry.Descriptors = []string{descriptor}
	}
	first := parseYarnBerryDescriptor(entry.Descriptors[0])
	entry.Name = first.Name
	entry.Reference = first.Reference
	entry.Protocol = first.Protocol
	entry.Range = first.Range

	unsupported := make([]YarnBerryUnsupportedEntry, 0)
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := strings.TrimSpace(node.Content[index].Value)
		value := node.Content[index+1]
		switch key {
		case "version":
			entry.Version = scalarString(value)
		case "resolution":
			entry.Resolution = scalarString(value)
		case "checksum":
			entry.Checksum = scalarString(value)
		case "linkType":
			entry.LinkType = scalarString(value)
		case "languageName":
			entry.LanguageName = scalarString(value)
		case "dependencies":
			deps, ok := yarnBerryStringMap(value)
			if !ok {
				unsupported = append(unsupported, YarnBerryUnsupportedEntry{Descriptor: descriptor, Reason: "dependencies must be a mapping"})
				continue
			}
			entry.Dependencies = deps
		case "peerDependencies":
			deps, ok := yarnBerryStringMap(value)
			if !ok {
				unsupported = append(unsupported, YarnBerryUnsupportedEntry{Descriptor: descriptor, Reason: "peerDependencies must be a mapping"})
				continue
			}
			entry.PeerDependencies = deps
		}
	}
	if entry.Version == "" {
		entry.Version = yarnBerryVersionFromResolution(entry.Resolution)
	}
	return entry, unsupported
}

func ParseYarnRC(data []byte) (YarnRC, error) {
	result := YarnRC{}
	if strings.TrimSpace(string(data)) == "" {
		return result, nil
	}
	var raw struct {
		NodeLinker any `yaml:"nodeLinker"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		result.Unsupported = append(result.Unsupported, YarnBerryUnsupportedEntry{
			Descriptor: ".yarnrc.yml",
			Reason:     "could not parse .yarnrc.yml",
		})
		return result, nil
	}
	if raw.NodeLinker == nil {
		return result, nil
	}
	if value, ok := raw.NodeLinker.(string); ok {
		result.NodeLinker = strings.TrimSpace(value)
		return result, nil
	}
	result.Unsupported = append(result.Unsupported, YarnBerryUnsupportedEntry{
		Descriptor: ".yarnrc.yml",
		Reason:     "nodeLinker must be a string",
	})
	return result, nil
}

func (l YarnBerryLockfile) FindEntry(name, requirement string) (YarnBerryEntry, bool) {
	requirement = strings.TrimSpace(requirement)
	matches := make([]YarnBerryEntry, 0)
	for _, entry := range l.Entries {
		if entry.Name != name {
			continue
		}
		if entry.MatchesRequirement(requirement) {
			return entry, true
		}
		matches = append(matches, entry)
	}
	if len(matches) == 0 {
		return YarnBerryEntry{}, false
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Descriptor == matches[j].Descriptor {
			return matches[i].Resolution < matches[j].Resolution
		}
		return matches[i].Descriptor < matches[j].Descriptor
	})
	return matches[0], true
}

func (l YarnBerryLockfile) ResolveDirectEntry(name, requirement string) (YarnBerryEntry, bool, []YarnBerryUnsupportedEntry) {
	requirement = strings.TrimSpace(requirement)
	matches := make([]YarnBerryEntry, 0)
	virtualMatches := make([]YarnBerryEntry, 0)
	for _, entry := range l.Entries {
		if entry.Name != name {
			continue
		}
		if entry.MatchesRequirement(requirement) && !entry.HasVirtualDescriptor() {
			return entry, true, nil
		}
		if entry.HasVirtualDescriptor() {
			virtualMatches = append(virtualMatches, entry)
			continue
		}
		matches = append(matches, entry)
	}
	switch len(matches) {
	case 0:
		if len(virtualMatches) > 0 {
			return YarnBerryEntry{}, false, []YarnBerryUnsupportedEntry{{
				Descriptor: name,
				Reason:     "Yarn Berry virtual lockfile entries cannot be matched to direct dependency declarations",
			}}
		}
		return YarnBerryEntry{}, false, nil
	case 1:
		if len(virtualMatches) > 0 {
			return YarnBerryEntry{}, false, []YarnBerryUnsupportedEntry{{
				Descriptor: name,
				Reason:     "ambiguous Yarn Berry lockfile entries for direct dependency",
			}}
		}
		return matches[0], true, nil
	default:
		return YarnBerryEntry{}, false, []YarnBerryUnsupportedEntry{{
			Descriptor: name,
			Reason:     "ambiguous Yarn Berry lockfile entries for direct dependency",
		}}
	}
}

func (e YarnBerryEntry) MatchesRequirement(requirement string) bool {
	if requirement == "" {
		return false
	}
	candidates := map[string]struct{}{
		e.Reference: {},
		e.Range:     {},
	}
	if e.Protocol != "" && e.Range != "" {
		candidates[e.Protocol+":"+e.Range] = struct{}{}
	}
	if strings.HasPrefix(requirement, "npm:") {
		candidates[strings.TrimPrefix(requirement, "npm:")] = struct{}{}
	}
	for _, descriptor := range e.Descriptors {
		parsed := parseYarnBerryDescriptor(descriptor)
		candidates[parsed.Reference] = struct{}{}
		candidates[parsed.Range] = struct{}{}
		if parsed.Protocol != "" && parsed.Range != "" {
			candidates[parsed.Protocol+":"+parsed.Range] = struct{}{}
		}
	}
	_, ok := candidates[requirement]
	return ok
}

func (e YarnBerryEntry) HasVirtualDescriptor() bool {
	if yarnBerryProtocol(e.Reference) == "virtual" || yarnBerryProtocol(e.Resolution) == "virtual" {
		return true
	}
	for _, descriptor := range e.Descriptors {
		parsed := parseYarnBerryDescriptor(descriptor)
		if parsed.Protocol == "virtual" {
			return true
		}
	}
	return false
}

func (e YarnBerryEntry) Source(requirement string) string {
	if source := yarnBerrySourceFromReference(requirement); source != "" {
		return source
	}
	if source := yarnBerrySourceFromReference(e.Reference); source != "" {
		return source
	}
	return yarnBerrySourceFromReference(e.Resolution)
}

func (e YarnBerryEntry) ProtocolSource(requirement string) (string, string) {
	for _, value := range []string{requirement, e.Reference, e.Resolution} {
		protocol := yarnBerryProtocol(value)
		if protocol == "" {
			continue
		}
		switch protocol {
		case "workspace", "portal", "link", "file", "patch", "git", "github", "http", "https":
			return protocol, value
		}
	}
	return "", ""
}

func (e YarnBerryEntry) IsRegistryLike(requirement string) bool {
	for _, value := range []string{requirement, e.Reference, e.Resolution} {
		protocol := yarnBerryProtocol(value)
		if protocol == "" || protocol == "npm" {
			continue
		}
		return false
	}
	return true
}

type yarnBerryDescriptor struct {
	Name      string
	Reference string
	Protocol  string
	Range     string
}

func splitYarnBerryDescriptors(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := strings.Trim(strings.TrimSpace(part), "\"")
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}

func parseYarnBerryDescriptor(value string) yarnBerryDescriptor {
	trimmed := strings.Trim(strings.TrimSpace(value), "\"")
	if trimmed == "" {
		return yarnBerryDescriptor{}
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
	protocol := yarnBerryProtocol(reference)
	rangeValue := reference
	if protocol != "" {
		rangeValue = strings.TrimPrefix(reference, protocol+":")
	}
	return yarnBerryDescriptor{
		Name:      strings.TrimSpace(name),
		Reference: strings.TrimSpace(reference),
		Protocol:  protocol,
		Range:     strings.TrimSpace(rangeValue),
	}
}

func yarnBerryStringMap(node *yaml.Node) (map[string]string, bool) {
	if node.Kind != yaml.MappingNode {
		return nil, false
	}
	result := map[string]string{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := strings.TrimSpace(node.Content[index].Value)
		if key == "" {
			return nil, false
		}
		valueNode := node.Content[index+1]
		if valueNode.Kind != yaml.ScalarNode {
			return nil, false
		}
		result[key] = strings.TrimSpace(valueNode.Value)
	}
	return result, true
}

func scalarString(node *yaml.Node) string {
	if node.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func yarnBerryVersionFromResolution(resolution string) string {
	parsed := parseYarnBerryDescriptor(resolution)
	if parsed.Protocol == "npm" && parsed.Range != "" && !strings.Contains(parsed.Range, "@") {
		return parsed.Range
	}
	return ""
}

func yarnBerryProtocol(value string) string {
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
		case "npm", "workspace", "portal", "link", "file", "patch", "git", "github", "http", "https", "virtual":
			return prefix
		}
	}
	return ""
}

func yarnBerrySourceFromReference(value string) string {
	protocol := yarnBerryProtocol(value)
	switch protocol {
	case "workspace", "portal", "link", "file", "patch", "git", "github", "http", "https":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
