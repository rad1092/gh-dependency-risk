package render

import (
	"fmt"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
)

func targetSectionTitle(lang string) string {
	if lang == "en" {
		return "Targets"
	}
	return "타깃 결과"
}

func localizeSummaryTargets(count int, lang string) string {
	if lang == "en" {
		return fmt.Sprintf("%d dependency targets were analyzed.", count)
	}
	return fmt.Sprintf("의존성 타깃 %d개를 분석했습니다.", count)
}

func displayTarget(target analysis.AnalysisTarget) string {
	if target.DisplayName != "" {
		return target.DisplayName
	}
	if dir := target.Directory(); dir != "" {
		return dir
	}
	return "root"
}

func targetContext(target analysis.AnalysisTarget) string {
	parts := []string{string(target.Kind)}
	if target.Ecosystem != "" {
		parts = append(parts, "ecosystem="+target.Ecosystem)
	}
	if target.PackageManager != "" && target.PackageManager != target.Ecosystem {
		parts = append(parts, "manager="+target.PackageManager)
	} else if target.PackageManager != "" && target.Ecosystem == "" {
		parts = append(parts, "manager="+target.PackageManager)
	}
	return strings.Join(parts, ", ")
}

func localizeNoteMessage(note analysis.Note, lang string) string {
	if note.Code == analysis.NoteApproximateAttribution {
		if lang == "en" {
			return fmt.Sprintf("Target attribution is approximate for %s because shared lockfile changes could not be mapped exactly.", note.Detail)
		}
		return fmt.Sprintf("%s 타깃은 공유 lockfile 변경을 정확히 매핑할 수 없어 근사 분석으로 표시했습니다.", note.Detail)
	}
	return localizeNote(note, lang)
}
