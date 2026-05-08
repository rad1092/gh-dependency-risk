package gomod

import "testing"

func TestParseSumFile(t *testing.T) {
	sum := ParseSumFile([]byte(`
example.com/lib v1.2.3 h1:abc
example.com/lib v1.2.3/go.mod h1:def
bad line
`))
	if len(sum.Entries) != 2 {
		t.Fatalf("expected checksum entries, got %#v", sum.Entries)
	}
	if len(sum.Unsupported) != 1 || sum.Unsupported[0].Line != 4 {
		t.Fatalf("expected unsupported bad line, got %#v", sum.Unsupported)
	}
}
