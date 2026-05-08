package render

import (
	"fmt"
	"sort"
	"strings"

	"gh-dep-risk/internal/analysis"
)

func translator(lang string) func(string) string {
	if lang != "en" {
		return func(key string) string {
			switch key {
			case "repo":
				return "저장소"
			case "pr":
				return "PR"
			case "score":
				return "점수"
			case "blast_radius":
				return "영향 범위"
			case "dependency_review":
				return "Dependency Review 사용 가능"
			case "notes":
				return "참고"
			case "summary":
				return "요약"
			case "what_changed":
				return "변경 사항"
			case "why_risky":
				return "왜 위험한가"
			case "risk_signals":
				return "위험 근거"
			case "recommended_actions":
				return "권장 조치"
			case "quick_commands":
				return "빠른 확인 명령"
			default:
				return key
			}
		}
	}

	return func(key string) string {
		switch key {
		case "repo":
			return "Repository"
		case "pr":
			return "PR"
		case "score":
			return "Score"
		case "blast_radius":
			return "Blast radius"
		case "dependency_review":
			return "Dependency review available"
		case "notes":
			return "Notes"
		case "summary":
			return "Summary"
		case "what_changed":
			return "What changed"
		case "why_risky":
			return "Why risky"
		case "risk_signals":
			return "Risk signals"
		case "recommended_actions":
			return "Recommended actions"
		case "quick_commands":
			return "Quick commands"
		default:
			return key
		}
	}
}

func localizeDrivers(drivers []string, lang string) []string {
	items := make([]string, 0, len(drivers))
	for _, driver := range drivers {
		if lang == "en" {
			switch driver {
			case analysis.DriverKnownVulnerabilities:
				items = append(items, "Known vulnerabilities were reported for the target version.")
			case analysis.DriverAddedDirectRuntime:
				items = append(items, "A new direct runtime dependency was added.")
			case analysis.DriverAddedDirectDev:
				items = append(items, "A new direct dev dependency was added.")
			case analysis.DriverMajorVersionBump:
				items = append(items, "The dependency crosses a major version boundary.")
			case analysis.DriverRecentlyPublished:
				items = append(items, "The target version was published within the last 7 days.")
			case analysis.DriverInstallScript:
				items = append(items, "The package declares an install script.")
			case analysis.DriverPlatformRestricted:
				items = append(items, "The package is restricted to specific OS/CPU targets.")
			case analysis.DriverTransitiveFive:
				items = append(items, "The PR adds at least 5 new transitive packages.")
			case analysis.DriverTransitiveFifteen:
				items = append(items, "The PR adds at least 15 new transitive packages.")
			default:
				items = append(items, driver)
			}
			continue
		}

		switch driver {
		case analysis.DriverKnownVulnerabilities:
			items = append(items, "대상 버전에 알려진 취약점이 있습니다.")
		case analysis.DriverAddedDirectRuntime:
			items = append(items, "직접 런타임 의존성이 새로 추가되었습니다.")
		case analysis.DriverAddedDirectDev:
			items = append(items, "직접 개발 의존성이 새로 추가되었습니다.")
		case analysis.DriverMajorVersionBump:
			items = append(items, "메이저 버전 경계를 넘는 변경입니다.")
		case analysis.DriverRecentlyPublished:
			items = append(items, "대상 버전이 최근 7일 이내에 배포되었습니다.")
		case analysis.DriverInstallScript:
			items = append(items, "패키지가 install script를 선언합니다.")
		case analysis.DriverPlatformRestricted:
			items = append(items, "특정 OS/CPU 대상으로 제한된 패키지입니다.")
		case analysis.DriverTransitiveFive:
			items = append(items, "새 전이 의존성이 5개 이상 추가되었습니다.")
		case analysis.DriverTransitiveFifteen:
			items = append(items, "새 전이 의존성이 15개 이상 추가되었습니다.")
		default:
			items = append(items, driver)
		}
	}
	return items
}

