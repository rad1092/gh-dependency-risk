package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func whyRiskyLine(report Report, lang string) string {
	change, ok := strongestChange(report.Analysis)
	if ok {
		if line := explainChangeRisk(change, len(report.Analysis.Targets) > 1, lang); line != "" {
			return line
		}
	}
	if len(report.Analysis.RiskDrivers) == 0 {
		return ""
	}
	labels := make([]string, 0, len(report.Analysis.RiskDrivers))
	for _, driver := range limitDrivers(report.Analysis.RiskDrivers) {
		labels = append(labels, localizeDriverLabel(driver, lang))
	}
	if lang == "en" {
		return fmt.Sprintf("Primary risk signals are %s.", strings.Join(labels, ", "))
	}
	return fmt.Sprintf("주요 위험 신호는 %s 입니다.", strings.Join(labels, ", "))
}

func recommendedActionLines(report Report, lang string) []string {
	lines := make([]string, 0, len(report.Analysis.RecommendedActions))
	for _, action := range report.Analysis.RecommendedActions {
		lines = append(lines, explainRecommendedAction(action, report, lang))
	}
	return lines
}

func strongestChange(result analysis.AnalysisResult) (analysis.DependencyChange, bool) {
	if len(result.ChangedDependencies) == 0 {
		return analysis.DependencyChange{}, false
	}
	changes := append([]analysis.DependencyChange(nil), result.ChangedDependencies...)
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Score != changes[j].Score {
			return changes[i].Score > changes[j].Score
		}
		if changes[i].Target != changes[j].Target {
			return changes[i].Target < changes[j].Target
		}
		if changes[i].Name != changes[j].Name {
			return changes[i].Name < changes[j].Name
		}
		if changes[i].ChangeType != changes[j].ChangeType {
			return changes[i].ChangeType < changes[j].ChangeType
		}
		return changes[i].Scope < changes[j].Scope
	})
	return changes[0], true
}

func explainChangeRisk(change analysis.DependencyChange, multiTarget bool, lang string) string {
	phrases := make([]string, 0, len(change.RiskDrivers))
	for _, driver := range limitDrivers(change.RiskDrivers) {
		if phrase := riskPhraseForDriver(driver, lang); phrase != "" {
			phrases = append(phrases, phrase)
		}
	}
	if len(phrases) == 0 {
		return ""
	}
	subject := riskSubject(change, multiTarget)
	if len(phrases) == 1 {
		return fmt.Sprintf("%s %s.", subject, phrases[0])
	}
	return fmt.Sprintf("%s %s and %s.", subject, phrases[0], phrases[1])
}

func riskSubject(change analysis.DependencyChange, multiTarget bool) string {
	if multiTarget && change.Target != "" {
		return fmt.Sprintf("%s / %s", change.Target, change.Name)
	}
	return change.Name
}

func riskPhraseForDriver(driver, lang string) string {
	if lang == "en" {
		switch driver {
		case analysis.DriverKnownVulnerabilities:
			return "has known vulnerabilities in the target version"
		case analysis.DriverAddedDirectRuntime:
			return "adds a new direct runtime dependency"
		case analysis.DriverAddedDirectDev:
			return "adds a new direct dev dependency"
		case analysis.DriverMajorVersionBump:
			return "crosses a major version boundary"
		case analysis.DriverRecentlyPublished:
			return "was published within the last 7 days"
		case analysis.DriverInstallScript:
			return "declares an install script"
		case analysis.DriverPlatformRestricted:
			return "is restricted to specific OS/CPU targets"
		case analysis.DriverTransitiveFifteen:
			return "pulls in 15 or more new transitive packages"
		case analysis.DriverTransitiveFive:
			return "pulls in at least 5 new transitive packages"
		default:
			return ""
		}
	}

	switch driver {
	case analysis.DriverKnownVulnerabilities:
		return "대상 버전에 알려진 취약점이 있습니다"
	case analysis.DriverAddedDirectRuntime:
		return "새 직접 런타임 의존성을 추가합니다"
	case analysis.DriverAddedDirectDev:
		return "새 직접 개발 의존성을 추가합니다"
	case analysis.DriverMajorVersionBump:
		return "메이저 버전 경계를 넘습니다"
	case analysis.DriverRecentlyPublished:
		return "최근 7일 이내에 배포됐습니다"
	case analysis.DriverInstallScript:
		return "install script를 선언합니다"
	case analysis.DriverPlatformRestricted:
		return "특정 OS/CPU 대상으로 제한됩니다"
	case analysis.DriverTransitiveFifteen:
		return "새 전이 의존성을 15개 이상 끌어옵니다"
	case analysis.DriverTransitiveFive:
		return "새 전이 의존성을 5개 이상 끌어옵니다"
	default:
		return ""
	}
}

