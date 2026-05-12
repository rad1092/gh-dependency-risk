package hygiene_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicDocsUseCurrentRepositorySlugForBadges(t *testing.T) {
	root := repoRoot(t)
	currentRemoteSlug := "rad1092/gh-dependency-risk"

	data, err := os.ReadFile(filepath.Join(root, "README.md"))
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
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "install-smoke.yml"))
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
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "dep-risk-manual.yml"))
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

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "README.md")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
