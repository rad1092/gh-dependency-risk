package gomod

import "strings"

type Manifest struct {
	ModulePath   string
	GoVersion    string
	Toolchain    string
	Requirements []Requirement
	Replacements []Replacement
}

type Requirement struct {
	Path     string
	Version  string
	Indirect bool
}

type Replacement struct {
	OldPath    string
	OldVersion string
	NewPath    string
	NewVersion string
	Local      bool
}

type SumFile struct {
	Entries     []SumEntry
	Unsupported []UnsupportedEntry
}

type SumEntry struct {
	Module  string
	Version string
	Hash    string
	Raw     string
}

type UnsupportedEntry struct {
	Line   int
	Text   string
	Reason string
}

func clean(value string) string {
	return strings.TrimSpace(value)
}