func explainRecommendedAction(action string, report Report, lang string) string {
	if lang != "en" {
		return explainRecommendedActionNonEnglish(action, report, lang)
	}

	switch action {
	case analysis.ActionInspectInstall:
		if names := packageNamesForDriver(report.Analysis.ChangedDependencies, analysis.DriverInstallScript); len(names) > 0 {
			return fmt.Sprintf("Inspect install scripts and published tarballs for %s before merging.", joinList(limitNames(names)))
		}
	case analysis.ActionInspectTree:
		if report.Analysis.AddedTransitiveCount > 0 {
			return fmt.Sprintf("Inspect the lockfile diff and dependency tree to confirm the %d new transitive additions are expected.", report.Analysis.AddedTransitiveCount)
		}
	case analysis.ActionReviewAdvisories:
		if names := packageNamesForDriver(report.Analysis.ChangedDependencies, analysis.DriverKnownVulnerabilities); len(names) > 0 {
			return fmt.Sprintf("Review GHSA advisories for %s and confirm a fixed or acceptable version before merging.", joinList(limitNames(names)))
		}
	case analysis.ActionReviewChangelog:
		if names := packageNamesForDriver(report.Analysis.ChangedDependencies, analysis.DriverMajorVersionBump); len(names) > 0 {
			return fmt.Sprintf("Check release notes and migration guidance for %s before merging.", joinList(limitNames(names)))
		}
	case analysis.ActionRunTargetedTests:
		if targets := targetNames(report.Analysis); len(targets) > 0 {
			return fmt.Sprintf("Run targeted smoke checks for %s and the code paths that import the changed packages.", joinList(limitNames(targets)))
		}
	case analysis.ActionValidateSources:
		if names := nonRegistryDependencies(report.Analysis.Notes); len(names) > 0 {
			return fmt.Sprintf("Validate the non-registry source for %s and confirm the fetched artifact is expected.", joinList(limitNames(names)))
		}
	}
	return localizeAction(action, lang)
}

func explainRecommendedActionNonEnglish(action string, report Report, lang string) string {
	base := localizeAction(action, lang)
	switch action {
	case analysis.ActionInspectInstall:
		if names := packageNamesForDriver(report.Analysis.ChangedDependencies, analysis.DriverInstallScript); len(names) > 0 {
			return fmt.Sprintf("%s 패키지: %s.", base, strings.Join(limitNames(names), ", "))
		}
	case analysis.ActionInspectTree:
		if report.Analysis.AddedTransitiveCount > 0 {
			return fmt.Sprintf("%s 새 전이 의존성 %d개를 확인하세요.", base, report.Analysis.AddedTransitiveCount)
		}
	case analysis.ActionReviewAdvisories:
		if names := packageNamesForDriver(report.Analysis.ChangedDependencies, analysis.DriverKnownVulnerabilities); len(names) > 0 {
			return fmt.Sprintf("%s 패키지: %s.", base, strings.Join(limitNames(names), ", "))
		}
	case analysis.ActionReviewChangelog:
		if names := packageNamesForDriver(report.Analysis.ChangedDependencies, analysis.DriverMajorVersionBump); len(names) > 0 {
			return fmt.Sprintf("%s 패키지: %s.", base, strings.Join(limitNames(names), ", "))
		}
	case analysis.ActionRunTargetedTests:
		if targets := targetNames(report.Analysis); len(targets) > 0 {
			return fmt.Sprintf("%s 대상: %s.", base, strings.Join(limitNames(targets), ", "))
		}
	case analysis.ActionValidateSources:
		if names := nonRegistryDependencies(report.Analysis.Notes); len(names) > 0 {
			return fmt.Sprintf("%s 대상: %s.", base, strings.Join(limitNames(names), ", "))
		}
	}
	return base
}

func packageNamesForDriver(changes []analysis.DependencyChange, driver string) []string {
	seen := map[string]struct{}{}
	names := make([]string, 0)
	for _, change := range changes {
		if !contains(change.RiskDrivers, driver) {
			continue
		}
		if _, ok := seen[change.Name]; ok {
			continue
		}
		seen[change.Name] = struct{}{}
		names = append(names, change.Name)
	}
	sort.Strings(names)
	return names
}

func targetNames(result analysis.AnalysisResult) []string {
	if len(result.Targets) == 0 {
		return nil
	}
	names := make([]string, 0, len(result.Targets))
	for _, target := range result.Targets {
		names = append(names, displayTarget(target.Target))
	}
	sort.Strings(names)
	return names
}

func limitDrivers(drivers []string) []string {
	if len(drivers) <= 2 {
		return drivers
	}
	return drivers[:2]
}

func joinList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
