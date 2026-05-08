package gomod

import (
	"bufio"
	"strings"
)

func ParseSumFile(data []byte) SumFile {
	var result SumFile
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) != 3 {
			result.Unsupported = append(result.Unsupported, UnsupportedEntry{
				Line:   lineNumber,
				Text:   raw,
				Reason: "go.sum checksum line must contain module, version, and hash fields",
			})
			continue
		}
		result.Entries = append(result.Entries, SumEntry{
			Module:  fields[0],
			Version: fields[1],
			Hash:    fields[2],
			Raw:     strings.Join(fields, " "),
		})
	}
	sortSumEntries(result.Entries)
	return result
}

func sortSumEntries(entries []SumEntry) {
	for i := 1; i < len(entries); i++ {
		current := entries[i]
		j := i - 1
		for j >= 0 && entries[j].Raw > current.Raw {
			entries[j+1] = entries[j]
			j--
		}
		entries[j+1] = current
	}
}

func sumEntrySet(sum SumFile) map[string]struct{} {
	set := map[string]struct{}{}
	for _, entry := range sum.Entries {
		if entry.Raw == "" {
			continue
		}
		set[entry.Raw] = struct{}{}
	}
	return set
}
