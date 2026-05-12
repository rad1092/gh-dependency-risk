package app

import (
	"context"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
	ghclient "github.com/rad1092/gh-dependency-risk/internal/github"
	"github.com/rad1092/gh-dependency-risk/internal/review"
)

type dependencyReviewSnapshot struct {
	Available      bool
	TargetChanges  map[string][]analysis.ReviewChange
	DerivedTargets []discoveredTarget
}

func loadDependencyReviewSnapshot(ctx context.Context, client ghclient.Client, repo ghclient.Repo, baseSHA, headSHA string) (dependencyReviewSnapshot, error) {
	rawChanges, err := client.CompareDependencies(ctx, repo, baseSHA, headSHA)
	if err != nil {
		if ghclient.IsDependencyReviewUnavailable(err) {
			return dependencyReviewSnapshot{
				Available:      false,
				TargetChanges:  map[string][]analysis.ReviewChange{},
				DerivedTargets: nil,
			}, nil
		}
		return dependencyReviewSnapshot{}, err
	}

	normalized := review.NormalizeChanges(rawChanges)
	targets := review.TargetsFromChanges(normalized)
	changesByTarget := review.ChangesByTarget(normalized)
	snapshot := dependencyReviewSnapshot{
		Available:      true,
		TargetChanges:  map[string][]analysis.ReviewChange{},
		DerivedTargets: make([]discoveredTarget, 0, len(targets)),
	}
	for _, derived := range targets {
		target := analysisTargetFromReview(derived)
		group := changesByTarget[derived.Identity]
		changes := make([]analysis.ReviewChange, 0, len(group))
		for _, change := range group {
			vulns := make([]analysis.Vulnerability, 0, len(change.Vulnerabilities))
			for _, vuln := range change.Vulnerabilities {
				vulns = append(vulns, analysis.Vulnerability{
					Severity: vuln.Severity,
					GHSAID:   vuln.GHSAID,
					Summary:  vuln.Summary,
					URL:      vuln.URL,
				})
			}
			changes = append(changes, analysis.ReviewChange{
				ChangeType:      analysis.ChangeType(change.ChangeType),
				Manifest:        change.ManifestPath,
				Name:            change.Name,
				Version:         change.Version,
				Ecosystem:       string(change.Ecosystem),
				PackageManager:  string(change.PackageManager),
				Vulnerabilities: vulns,
			})
		}
		snapshot.TargetChanges[target.Key()] = changes
		snapshot.DerivedTargets = append(snapshot.DerivedTargets, discoveredTarget{
			ManifestPath: target.ManifestPath,
			DisplayName:  target.DisplayName,
			Variants:     []analysis.AnalysisTarget{target},
		})
	}
	return snapshot, nil
}

func analysisTargetFromReview(target review.Target) analysis.AnalysisTarget {
	kind := analysis.TargetKindStandalone
	if target.OwningDirectory == "" {
		kind = analysis.TargetKindRoot
	}
	return analysis.AnalysisTarget{
		DisplayName:     displayNameForDirectory(target.OwningDirectory),
		ManifestPath:    target.ManifestPath,
		Kind:            kind,
		PackageManager:  string(target.PackageManager),
		Ecosystem:       string(target.Ecosystem),
		TargetID:        target.Identity,
		OwningDirectory: target.OwningDirectory,
		LocalFallback:   review.HasLocalFallback(target.PackageManager),
		FallbackReason:  fallbackUnavailableReason(target.Ecosystem),
	}
}