func localizeAction(action, lang string) string {
	if lang == "en" {
		switch action {
		case analysis.ActionInspectInstall:
			return "Inspect install scripts and package tarballs before merging."
		case analysis.ActionInspectTree:
			return "Inspect the dependency tree and lockfile diff."
		case analysis.ActionReviewAdvisories:
			return "Review GHSA advisories before merging."
		case analysis.ActionReviewChangelog:
			return "Read upstream release notes and migration guidance."
		case analysis.ActionRunTargetedTests:
			return "Run targeted tests and smoke checks for affected paths."
		case analysis.ActionValidateSources:
			return "Validate non-registry or git sources explicitly."
		default:
			return action
		}
	}

	switch action {
	case analysis.ActionInspectInstall:
		return "병합 전에 설치 스크립트와 패키지 tarball을 확인하세요."
	case analysis.ActionInspectTree:
		return "의존성 트리와 lockfile diff를 확인하세요."
	case analysis.ActionReviewAdvisories:
		return "병합 전에 GHSA advisory를 검토하세요."
	case analysis.ActionReviewChangelog:
		return "업스트림 릴리스 노트와 마이그레이션 가이드를 읽어 보세요."
	case analysis.ActionRunTargetedTests:
		return "영향 경로에 대한 타깃 테스트와 스모크 체크를 실행하세요."
	case analysis.ActionValidateSources:
		return "비레지스트리 또는 git 소스를 별도로 검증하세요."
	default:
		return action
	}
}

func localizeNote(note analysis.Note, lang string) string {
	if lang != "en" {
		if text, ok := localizeYarnNoteKorean(note); ok {
			return text
		}
		if text, ok := localizeBunNoteKorean(note); ok {
			return text
		}
	}
	if lang == "en" {
		switch note.Code {
		case analysis.NoteDependencyReviewFallback:
			return "Dependency review API was unavailable, so local fallback analysis was used."
		case analysis.NoteNonRegistrySource:
			return fmt.Sprintf("%s resolves from a non-default package source: %s", note.Dependency, note.Detail)
		case analysis.NoteUnsupportedDependency:
			return fmt.Sprintf("Some dependency entries were not analyzed by local fallback: %s", note.Detail)
		case analysis.NoteGoReplaceDirective:
			return fmt.Sprintf("Go replace directive changed for %s: %s", note.Dependency, note.Detail)
		case analysis.NoteGoLocalReplace:
			return fmt.Sprintf("Go module %s uses a local replace target: %s", note.Dependency, note.Detail)
		case analysis.NoteGoPseudoVersion:
			return fmt.Sprintf("Go module %s uses a pseudo-version: %s", note.Dependency, note.Detail)
		case analysis.NoteGoChecksumChanged:
			return fmt.Sprintf("Go checksum evidence changed: %s", note.Detail)
		case analysis.NoteGoDirectiveChanged:
			return fmt.Sprintf("Go language directive changed: %s", note.Detail)
		case analysis.NoteGoToolchainChanged:
			return fmt.Sprintf("Go toolchain directive changed: %s", note.Detail)
		case analysis.NoteYarnBerryLockfile:
			return fmt.Sprintf("Yarn Berry lockfile fallback was used: %s", note.Detail)
		case analysis.NoteYarnNodeLinker:
			return fmt.Sprintf("Yarn nodeLinker setting was detected: %s", note.Detail)
		case analysis.NoteYarnWorkspaceProtocol:
			return fmt.Sprintf("Yarn dependency %s uses the workspace protocol: %s", note.Dependency, note.Detail)
		case analysis.NoteYarnPatchProtocol:
			return fmt.Sprintf("Yarn dependency %s uses the patch protocol: %s", note.Dependency, note.Detail)
		case analysis.NoteYarnPortalProtocol:
			return fmt.Sprintf("Yarn dependency %s uses the portal protocol: %s", note.Dependency, note.Detail)
		case analysis.NoteYarnLinkProtocol:
			return fmt.Sprintf("Yarn dependency %s uses the link protocol: %s", note.Dependency, note.Detail)
		case analysis.NoteYarnFileProtocol:
			return fmt.Sprintf("Yarn dependency %s uses the file protocol: %s", note.Dependency, note.Detail)
		case analysis.NoteYarnGitSource:
			return fmt.Sprintf("Yarn dependency %s uses a git source: %s", note.Dependency, note.Detail)
		case analysis.NoteYarnChecksumChanged:
			return fmt.Sprintf("Yarn checksum evidence changed for %s: %s", note.Dependency, note.Detail)
		case analysis.NoteBunLockfile:
			return fmt.Sprintf("Bun lockfile fallback was used: %s", note.Detail)
		case analysis.NoteBunWorkspaceProtocol:
			return fmt.Sprintf("Bun dependency %s uses the workspace protocol: %s", note.Dependency, note.Detail)
		case analysis.NoteBunChecksumChanged:
			return fmt.Sprintf("Bun checksum evidence changed for %s: %s", note.Dependency, note.Detail)
		case analysis.NoteBunBinaryLockfile:
			return fmt.Sprintf("Bun binary lockfile is unsupported by local fallback: %s", note.Detail)
		default:
			return note.Code
		}
	}

	switch note.Code {
	case analysis.NoteDependencyReviewFallback:
		return "Dependency Review API를 사용할 수 없어 local fallback 분석을 사용했습니다."
	case analysis.NoteNonRegistrySource:
		return fmt.Sprintf("%s 패키지가 기본 레지스트리 외 소스로 해석됩니다: %s", note.Dependency, note.Detail)
	case analysis.NoteUnsupportedDependency:
		return fmt.Sprintf("일부 의존성 항목은 local fallback에서 분석되지 않았습니다: %s", note.Detail)
	case analysis.NoteGoReplaceDirective:
		return fmt.Sprintf("Go replace 지시자가 변경되었습니다(%s): %s", note.Dependency, note.Detail)
	case analysis.NoteGoLocalReplace:
		return fmt.Sprintf("Go 모듈 %s가 local replace 대상을 사용합니다: %s", note.Dependency, note.Detail)
	case analysis.NoteGoPseudoVersion:
		return fmt.Sprintf("Go 모듈 %s가 pseudo-version을 사용합니다: %s", note.Dependency, note.Detail)
	case analysis.NoteGoChecksumChanged:
		return fmt.Sprintf("Go checksum 근거가 변경되었습니다: %s", note.Detail)
	case analysis.NoteGoDirectiveChanged:
		return fmt.Sprintf("Go language 지시자가 변경되었습니다: %s", note.Detail)
	case analysis.NoteGoToolchainChanged:
		return fmt.Sprintf("Go toolchain 지시자가 변경되었습니다: %s", note.Detail)
	default:
		return note.Code
	}
}

