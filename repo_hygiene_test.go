package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicDocsUseCurrentRepositorySlugForBadges(t *testing.T) {
	currentRemoteSlug := "rad1092/gh-dependency-risk"

	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	for _, want := range []string{
		"https://github.com/" + currentRemoteSlug + "/actions/workflows/test.yml",
		"https://github.com/" + currentRemoteSlug + "/actions/workflows/install-smoke.yml",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("README.md should reference the current remote repository slug in badge %q", want)
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
		"gh extension install rad1092/gh-dep-risk --force",
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

func TestManualWorkflowDoesNotRequireCrossRepoCommentToken(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".github", "workflows", "dep-risk-manual.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	want := "GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}"
	if !strings.Contains(content, want) {
		t.Fatalf("dep-risk-manual.yml missing default GitHub token %q", want)
	}

	if strings.Contains(content, "DEP_RISK_GH_TOKEN") {
		t.Fatal("dep-risk-manual.yml should not require a cross-repo comment PAT secret")
	}

	if !strings.Contains(content, "comment mode is limited to PRs in $DEFAULT_REPO") {
		t.Fatal("dep-risk-manual.yml should guard against cross-repo comment mode")
	}
}
