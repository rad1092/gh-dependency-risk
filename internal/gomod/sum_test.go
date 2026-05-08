package gomod

import "testing"

func TestParseSumFile(t *testing.T) {
	sum := ParseSumFile([]byte(`
example.com/lib v1.2.3/go.mod h1:def
example.com/lib v1.2.3 h1:abc
bad line
`))
	if len(sum.Entries) != 2 {
		t.Fatalf("expected checksum entries, got %#v", sum.Entries)
	}
	if sum.Entries[0].Raw != "example.com/lib v1.2.3 h1:abc" || sum.Entries[1].Raw != "example.com/lib v1.2.3/go.mod h1:def" {
		t.Fatalf("expected deterministic checksum sorting, got %#v", sum.Entries)
	}
	if len(sum.Unsupported) != 1 || sum.Unsupported[0].Line != 4 {
		t.Fatalf("expected unsupported bad line, got %#v", sum.Unsupported)
	}
}

func TestParseSumFileUnsupportedOnly(t *testing.T) {
	sum := ParseSumFile([]byte("bad-checksum-line\n"))
	if len(sum.Entries) != 0 {
		t.Fatalf("did not expect checksum entries from unsupported line, got %#v", sum.Entries)
	}
	if len(sum.Unsupported) != 1 {
		t.Fatalf("expected unsupported checksum line, got %#v", sum.Unsupported)
	}
}
