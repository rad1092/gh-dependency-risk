package npm

import "testing"

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
    "bad": "not an entry",
    "a-lib": ["a-lib@1.0.0", ""]
  }
}`))
	if err != nil {
		t.Fatalf("parse bun.lock: %v", err)
	}
	if len(lockfile.Unsupported) != 1 || lockfile.Unsupported[0].Descriptor != "bad" {
		t.Fatalf("expected unsupported bad entry, got %#v", lockfile.Unsupported)
	}
	if len(lockfile.Entries) != 2 || lockfile.Entries[0].Name != "a-lib" || lockfile.Entries[1].Name != "z-lib" {
		t.Fatalf("expected deterministic sorted entries, got %#v", lockfile.Entries)
	}
}

func TestParseBunLockfileMalformedJSONC(t *testing.T) {
	if _, err := ParseBunLockfile([]byte(`{"packages": { /* nope `)); err == nil {
		t.Fatalf("expected malformed JSONC error")
	}
}
