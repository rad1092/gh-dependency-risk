package npm

import "testing"

func TestParseYarnBerryLockfileParsesModernEntries(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8
  cacheKey: 10

"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
  checksum: 10c0
  languageName: node
  linkType: hard
  dependencies:
    repeat-string: "npm:^1.0.0"
  peerDependencies:
    react: "npm:^18.0.0"

"alias@npm:left-pad@^1.0.0":
  version: 1.0.1
  resolution: "alias@npm:1.0.1"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	if !lockfile.HasMetadata {
		t.Fatalf("expected __metadata to be detected")
	}
	if len(lockfile.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %#v", lockfile.Entries)
	}
	leftPad, ok := lockfile.FindEntry("left-pad", "^1.0.0")
	if !ok {
		t.Fatalf("expected left-pad entry")
	}
	if leftPad.Version != "1.0.1" || leftPad.Resolution != "left-pad@npm:1.0.1" || leftPad.Checksum != "10c0" {
		t.Fatalf("unexpected left-pad entry: %#v", leftPad)
	}
	if leftPad.Dependencies["repeat-string"] != "npm:^1.0.0" || leftPad.PeerDependencies["react"] != "npm:^18.0.0" {
		t.Fatalf("expected dependencies and peerDependencies, got %#v", leftPad)
	}
	alias, ok := lockfile.FindEntry("alias", "npm:left-pad@^1.0.0")
	if !ok || alias.Name != "alias" {
		t.Fatalf("expected npm alias entry, got %#v", alias)
	}
}

