package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicDocsUseCurrentRepositorySlug(t *testing.T) {
	oldRemoteSlug := "rad1092/" + "gh-dep-risk"
	currentRemoteSlug := "rad1092/gh-dependency-risk"

	files := []string{
		"README.md",
		"RELEASING.md",
		filepath.Join("docs", "smoke-test.md"),
		filepath.Join(".github", "workflows", "install-smoke.yml"),
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		content := string(data)
		if strings.Contains(content, oldRemoteSlug) {
			t.Fatalf("%s still references the old remote repository slug", file)
		}
		if !strings.Contains(content, currentRemoteSlug) {
			t.Fatalf("%s should reference the current remote repository slug", file)
		}
	}
}

func TestInstallSmokeKeepsStableCommandName(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".github", "workflows", "install-smoke.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	for _, want := range []string{
		"gh extension install rad1092/gh-dependency-risk --force",
		"gh dep-risk version",
		"gh dep-risk pr 1 --repo rad1092/ascii-diagram-editor",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("install-smoke.yml missing %q", want)
		}
	}

	if strings.Contains(content, "gh "+"dependency-risk") {
		t.Fatal("install-smoke.yml should keep the stable command name gh dep-risk")
	}
}
