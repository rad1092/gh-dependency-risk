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