func TestYarnBerryResolveDirectEntryExactMatchBeatsSameNameCandidates(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"

"left-pad@npm:^2.0.0":
  version: 2.0.0
  resolution: "left-pad@npm:2.0.0"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^2.0.0")
	if !ok || entry.Version != "2.0.0" || len(unsupported) != 0 {
		t.Fatalf("expected exact match for ^2.0.0, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
}

func TestYarnBerryResolveDirectEntryAmbiguousSameNameDoesNotGuess(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"

"left-pad@npm:^2.0.0":
  version: 2.0.0
  resolution: "left-pad@npm:2.0.0"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^3.0.0")
	if ok || entry.Version != "" || len(unsupported) != 1 {
		t.Fatalf("expected ambiguous same-name match to stay unresolved, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
	if unsupported[0].Reason != "ambiguous Yarn Berry lockfile entries for direct dependency" {
		t.Fatalf("unexpected unsupported reason: %#v", unsupported)
	}
}

func TestYarnBerryResolveDirectEntryVirtualCandidateKeepsNameFallbackAmbiguous(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"

"left-pad@virtual:abc123#npm:^2.0.0":
  version: 2.0.0
  resolution: "left-pad@virtual:abc123#npm:^2.0.0"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^3.0.0")
	if ok || entry.Version != "" || len(unsupported) != 1 {
		t.Fatalf("expected virtual same-name candidate to prevent name-only guessing, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
	if unsupported[0].Reason != "ambiguous Yarn Berry lockfile entries for direct dependency" {
		t.Fatalf("unexpected unsupported reason: %#v", unsupported)
	}
}

func TestYarnBerryResolveDirectEntrySingleNameFallback(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"left-pad@npm:^1.0.0":
  version: 1.0.1
  resolution: "left-pad@npm:1.0.1"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	entry, ok, unsupported := lockfile.ResolveDirectEntry("left-pad", "^9.0.0")
	if !ok || entry.Version != "1.0.1" || len(unsupported) != 0 {
		t.Fatalf("expected single same-name fallback, got entry=%#v ok=%v unsupported=%#v", entry, ok, unsupported)
	}
}

func TestParseYarnBerryLockfileDescriptorEdgeCases(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"@scope/pkg@npm:^1.0.0":
  version: 1.0.1
  resolution: "@scope/pkg@npm:1.0.1"

"alias-name@npm:real-name@^1.0.0":
  version: 1.0.1
  resolution: "alias-name@npm:1.0.1"

"multi@npm:^1.0.0, multi@npm:^1.1.0":
  version: 1.1.0
  resolution: "multi@npm:1.1.0"

"virtual-lib@virtual:abc123#npm:^1.0.0":
  version: 1.0.0
  resolution: "virtual-lib@virtual:abc123#npm:^1.0.0"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	scoped, ok, unsupported := lockfile.ResolveDirectEntry("@scope/pkg", "^1.0.0")
	if !ok || scoped.Name != "@scope/pkg" || len(unsupported) != 0 {
		t.Fatalf("expected scoped package match, got entry=%#v ok=%v unsupported=%#v", scoped, ok, unsupported)
	}
	alias, ok, unsupported := lockfile.ResolveDirectEntry("alias-name", "npm:real-name@^1.0.0")
	if !ok || alias.Name != "alias-name" || len(unsupported) != 0 {
		t.Fatalf("expected alias declared name match, got entry=%#v ok=%v unsupported=%#v", alias, ok, unsupported)
	}
	multi, ok, unsupported := lockfile.ResolveDirectEntry("multi", "^1.1.0")
	if !ok || multi.Version != "1.1.0" || len(unsupported) != 0 {
		t.Fatalf("expected comma-separated descriptor match, got entry=%#v ok=%v unsupported=%#v", multi, ok, unsupported)
	}
	virtual, ok, unsupported := lockfile.ResolveDirectEntry("virtual-lib", "^1.0.0")
	if ok || virtual.Version != "" || len(unsupported) != 1 {
		t.Fatalf("expected virtual descriptor to stay unsupported, got entry=%#v ok=%v unsupported=%#v", virtual, ok, unsupported)
	}
}

func TestParseYarnBerryLockfileProtocols(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"workspace-lib@workspace:*":
  version: 0.0.0-use.local
  resolution: "workspace-lib@workspace:*"

"patched@patch:patched@npm%3A1.0.0#./patches/patched.patch":
  version: 1.0.0
  resolution: "patched@patch:patched@npm%3A1.0.0#./patches/patched.patch"

"portal-lib@portal:../portal-lib":
  version: 0.0.0-use.local
  resolution: "portal-lib@portal:../portal-lib"

"linked-lib@link:../linked-lib":
  version: 0.0.0-use.local
  resolution: "linked-lib@link:../linked-lib"

"file-lib@file:../file-lib.tgz":
  version: 1.0.0
  resolution: "file-lib@file:../file-lib.tgz"

"git-lib@git:https://github.com/acme/git-lib.git#commit=abc":
  version: 1.0.0
  resolution: "git-lib@git:https://github.com/acme/git-lib.git#commit=abc"

"http-lib@https://example.com/http-lib.tgz":
  version: 1.0.0
  resolution: "http-lib@https://example.com/http-lib.tgz"
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	tests := map[string]string{
		"workspace-lib": "workspace",
		"patched":       "patch",
		"portal-lib":    "portal",
		"linked-lib":    "link",
		"file-lib":      "file",
		"git-lib":       "git",
		"http-lib":      "https",
	}
	for name, wantProtocol := range tests {
		entry, ok := lockfile.FindEntry(name, "")
		if !ok {
			t.Fatalf("expected entry for %s", name)
		}
		protocol, source := entry.ProtocolSource("")
		if protocol != wantProtocol || source == "" {
			t.Fatalf("expected %s protocol for %s, got protocol=%q source=%q", wantProtocol, name, protocol, source)
		}
	}
}

func TestParseYarnBerryLockfileUnsupportedShape(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"bad@npm:^1.0.0":
  - unsupported

"bad-deps@npm:^1.0.0":
  version: 1.0.0
  dependencies:
    broken:
      nested: true
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	if len(lockfile.Unsupported) != 2 {
		t.Fatalf("expected unsupported entries, got %#v", lockfile.Unsupported)
	}
}

func TestParseYarnBerryLockfileSortsDeterministically(t *testing.T) {
	lockfile, err := ParseYarnBerryLockfile([]byte(`
__metadata:
  version: 8

"zeta@npm:^1.0.0":
  version: 1.0.0

"alpha@npm:^1.0.0":
  version: 1.0.0
`))
	if err != nil {
		t.Fatalf("parse Yarn Berry lockfile: %v", err)
	}
	if got := []string{lockfile.Entries[0].Name, lockfile.Entries[1].Name}; got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("expected sorted entries, got %#v", got)
	}
}

func TestParseYarnRCNodeLinker(t *testing.T) {
	yarnRC, err := ParseYarnRC([]byte("nodeLinker: pnp\n"))
	if err != nil {
		t.Fatalf("parse yarnrc: %v", err)
	}
	if yarnRC.NodeLinker != "pnp" {
		t.Fatalf("expected nodeLinker, got %#v", yarnRC)
	}
}
