package npm

import (
	"strings"
	"testing"
)

func TestParseBunLockfileParsesDirectEntries(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  // bun.lock is JSONC
  "lockfileVersion": 1,
  "workspaces": {
    "": {
      "dependencies": {
        "left-pad": "^1.0.0",
      },
    },
  },
  "packages": {
    "left-pad": ["left-pad@1.0.1", "", { "dependencies": { "repeat-string": "^1.0.0" } }, "sha512-left"],
    "workspace-lib": ["workspace-lib@workspace:*", { "dependencies": {} }],
    "file-lib": ["file-lib@file:../file-lib.tgz", "file:../file-lib.tgz"],
    "link-lib": ["link-lib@link:../link-lib", "link:../link-lib"],
    "git-lib": ["git-lib@git:https://github.com/acme/git-lib.git", "git:https://github.com/acme/git-lib.git"],
    "github-lib": ["github-lib@github:acme/github-lib", "github:acme/github-lib"],
    "http-lib": ["http-lib@https://example.com/http-lib.tgz", "https://example.com/http-lib.tgz"],
  },
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	if lockfile.LockfileVersion != 1 {
		t.Fatalf("expected lockfile version, got %#v", lockfile)
	}
	left, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^1.0.0")
	if !ok || left.Version != "1.0.1" || left.Checksum != "sha512-left" || len(left.Dependencies) != 1 || len(unsupported) != 0 {
		t.Fatalf("unexpected left-pad entry=%#v ok=%v unsupported=%#v", left, ok, unsupported)
	}
	for _, tc := range []struct {
		name        string
		requirement string
		wantSource  string
	}{
		{"workspace-lib", "workspace:*", "workspace:*"},
		{"file-lib", "file:../file-lib.tgz", "file:../file-lib.tgz"},
		{"link-lib", "link:../link-lib", "link:../link-lib"},
		{"git-lib", "git:https://github.com/acme/git-lib.git", "git:https://github.com/acme/git-lib.git"},
		{"github-lib", "github:acme/github-lib", "github:acme/github-lib"},
		{"http-lib", "https://example.com/http-lib.tgz", "https://example.com/http-lib.tgz"},
	} {
		entry, ok, unsupported := lockfile.ResolveDirectEntry(tc.name, tc.requirement)
		if !ok || len(unsupported) != 0 {
			t.Fatalf("expected %s to resolve, got entry=%#v ok=%v unsupported=%#v", tc.name, entry, ok, unsupported)
		}
		if source := entry.SourceForRequirement(tc.requirement); source != tc.wantSource {
			t.Fatalf("expected source %q for %s, got %q", tc.wantSource, tc.name, source)
		}
	}
}

func TestParseBunLockfileJSONCRobustness(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  // line comment outside strings
  "lockfileVersion": 1,
  /* block comment outside strings */
  "packages": {
    "url-lib": [
      "url-lib@https://example.com/pkg//artifact.tgz?literal=/*not-comment*/",
      "https://example.com/pkg//artifact.tgz?literal=/*not-comment*/",
    ],
    "escaped-lib": [
      "escaped-lib@file:C:\\tmp\\\"quoted\".tgz",
      "file:C:\\tmp\\\"quoted\".tgz",
    ],
  },
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}

	urlEntry, ok, unsupported := lockfile.ResolveDirectEntry("url-lib", "https://example.com/pkg//artifact.tgz?literal=/*not-comment*/")
	if !ok || len(unsupported) != 0 {
		t.Fatalf("expected URL entry to resolve, got entry=%#v ok=%v unsupported=%#v", urlEntry, ok, unsupported)
	}
	if source := urlEntry.SourceForRequirement("https://example.com/pkg//artifact.tgz?literal=/*not-comment*/"); source != "https://example.com/pkg//artifact.tgz?literal=/*not-comment*/" {
		t.Fatalf("expected URL string content to be preserved, got %q", source)
	}

	escapedEntry, ok, unsupported := lockfile.ResolveDirectEntry("escaped-lib", `file:C:\tmp\"quoted".tgz`)
	if !ok || len(unsupported) != 0 {
		t.Fatalf("expected escaped entry to resolve, got entry=%#v ok=%v unsupported=%#v", escapedEntry, ok, unsupported)
	}
	if source := escapedEntry.SourceForRequirement(`file:C:\tmp\"quoted".tgz`); !strings.Contains(source, `"quoted"`) {
		t.Fatalf("expected escaped quote/backslash string content to be preserved, got %q", source)
	}
}

