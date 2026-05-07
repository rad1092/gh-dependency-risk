package python

type Scope string

const (
	ScopeRuntime  Scope = "runtime"
	ScopeOptional Scope = "optional"
)

type Dependency struct {
	Name        string
	Requirement string
	Version     string
	Source      string
	Scope       Scope
	Raw         string
	Line        int
}

type UnsupportedEntry struct {
	Line   int
	Text   string
	Reason string
}

type ParseResult struct {
	Dependencies []Dependency
	Unsupported  []UnsupportedEntry
}
