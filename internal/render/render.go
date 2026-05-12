package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

type PullRequestMetadata struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Draft       bool   `json:"draft"`
	BaseSHA     string `json:"base_sha"`
	HeadSHA     string `json:"head_sha"`
	AuthorLogin string `json:"author_login"`
}

type Report struct {
	Repo     string                  `json:"repo"`
	PR       PullRequestMetadata     `json:"pr"`
	Analysis analysis.AnalysisResult `json:"analysis"`
}

type JSONReport struct {
	Repo                      string                          `json:"repo"`
	PR                        PullRequestMetadata             `json:"pr"`
	Score                     int                             `json:"score"`
	Level                     analysis.RiskLevel              `json:"level"`
	BlastRadius               analysis.BlastRadius            `json:"blast_radius"`
	DependencyReviewAvailable bool                            `json:"dependency_review_available"`
	Summary                   []string                        `json:"summary"`
	RiskDrivers               []string                        `json:"risk_drivers"`
	RecommendedActions        []string                        `json:"recommended_actions"`
	QuickCommands             []string                        `json:"quick_commands"`
	Notes                     []analysis.Note                 `json:"notes,omitempty"`
	Changes                   []analysis.DependencyChange     `json:"changes"`
	Targets                   []analysis.TargetAnalysisResult `json:"targets,omitempty"`
}

func Render(report Report, format, lang string) (string, error) {
	switch strings.ToLower(format) {
	case "human":
		return renderHuman(report, lang), nil
	case "markdown":
		return renderMarkdown(report, lang), nil
	case "json":
		payload, err := json.MarshalIndent(toJSONReport(report, lang), "", "  ")
		if err != nil {
			return "", err
		}
		return string(payload) + "\n", nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func toJSONReport(report Report, lang string) JSONReport {
	return JSONReport{
		Repo:                      report.Repo,
		PR:                        report.PR,
		Score:                     report.Analysis.Score,
		Level:                     report.Analysis.Level,
		BlastRadius:               report.Analysis.BlastRadius,
		DependencyReviewAvailable: report.Analysis.DependencyReviewAvailable,
		Summary:                   summaryBullets(report, lang),
		RiskDrivers:               append([]string(nil), report.Analysis.RiskDrivers...),
		RecommendedActions:        append([]string(nil), report.Analysis.RecommendedActions...),
		QuickCommands:             append([]string(nil), report.Analysis.QuickCommands...),
		Notes:                     append([]analysis.Note(nil), report.Analysis.Notes...),
		Changes:                   append([]analysis.DependencyChange(nil), report.Analysis.ChangedDependencies...),
		Targets:                   append([]analysis.TargetAnalysisResult(nil), report.Analysis.Targets...),
	}
}

func summaryBullets(report Report, lang string) []string {
	summary := []string{
		localizeSummaryCount(len(report.Analysis.ChangedDependencies), lang),
	}
	if report.Analysis.AddedTransitiveCount > 0 {
		summary = append(summary, localizeSummaryTransitive(report.Analysis.AddedTransitiveCount, lang))
	}
	if len(report.Analysis.Targets) > 1 {
		summary = append(summary, localizeSummaryTargets(len(report.Analysis.Targets), lang))
	}
	if !report.Analysis.DependencyReviewAvailable {
		summary = append(summary, localizeSummaryFallback(lang))
	}
	if names := nonRegistryDependencies(report.Analysis.Notes); len(names) > 0 {
		summary = append(summary, localizeSummarySources(names, lang))
	}
	if len(report.Analysis.RiskDrivers) > 0 {
		summary = append(summary, localizeSummaryDrivers(report.Analysis.RiskDrivers, lang))
	}
	return summary
}

func nonRegistryDependencies(notes []analysis.Note) []string {
	seen := map[string]struct{}{}
	names := make([]string, 0)
	for _, note := range notes {
		if note.Code == analysis.NoteNonRegistrySource && note.Dependency != "" {
			if _, ok := seen[note.Dependency]; ok {
				continue
			}
			seen[note.Dependency] = struct{}{}
			names = append(names, note.Dependency)
		}
	}
	return names
}