func localizeYarnNoteKorean(note analysis.Note) (string, bool) {
	switch note.Code {
	case analysis.NoteYarnBerryLockfile:
		return fmt.Sprintf("Yarn Berry lockfile fallback을 사용했습니다: %s", note.Detail), true
	case analysis.NoteYarnNodeLinker:
		return fmt.Sprintf("Yarn nodeLinker 설정을 감지했습니다: %s", note.Detail), true
	case analysis.NoteYarnWorkspaceProtocol:
		return fmt.Sprintf("Yarn 의존성 %s가 workspace 프로토콜을 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteYarnPatchProtocol:
		return fmt.Sprintf("Yarn 의존성 %s가 patch 프로토콜을 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteYarnPortalProtocol:
		return fmt.Sprintf("Yarn 의존성 %s가 portal 프로토콜을 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteYarnLinkProtocol:
		return fmt.Sprintf("Yarn 의존성 %s가 link 프로토콜을 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteYarnFileProtocol:
		return fmt.Sprintf("Yarn 의존성 %s가 file 프로토콜을 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteYarnGitSource:
		return fmt.Sprintf("Yarn 의존성 %s가 git 소스를 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteYarnChecksumChanged:
		return fmt.Sprintf("Yarn checksum 근거가 변경되었습니다(%s): %s", note.Dependency, note.Detail), true
	default:
		return "", false
	}
}

func localizeBunNoteKorean(note analysis.Note) (string, bool) {
	switch note.Code {
	case analysis.NoteBunLockfile:
		return fmt.Sprintf("Bun lockfile fallback을 사용했습니다: %s", note.Detail), true
	case analysis.NoteBunWorkspaceProtocol:
		return fmt.Sprintf("Bun 의존성 %s가 workspace 프로토콜을 사용합니다: %s", note.Dependency, note.Detail), true
	case analysis.NoteBunChecksumChanged:
		return fmt.Sprintf("Bun checksum 근거가 변경되었습니다(%s): %s", note.Dependency, note.Detail), true
	case analysis.NoteBunBinaryLockfile:
		return fmt.Sprintf("Bun binary lockfile은 local fallback에서 지원되지 않습니다: %s", note.Detail), true
	default:
		return "", false
	}
}

func localizeSummaryCount(changeCount int, lang string) string {
	if lang == "en" {
		return fmt.Sprintf("%d dependency changes were detected.", changeCount)
	}
	return fmt.Sprintf("의존성 변경 %d건이 감지되었습니다.", changeCount)
}

func localizeSummaryTransitive(count int, lang string) string {
	if lang == "en" {
		return fmt.Sprintf("%d newly added transitive dependencies were detected.", count)
	}
	return fmt.Sprintf("새로 추가된 전이 의존성 %d건이 감지되었습니다.", count)
}

func localizeSummaryFallback(lang string) string {
	if lang == "en" {
		return "Dependency Review was unavailable, so local fallback analysis was used."
	}
	return "Dependency Review를 사용할 수 없어 local fallback 분석을 사용했습니다."
}

func localizeSummarySources(names []string, lang string) string {
	sortedNames := append([]string(nil), names...)
	sort.Strings(sortedNames)
	display := strings.Join(limitNames(sortedNames), ", ")
	if lang == "en" {
		if len(sortedNames) > 3 {
			return fmt.Sprintf("Non-default dependency sources were detected in %d packages, including %s.", len(sortedNames), display)
		}
		return fmt.Sprintf("Non-default dependency sources were detected for %s.", display)
	}
	if len(sortedNames) > 3 {
		return fmt.Sprintf("기본 레지스트리 외 소스가 %d개 패키지에서 감지되었고, 예시는 %s 입니다.", len(sortedNames), display)
	}
	return fmt.Sprintf("기본 레지스트리 외 소스가 %s 에서 감지되었습니다.", display)
}

func localizeSummaryDrivers(drivers []string, lang string) string {
	labels := make([]string, 0, len(drivers))
	for _, driver := range drivers {
		labels = append(labels, localizeDriverLabel(driver, lang))
	}
	labels = limitNames(labels)
	if lang == "en" {
		return fmt.Sprintf("Top risk signals: %s.", strings.Join(labels, ", "))
	}
	return fmt.Sprintf("주요 위험 신호: %s.", strings.Join(labels, ", "))
}

func localizeDriverLabel(driver, lang string) string {
	if lang == "en" {
		switch driver {
		case analysis.DriverKnownVulnerabilities:
			return "known vulnerabilities"
		case analysis.DriverAddedDirectRuntime:
			return "direct runtime addition"
		case analysis.DriverAddedDirectDev:
			return "direct dev addition"
		case analysis.DriverMajorVersionBump:
			return "major version bump"
		case analysis.DriverRecentlyPublished:
			return "recently published version"
		case analysis.DriverInstallScript:
			return "install script"
		case analysis.DriverPlatformRestricted:
			return "platform restriction"
		case analysis.DriverTransitiveFive:
			return "5+ transitive additions"
		case analysis.DriverTransitiveFifteen:
			return "15+ transitive additions"
		default:
			return driver
		}
	}

	switch driver {
	case analysis.DriverKnownVulnerabilities:
		return "알려진 취약점"
	case analysis.DriverAddedDirectRuntime:
		return "직접 런타임 추가"
	case analysis.DriverAddedDirectDev:
		return "직접 개발 의존성 추가"
	case analysis.DriverMajorVersionBump:
		return "메이저 버전 변경"
	case analysis.DriverRecentlyPublished:
		return "최근 배포 버전"
	case analysis.DriverInstallScript:
		return "설치 스크립트"
	case analysis.DriverPlatformRestricted:
		return "플랫폼 제한"
	case analysis.DriverTransitiveFive:
		return "전이 의존성 5+"
	case analysis.DriverTransitiveFifteen:
		return "전이 의존성 15+"
	default:
		return driver
	}
}

func limitNames(values []string) []string {
	if len(values) <= 3 {
		return values
	}
	return values[:3]
}
