package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func TestWriteBundle(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_RUN_ID", "12345")

	report := Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number:      123,
			URL:         "https://github.com/owner/repo/pull/123",
			Title:       "Update dependencies",
			BaseSHA:     "base",
			HeadSHA:     "head",
			AuthorLogin: "octocat",
		},
		Analysis: analysis.AnalysisResult{
			DependencyReviewAvailable: true,
			Score:                     48,
			Level:                     analysis.RiskLevelHigh,
			BlastRadius:               analysis.BlastRadiusMedium,
			ChangedDependencies: []analysis.DependencyChange{
				{
					Name:        "left-pad",
					ChangeType:  analysis.ChangeUpdated,
					Scope:       analysis.ScopeRuntime,
					Score:       48,
					RiskDrivers: []string{analysis.DriverMajorVersionBump},
					FromVersion: "1.0.0",
					ToVersion:   "2.0.0",
				},
			},
		},
	}

	dir := t.TempDir()
	paths, err := WriteBundle(report, "en", dir)
	if err != nil {
		t.Fatal(err)
	}

	before := readBundleFiles(t, paths)
	if !strings.HasPrefix(before.Markdown, "<!-- gh-dep-risk -->") {
		t.Fatalf("expected markdown bundle to start with marker comment")
	}

	var metadata BundleMetadata
	if err := json.Unmarshal([]byte(before.Metadata), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.WorkflowRunURL != "https://github.com/owner/repo/actions/runs/12345" {
		t.Fatalf("unexpected workflow URL: %q", metadata.WorkflowRunURL)
	}
	if metadata.Score != 48 || metadata.Level != analysis.RiskLevelHigh {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}

	if _, err := WriteBundle(report, "en", dir); err != nil {
		t.Fatal(err)
	}
	after := readBundleFiles(t, paths)
	if before != after {
		t.Fatalf("expected deterministic bundle outputs")
	}
}

func TestWriteBundleIncludesPerTargetFiles(t *testing.T) {
	report := Report{
		Repo: "owner/repo",
		PR: PullRequestMetadata{
			Number: 123,
			URL:    "https://github.com/owner/repo/pull/123",
			Title:  "Workspace update",
		},
		Analysis: analysis.AnalysisResult{
			DependencyReviewAvailable: true,
			Score:                     52,
			Level:                     analysis.RiskLevelHigh,
			BlastRadius:               analysis.BlastRadiusMedium,
			ChangedDependencies: []analysis.DependencyChange{
				{Name: "axios", Target: "apps/web", ChangeType: analysis.ChangeAdded, Scope: analysis.ScopeRuntime, Score: 30},
				{Name: "tailwind-merge", Target: "packages/ui", ChangeType: analysis.ChangeAdded, Scope: analysis.ScopeRuntime, Score: 22},
			},
			Targets: []analysis.TargetAnalysisResult{
				{
					Target:                    analysis.AnalysisTarget{DisplayName: "apps/web", ManifestPath: "apps/web/package.json", LockfilePath: "package-lock.json", Kind: analysis.TargetKindWorkspace},
					DependencyReviewAvailable: true,
					Score:                     30,
					Level:                     analysis.RiskLevelMedium,
					BlastRadius:               analysis.BlastRadiusMedium,
					ChangedDependencies:       []analysis.DependencyChange{{Name: "axios", Target: "apps/web", ChangeType: analysis.ChangeAdded, Scope: analysis.ScopeRuntime, Score: 30}},
				},
				{
					Target:                    analysis.AnalysisTarget{DisplayName: "packages/ui", ManifestPath: "packages/ui/package.json", LockfilePath: "package-lock.json", Kind: analysis.TargetKindWorkspace},
					DependencyReviewAvailable: true,
					Score:                     22,
					Level:                     analysis.RiskLevelMedium,
					BlastRadius:               analysis.BlastRadiusLow,
					ChangedDependencies:       []analysis.DependencyChange{{Name: "tailwind-merge", Target: "packages/ui", ChangeType: analysis.ChangeAdded, Scope: analysis.ScopeRuntime, Score: 22}},
				},
			},
		},
	}

	dir := t.TempDir()
	paths, err := WriteBundle(report, "en", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(paths.Dir, "targets", "apps-web", "dep-risk.md")), "<!-- gh-dep-risk -->") {
		t.Fatalf("expected per-target markdown file")
	}
	if !strings.Contains(readFile(t, filepath.Join(paths.Dir, "targets", "packages-ui", "dep-risk.json")), `"display_name": "packages/ui"`) {
		t.Fatalf("expected per-target JSON file")
	}
	var metadata BundleMetadata
	if err := json.Unmarshal([]byte(readFile(t, paths.Metadata)), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.TargetCount != 2 || len(metadata.Targets) != 2 {
		t.Fatalf("expected target metadata, got %#v", metadata)
	}
}

type bundleContents struct {
	Human    string
	JSON     string
	Markdown string
	Metadata string
}

func readBundleFiles(t *testing.T, paths BundlePaths) bundleContents {
	t.Helper()
	return bundleContents{
		Human:    readFile(t, paths.Human),
		JSON:     readFile(t, paths.JSON),
		Markdown: readFile(t, paths.Markdown),
		Metadata: readFile(t, paths.Metadata),
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
