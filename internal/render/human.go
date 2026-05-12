package render

import (
	"fmt"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func renderHuman(report Report, lang string) string {
	var b strings.Builder
	tr := translator(lang)

	fmt.Fprintf(&b, "%s: %s\n", tr("repo"), report.Repo)
	fmt.Fprintf(&b, "%s: #%d %s\n", tr("pr"), report.PR.Number, report.PR.Title)
	fmt.Fprintf(&b, "%s: %d (%s)\n", tr("score"), report.Analysis.Score, report.Analysis.Level)
	fmt.Fprintf(&b, "%s: %s\n", tr("blast_radius"), report.Analysis.BlastRadius)
	fmt.Fprintf(&b, "%s: %t\n", tr("dependency_review"), report.Analysis.DependencyReviewAvailable)
	if why := whyRiskyLine(report, lang); why != "" {
		fmt.Fprintf(&b, "%s: %s\n", tr("why_risky"), why)
	}

	b.WriteString("\n")
	b.WriteString(tr("summary"))
	b.WriteString(":\n")
	for _, line := range summaryBullets(report, lang) {
		fmt.Fprintf(&b, "- %s\n", line)
	}

	if len(report.Analysis.Notes) > 0 {
		b.WriteString("\n")
		b.WriteString(tr("notes"))
		b.WriteString(":\n")
		for _, note := range report.Analysis.Notes {
			fmt.Fprintf(&b, "- %s\n", localizeNoteMessage(note, lang))
		}
	}

	if len(report.Analysis.Targets) > 0 {
		b.WriteString("\n")
		b.WriteString(targetSectionTitle(lang))
		b.WriteString(":\n")
		for _, target := range report.Analysis.Targets {
			fmt.Fprintf(&b, "- %s [%s] score=%d (%s), blast=%s\n", displayTarget(target.Target), targetContext(target.Target), target.Score, target.Level, target.BlastRadius)
			for _, change := range target.ChangedDependencies {
				fmt.Fprintf(&b, "  %s [%s/%s] score=%d\n", displayChange(change), change.ChangeType, change.Scope, change.Score)
				if len(change.RiskDrivers) > 0 {
					fmt.Fprintf(&b, "    %s: %s\n", tr("risk_signals"), strings.Join(localizeDrivers(change.RiskDrivers, lang), ", "))
				}
			}
			for _, note := range target.Notes {
				fmt.Fprintf(&b, "  %s: %s\n", tr("notes"), localizeNoteMessage(note, lang))
			}
		}
	} else if len(report.Analysis.ChangedDependencies) > 0 {
		b.WriteString("\n")
		b.WriteString(tr("what_changed"))
		b.WriteString(":\n")
		for _, change := range report.Analysis.ChangedDependencies {
			fmt.Fprintf(&b, "- %s [%s/%s] score=%d\n", displayChange(change), change.ChangeType, change.Scope, change.Score)
			if len(change.RiskDrivers) > 0 {
				fmt.Fprintf(&b, "  %s: %s\n", tr("risk_signals"), strings.Join(localizeDrivers(change.RiskDrivers, lang), ", "))
			}
		}
	}

	if len(report.Analysis.RecommendedActions) > 0 {
		b.WriteString("\n")
		b.WriteString(tr("recommended_actions"))
		b.WriteString(":\n")
		for _, action := range recommendedActionLines(report, lang) {
			fmt.Fprintf(&b, "- %s\n", action)
		}
	}

	if len(report.Analysis.QuickCommands) > 0 {
		b.WriteString("\n")
		b.WriteString(tr("quick_commands"))
		b.WriteString(":\n")
		for _, command := range report.Analysis.QuickCommands {
			fmt.Fprintf(&b, "- %s\n", command)
		}
	}

	return b.String()
}

func displayChange(change analysis.DependencyChange) string {
	left := change.FromVersion
	if left == "" {
		left = change.FromRequirement
	}
	right := change.ToVersion
	if right == "" {
		right = change.ToRequirement
	}
	switch change.ChangeType {
	case analysis.ChangeAdded:
		if right == "" {
			return change.Name
		}
		return fmt.Sprintf("%s -> %s", change.Name, right)
	case analysis.ChangeRemoved:
		if left == "" {
			return change.Name
		}
		return fmt.Sprintf("%s <- %s", change.Name, left)
	default:
		if left == "" && right == "" {
			return change.Name
		}
		return fmt.Sprintf("%s %s -> %s", change.Name, left, right)
	}
}