func TestParseBunLockfileObjectEntryAndRegistrySource(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  "lockfileVersion": 1,
  "packages": {
    "@scope/pkg": {
      "descriptor": "@scope/pkg@npm:2.0.0",
      "version": "2.0.0",
      "resolved": "https://registry.npmjs.org/@scope/pkg/-/pkg-2.0.0.tgz",
      "integrity": "sha512-registry"
    }
  }
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("@scope/pkg", "^2.0.0")
	if !ok || entry.Version != "2.0.0" || len(unsupported) != 0 {
		t.Fatalf("unexpected scoped entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
	if source := entry.SourceForRequirement("^2.0.0"); source != "" {
		t.Fatalf("expected default registry URL not to become non-registry source, got %q", source)
	}
}

func TestBunResolveDirectEntrySingleSameNameFallback(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  "lockfileVersion": 1,
  "packages": {
    "left-pad@npm:^1.0.0": ["left-pad@1.0.1", ""]
  }
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^2.0.0")
	if !ok || entry.Version != "1.0.1" || len(unsupported) != 0 {
		t.Fatalf("expected single same-name fallback, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
}

func TestBunResolveDirectEntryAmbiguousSameNameDoesNotGuess(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  "lockfileVersion": 1,
  "packages": {
    "left-pad@npm:^1.0.0": ["left-pad@1.0.1", ""],
    "left-pad@npm:^2.0.0": ["left-pad@2.0.0", ""]
  }
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^3.0.0")
	if ok || entry.Version != "" || len(unsupported) != 1 {
		t.Fatalf("expected ambiguous direct match not to guess, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
	if unsupported[0].Reason != "ambiguous Bun lockfile entries for direct dependency" {
		t.Fatalf("unexpected unsupported reason: %#v", unsupported)
	}
}

func TestBunResolveDirectEntryPrefersExactDescriptorMatch(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  "lockfileVersion": 1,
  "packages": {
    "left-pad@npm:^1.0.0": ["left-pad@1.0.1", ""],
    "left-pad@npm:^2.0.0": ["left-pad@2.0.0", ""]
  }
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^2.0.0")
	if !ok || entry.Version != "2.0.0" || len(unsupported) != 0 {
		t.Fatalf("expected exact descriptor match, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
}

func TestParseBunLockfileUnsupportedShapeAndDeterministicSort(t *testing.T) {
	lockfile, err := ParseBunLockfile([]byte(`{
  "lockfileVersion": 1,
  "packages": {
    "z-lib": ["z-lib@1.0.0", ""],
    "bad-z": "not an entry",
    "bad-a": [],
    "bad-m": [42],
    "bad-deps": { "descriptor": "bad-deps@1.0.0", "dependencies": "nope" },
    "a-lib": ["a-lib@1.0.0", ""]
  }
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	if len(lockfile.Unsupported) != 4 {
		t.Fatalf("expected unsupported bad entries, got %#v", lockfile.Unsupported)
	}
	for i, expected := range []string{"bad-a", "bad-deps", "bad-m", "bad-z"} {
		if lockfile.Unsupported[i].Descriptor != expected {
			t.Fatalf("expected deterministic unsupported entry %d to be %q, got %#v", i, expected, lockfile.Unsupported)
		}
	}
	if len(lockfile.Entries) != 3 || lockfile.Entries[0].Name != "a-lib" || lockfile.Entries[1].Name != "bad-deps" || lockfile.Entries[2].Name != "z-lib" {
		t.Fatalf("expected deterministic sorted entries, got %#v", lockfile.Entries)
	}
}

func TestParseBunLockfileMalformedJSONC(t *testing.T) {
	if _, err := ParseBunLockfile([]byte(`{"packages": { /* nope `)); err == nil || !strings.Contains(err.Error(), "parse bun.lock") {
		t.Fatalf("expected malformed JSONC error")
	}
	if _, err := ParseBunLockfile([]byte(`{"packages": {"unterminated": ["unterminated@file:\"}`)); err == nil || !strings.Contains(err.Error(), "parse bun.lock") {
		t.Fatalf("expected malformed string error, got %v", err)
	}
}
