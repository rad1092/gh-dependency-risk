package python

type Scope string

const (
	ScopeRuntime  Scope = "runtime"
	ScopeDev      Scope = "dev"
	ScopeOptional Scope = "optional"
	ScopeUnknown  Scope = "unknown"
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

type LockPackage struct {
	Name            string
	Version         string
	Category        string
	Groups          []string
	Optional        bool
	Source          string
	SourceType      string
	SourceURL       string
	SourceReference string
}

type Lockfile struct {
	Packages    []LockPackage
	Unsupported []UnsupportedEntry
}
