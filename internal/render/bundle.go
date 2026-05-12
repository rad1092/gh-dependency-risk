package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

const (
	BundleHumanFile    = "dep-risk-human.txt"
	BundleJSONFile     = "dep-risk.json"
	BundleMarkdownFile = "dep-risk.md"
	BundleMetadataFile = "metadata.json"
)

type BundlePaths struct {
	Dir      string
	Human    string
	JSON     string
	Markdown string
	Metadata string
}

type BundleMetadata struct {
	Repo                      string                    `json:"repo"`
	PR                        PullRequestMetadata       `json:"pr"`
	WorkflowRunURL            string                    `json:"workflow_run_url,omitempty"`
	Score                     int                       `json:"score"`
	Level                     analysis.RiskLevel        `json:"level"`
	BlastRadius               analysis.BlastRadius      `json:"blast_radius"`
	DependencyReviewAvailable bool                      `json:"dependency_review_available"`
	Targets                   []analysis.AnalysisTarget `json:"targets,omitempty"`
	TargetCount               int                       `json:"target_count"`
}

func WriteBundle(report Report, lang, dir string) (BundlePaths, error) {
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return BundlePaths{}, fmt.Errorf("bundle directory is required")
	}

	if err := os.MkdirAll(trimmedDir, 0o755); err != nil {
		return BundlePaths{}, fmt.Errorf("create bundle directory: %w", err)
	}

	paths := BundlePaths{
		Dir:      trimmedDir,
		Human:    filepath.Join(trimmedDir, BundleHumanFile),
		JSON:     filepath.Join(trimmedDir, BundleJSONFile),
		Markdown: filepath.Join(trimmedDir, BundleMarkdownFile),
		Metadata: filepath.Join(trimmedDir, BundleMetadataFile),
	}

	human, err := Render(report, "human", lang)
	if err != nil {
		return BundlePaths{}, err
	}
	jsonOutput, err := Render(report, "json", lang)
	if err != nil {
		return BundlePaths{}, err
	}
	markdown, err := Render(report, "markdown", lang)
	if err != nil {
		return BundlePaths{}, err
	}

	metadata, err := json.MarshalIndent(toBundleMetadata(report), "", "  ")
	if err != nil {
		return BundlePaths{}, fmt.Errorf("marshal bundle metadata: %w", err)
	}

	for path, content := range map[string]string{
		paths.Human:    human,
		paths.JSON:     jsonOutput,
		paths.Markdown: markdown,
		paths.Metadata: string(metadata) + "\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return BundlePaths{}, fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	for _, target := range report.Analysis.Targets {
		targetDir := filepath.Join(trimmedDir, "targets", safeTargetDir(target.Target))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return BundlePaths{}, fmt.Errorf("create target bundle directory: %w", err)
		}
		targetReport := Report{
			Repo: report.Repo,
			PR:   report.PR,
			Analysis: analysis.AnalysisResult{
				DependencyReviewAvailable: target.DependencyReviewAvailable,
				Score:                     target.Score,
				Level:                     target.Level,
				BlastRadius:               target.BlastRadius,
				ChangedDependencies:       append([]analysis.DependencyChange(nil), target.ChangedDependencies...),
				RiskDrivers:               append([]string(nil), target.RiskDrivers...),
				RecommendedActions:        append([]string(nil), target.RecommendedActions...),
				QuickCommands:             append([]string(nil), target.QuickCommands...),
				Notes:                     append([]analysis.Note(nil), target.Notes...),
				AddedTransitiveCount:      target.AddedTransitiveCount,
				Targets:                   []analysis.TargetAnalysisResult{target},
			},
		}
		targetJSON, err := Render(targetReport, "json", lang)
		if err != nil {
			return BundlePaths{}, err
		}
		targetMarkdown, err := Render(targetReport, "markdown", lang)
		if err != nil {
			return BundlePaths{}, err
		}
		for filePath, content := range map[string]string{
			filepath.Join(targetDir, BundleJSONFile):     targetJSON,
			filepath.Join(targetDir, BundleMarkdownFile): targetMarkdown,
		} {
			if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
				return BundlePaths{}, fmt.Errorf("write %s: %w", filepath.Base(filePath), err)
			}
		}
	}

	return paths, nil
}

func toBundleMetadata(report Report) BundleMetadata {
	return BundleMetadata{
		Repo:                      report.Repo,
		PR:                        report.PR,
		WorkflowRunURL:            workflowRunURL(),
		Score:                     report.Analysis.Score,
		Level:                     report.Analysis.Level,
		BlastRadius:               report.Analysis.BlastRadius,
		DependencyReviewAvailable: report.Analysis.DependencyReviewAvailable,
		Targets:                   collectBundleTargets(report.Analysis.Targets),
		TargetCount:               len(report.Analysis.Targets),
	}
}

func workflowRunURL() string {
	serverURL := strings.TrimRight(os.Getenv("GITHUB_SERVER_URL"), "/")
	repository := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY"))
	runID := strings.TrimSpace(os.Getenv("GITHUB_RUN_ID"))
	if serverURL == "" || repository == "" || runID == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/actions/runs/%s", serverURL, repository, runID)
}

func collectBundleTargets(targets []analysis.TargetAnalysisResult) []analysis.AnalysisTarget {
	result := make([]analysis.AnalysisTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, target.Target)
	}
	return result
}

func safeTargetDir(target analysis.AnalysisTarget) string {
	name := displayTarget(target)
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	safe := replacer.Replace(name)
	safe = strings.Trim(safe, "-.")
	if safe == "" {
		safe = "root"
	}
	return safe
}
