package analysis

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/rad1092/gh-dependency-risk/internal/npm"
)

func TestScoreDriverContributions(t *testing.T) {
	now := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name        string
		build       func(time.Time) (Input, map[PackageVersion]time.Time)
		wantScore   int
		wantDrivers []string
	}{
		{
			name: "known vulnerabilities",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("vulnerable", "^1.0.0")),
						pkg("node_modules/app/node_modules/vulnerable", "vulnerable", "1.0.0", nil),
					),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("vulnerable", "^1.1.0")),
						pkg("node_modules/app/node_modules/vulnerable", "vulnerable", "1.1.0", nil),
					),
					[]ReviewChange{
						{Name: "vulnerable", Manifest: "package-lock.json", ChangeType: ChangeRemoved, Version: "1.0.0"},
						{Name: "vulnerable", Manifest: "package-lock.json", ChangeType: ChangeAdded, Version: "1.1.0", Vulnerabilities: []Vulnerability{{GHSAID: "GHSA-test", Severity: "high", Summary: "demo", URL: "https://example.com"}}},
					},
				)
				return input, nil
			},
			wantScore:   scoreKnownVulnerabilities,
			wantDrivers: []string{DriverKnownVulnerabilities},
		},
		{
			name: "added direct runtime",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(nil, nil, nil),
					manifest(runtimeDeps("runtime-new", "^1.0.0"), nil, nil),
					lockfile(),
					lockfile(pkg("node_modules/runtime-new", "runtime-new", "1.0.0", nil)),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreAddedDirectRuntime,
			wantDrivers: []string{DriverAddedDirectRuntime},
		},
		{
			name: "added direct dev",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(nil, nil, nil),
					manifest(nil, runtimeDeps("dev-new", "^1.0.0"), nil),
					lockfile(),
					lockfile(pkg("node_modules/dev-new", "dev-new", "1.0.0", nil)),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreAddedDirectDev,
			wantDrivers: []string{DriverAddedDirectDev},
		},
		{
			name: "major version bump",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("major-lib", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("major-lib", "^2.0.0"), nil, nil),
					lockfile(pkg("node_modules/major-lib", "major-lib", "1.2.0", nil)),
					lockfile(pkg("node_modules/major-lib", "major-lib", "2.0.0", nil)),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreMajorVersionBump,
			wantDrivers: []string{DriverMajorVersionBump},
		},
		{
			name: "recently published",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("fresh-lib", "^1.0.0")),
						pkg("node_modules/app/node_modules/fresh-lib", "fresh-lib", "1.0.0", nil),
					),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("fresh-lib", "^1.1.0")),
						pkg("node_modules/app/node_modules/fresh-lib", "fresh-lib", "1.1.0", nil),
					),
					nil,
				)
				return input, map[PackageVersion]time.Time{
					{Name: "fresh-lib", Version: "1.1.0"}: now.Add(-24 * time.Hour),
				}
			},
			wantScore:   scoreRecentlyPublished,
			wantDrivers: []string{DriverRecentlyPublished},
		},
		{
			name: "install script detected",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("script-lib", "^1.0.0")),
						pkg("node_modules/app/node_modules/script-lib", "script-lib", "1.0.0", nil),
					),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("script-lib", "^1.1.0")),
						pkg("node_modules/app/node_modules/script-lib", "script-lib", "1.1.0", nil, withInstallScript()),
					),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreInstallScript,
			wantDrivers: []string{DriverInstallScript},
		},
		{
			name: "platform restricted package",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("platform-lib", "^1.0.0")),
						pkg("node_modules/app/node_modules/platform-lib", "platform-lib", "1.0.0", nil),
					),
					lockfile(
						pkg("node_modules/app", "app", "1.0.0", runtimeDeps("platform-lib", "^1.1.0")),
						pkg("node_modules/app/node_modules/platform-lib", "platform-lib", "1.1.0", nil, withPlatform([]string{"linux"}, []string{"x64"})),
					),
					nil,
				)
				return input, nil
			},
			wantScore:   scorePlatformRestricted,
			wantDrivers: []string{DriverPlatformRestricted},
		},
		{
			name: "added transitive threshold",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(pkg("node_modules/app", "app", "1.0.0", nil)),
					lockfile(transitiveExpansion("app", 5)...),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreTransitiveFive,
			wantDrivers: []string{DriverTransitiveFive},
		},
		{
			name: "heavy added transitive threshold",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(pkg("node_modules/app", "app", "1.0.0", nil)),
					lockfile(transitiveExpansion("app", 15)...),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreTransitiveFive + scoreTransitiveFifteen,
			wantDrivers: []string{DriverTransitiveFive, DriverTransitiveFifteen},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input, publishedAt := tc.build(now)
			result := Analyze(input, publishedAt)
			if len(result.ChangedDependencies) == 0 {
				t.Fatalf("expected at least one dependency change")
			}
			for _, change := range result.ChangedDependencies {
				if change.Score != tc.wantScore {
					t.Fatalf("expected score %d, got %#v", tc.wantScore, change)
				}
				assertStringSetEqual(t, change.RiskDrivers, tc.wantDrivers)
			}
		})
	}
}

func TestRiskLevelThresholds(t *testing.T) {
	testCases := []struct {
		score int
		want  RiskLevel
	}{
		{score: 0, want: RiskLevelLow},
		{score: levelThresholdMedium - 1, want: RiskLevelLow},
		{score: levelThresholdMedium, want: RiskLevelMedium},
		{score: levelThresholdHigh - 1, want: RiskLevelMedium},
		{score: levelThresholdHigh, want: RiskLevelHigh},
		{score: levelThresholdCritical - 1, want: RiskLevelHigh},
		{score: levelThresholdCritical, want: RiskLevelCritical},
	}

	for _, tc := range testCases {
		if got := LevelForScore(tc.score); got != tc.want {
			t.Fatalf("expected score %d to map to %s, got %s", tc.score, tc.want, got)
		}
	}
}

func TestAggregateTargetScoreRules(t *testing.T) {
	testCases := []struct {
		name   string
		scores []int
		want   int
	}{
		{name: "empty", scores: nil, want: 0},
		{name: "single", scores: []int{48}, want: 48},
		{name: "high plus medium", scores: []int{48, 22}, want: 52},
		{name: "high plus low", scores: []int{48, 18}, want: 49},
		{name: "max plus capped bonus", scores: []int{100, 80, 70, 60, 50, 40}, want: 100},
		{name: "bonus cap without max cap", scores: []int{70, 20, 20, 20, 20, 20, 20}, want: 85},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aggregateTargetScore(tc.scores); got != tc.want {
				t.Fatalf("expected aggregate score %d, got %d", tc.want, got)
			}
		})
	}
}

func TestRepresentativeAnalysisScenarios(t *testing.T) {
	now := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name        string
		build       func(time.Time) (Input, map[PackageVersion]time.Time)
		wantScore   int
		wantLevel   RiskLevel
		wantDrivers []string
		wantActions []string
		wantNotes   []string
		wantChanges int
	}{
		{
			name: "clearly severe signal caps at one hundred",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				headPackages := transitiveExpansion("cap-lib", 15)
				headPackages[0].Version = "2.0.0"
				headPackages[0].HasInstallScript = true
				headPackages[0].OS = []string{"linux"}
				input := rootInput(
					now,
					manifest(runtimeDeps("cap-lib", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("cap-lib", "^2.0.0"), nil, nil),
					lockfile(pkg("node_modules/cap-lib", "cap-lib", "1.0.0", nil)),
					lockfile(headPackages...),
					[]ReviewChange{
						{Name: "cap-lib", Manifest: "package.json", ChangeType: ChangeRemoved, Version: "1.0.0"},
						{Name: "cap-lib", Manifest: "package.json", ChangeType: ChangeAdded, Version: "2.0.0", Vulnerabilities: []Vulnerability{{GHSAID: "GHSA-cap", Severity: "critical", Summary: "demo", URL: "https://example.com"}}},
					},
				)
				return input, map[PackageVersion]time.Time{
					{Name: "cap-lib", Version: "2.0.0"}: now.Add(-12 * time.Hour),
				}
			},
			wantScore:   changeScoreCap,
			wantLevel:   RiskLevelCritical,
			wantDrivers: []string{DriverKnownVulnerabilities, DriverMajorVersionBump, DriverRecentlyPublished, DriverInstallScript, DriverPlatformRestricted, DriverTransitiveFive, DriverTransitiveFifteen},
			wantActions: []string{ActionInspectTree, ActionInspectInstall, ActionReviewAdvisories, ActionReviewChangelog, ActionRunTargetedTests},
			wantChanges: 1,
		},
		{
			name: "several medium signals together",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(nil, nil, nil),
					manifest(runtimeDeps("fresh-runtime", "^1.0.0"), nil, nil),
					lockfile(),
					lockfile(pkg("node_modules/fresh-runtime", "fresh-runtime", "1.0.0", nil)),
					nil,
				)
				return input, map[PackageVersion]time.Time{
					{Name: "fresh-runtime", Version: "1.0.0"}: now.Add(-48 * time.Hour),
				}
			},
			wantScore:   scoreAddedDirectRuntime + scoreRecentlyPublished,
			wantLevel:   RiskLevelMedium,
			wantDrivers: []string{DriverAddedDirectRuntime, DriverRecentlyPublished},
			wantActions: []string{ActionRunTargetedTests},
			wantChanges: 1,
		},
		{
			name: "major bump plus install script",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("scripted-major", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("scripted-major", "^2.0.0"), nil, nil),
					lockfile(pkg("node_modules/scripted-major", "scripted-major", "1.0.0", nil)),
					lockfile(pkg("node_modules/scripted-major", "scripted-major", "2.0.0", nil, withInstallScript())),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreMajorVersionBump + scoreInstallScript,
			wantLevel:   RiskLevelMedium,
			wantDrivers: []string{DriverMajorVersionBump, DriverInstallScript},
			wantActions: []string{ActionInspectInstall, ActionReviewChangelog, ActionRunTargetedTests},
			wantChanges: 1,
		},
		{
			name: "non registry source stays informational",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(nil, nil, nil),
					manifest(runtimeDeps("git-lib", "github:example/git-lib#main"), nil, nil),
					lockfile(),
					lockfile(pkg("node_modules/git-lib", "git-lib", "1.0.0", nil, withResolved("git+https://github.com/example/git-lib.git"))),
					nil,
				)
				return input, nil
			},
			wantScore:   scoreAddedDirectRuntime,
			wantLevel:   RiskLevelLow,
			wantDrivers: []string{DriverAddedDirectRuntime},
			wantActions: []string{ActionRunTargetedTests, ActionValidateSources},
			wantNotes:   []string{NoteNonRegistrySource},
			wantChanges: 1,
		},
		{
			name: "transitive heavy change",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				input := rootInput(
					now,
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("app", "^1.0.0"), nil, nil),
					lockfile(pkg("node_modules/app", "app", "1.0.0", nil)),
					lockfile(transitiveExpansion("app", 15)...),
					nil,
				)
				return input, nil
			},
			wantScore:   aggregateTargetScore(repeatScore(scoreTransitiveFive+scoreTransitiveFifteen, 15)),
			wantLevel:   RiskLevelMedium,
			wantDrivers: []string{DriverTransitiveFive, DriverTransitiveFifteen},
			wantActions: []string{ActionInspectTree},
			wantChanges: 15,
		},
		{
			name: "no meaningful dependency change",
			build: func(now time.Time) (Input, map[PackageVersion]time.Time) {
				lock := lockfile(pkg("node_modules/stable", "stable", "1.0.0", nil))
				input := rootInput(
					now,
					manifest(runtimeDeps("stable", "^1.0.0"), nil, nil),
					manifest(runtimeDeps("stable", "^1.0.0"), nil, nil),
					lock,
					lock,
					nil,
				)
				return input, nil
			},
			wantScore:   0,
			wantLevel:   RiskLevelLow,
			wantDrivers: nil,
			wantActions: nil,
			wantChanges: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input, publishedAt := tc.build(now)
			result := Analyze(input, publishedAt)
			if result.Score != tc.wantScore {
				t.Fatalf("expected score %d, got %#v", tc.wantScore, result)
			}
			if result.Level != tc.wantLevel {
				t.Fatalf("expected level %s, got %#v", tc.wantLevel, result)
			}
			if len(result.ChangedDependencies) != tc.wantChanges {
				t.Fatalf("expected %d changes, got %#v", tc.wantChanges, result.ChangedDependencies)
			}
			assertStringSetEqual(t, result.RiskDrivers, tc.wantDrivers)
			assertStringSetEqual(t, result.RecommendedActions, tc.wantActions)
			if len(tc.wantNotes) > 0 {
				gotCodes := make([]string, 0, len(result.Notes))
				for _, note := range result.Notes {
					gotCodes = append(gotCodes, note.Code)
				}
				if !sameStringSet(gotCodes, tc.wantNotes) {
					t.Fatalf("expected notes %v, got %#v", tc.wantNotes, result.Notes)
				}
			}
		})
	}
}

func TestAnalyzeAddsApproximateAttributionNote(t *testing.T) {
	now := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)
	target := AnalysisTarget{
		DisplayName:       "apps/web",
		ManifestPath:      "apps/web/package.json",
		LockfilePath:      "package-lock.json",
		Kind:              TargetKindWorkspace,
		WorkspaceRootPath: "",
		PackageManager:    "npm",
	}
	result := Analyze(Input{
		Now:                       now,
		Target:                    target,
		DependencyReviewAvailable: false,
		BaseManifest:              manifest(runtimeDeps("direct", "^1.0.0"), nil, nil),
		HeadManifest:              manifest(runtimeDeps("direct", "^1.0.0"), nil, nil),
		BaseLockfile: lockfile(
			pkg("apps/web", "web", "", runtimeDeps("direct", "^1.0.0")),
			pkg("node_modules/direct", "direct", "1.0.0", nil),
		),
		HeadLockfile: lockfile(
			pkg("apps/web", "web", "", runtimeDeps("direct", "^1.0.0")),
			pkg("node_modules/direct", "direct", "1.0.0", runtimeDeps("shared", "^1.0.0")),
			pkg("packages/ui/node_modules/shared", "shared", "1.0.0", nil),
		),
	}, nil)

	if len(result.Notes) != 2 {
		t.Fatalf("expected fallback and approximate notes, got %#v", result.Notes)
	}
	if result.Notes[0].Code != NoteApproximateAttribution && result.Notes[1].Code != NoteApproximateAttribution {
		t.Fatalf("expected approximate attribution note, got %#v", result.Notes)
	}
	if result.AddedTransitiveCount != 1 {
		t.Fatalf("expected one approximate transitive package, got %d", result.AddedTransitiveCount)
	}
}

func rootInput(now time.Time, baseManifest, headManifest *npm.PackageManifest, baseLock, headLock *npm.Lockfile, review []ReviewChange) Input {
	return Input{
		Now:                       now,
		Target:                    AnalysisTarget{DisplayName: "root", ManifestPath: "package.json", LockfilePath: "package-lock.json", Kind: TargetKindRoot, PackageManager: "npm"},
		DependencyReviewAvailable: true,
		ReviewChanges:             review,
		BaseManifest:              baseManifest,
		HeadManifest:              headManifest,
		BaseLockfile:              baseLock,
		HeadLockfile:              headLock,
	}
}

func manifest(runtime, dev, optional map[string]string) *npm.PackageManifest {
	return &npm.PackageManifest{
		Dependencies:         copyStringMap(runtime),
		DevDependencies:      copyStringMap(dev),
		OptionalDependencies: copyStringMap(optional),
	}
}

func runtimeDeps(name, requirement string) map[string]string {
	return map[string]string{name: requirement}
}

func copyStringMap(source map[string]string) map[string]string {
	if source == nil {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func lockfile(packages ...npm.LockPackage) *npm.Lockfile {
	result := &npm.Lockfile{
		Manager:   "npm",
		Packages:  map[string]npm.LockPackage{},
		Importers: map[string]npm.LockImporter{},
	}
	for _, pkg := range packages {
		result.Packages[pkg.Path] = pkg
	}
	return result
}

type packageOption func(*npm.LockPackage)

func pkg(pkgPath, name, version string, deps map[string]string, options ...packageOption) npm.LockPackage {
	pkg := npm.LockPackage{
		Path:         pkgPath,
		Name:         name,
		Version:      version,
		Dependencies: copyStringMap(deps),
	}
	for _, option := range options {
		option(&pkg)
	}
	return pkg
}

func withInstallScript() packageOption {
	return func(pkg *npm.LockPackage) {
		pkg.HasInstallScript = true
	}
}

func withPlatform(osValues, cpuValues []string) packageOption {
	return func(pkg *npm.LockPackage) {
		pkg.OS = append([]string(nil), osValues...)
		pkg.CPU = append([]string(nil), cpuValues...)
	}
}

func withResolved(resolved string) packageOption {
	return func(pkg *npm.LockPackage) {
		pkg.Resolved = resolved
	}
}

func transitiveExpansion(rootName string, count int) []npm.LockPackage {
	deps := make(map[string]string, count)
	packages := make([]npm.LockPackage, 0, count+1)
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("transitive-%02d", i)
		deps[name] = "^1.0.0"
		packages = append(packages, pkg("node_modules/"+rootName+"/node_modules/"+name, name, "1.0.0", nil))
	}
	root := pkg("node_modules/"+rootName, rootName, "1.0.0", deps)
	return append([]npm.LockPackage{root}, packages...)
}

func repeatScore(score, count int) []int {
	result := make([]int, 0, count)
	for i := 0; i < count; i++ {
		result = append(result, score)
	}
	return result
}

func assertStringSetEqual(t *testing.T, got, want []string) {
	t.Helper()
	if !sameStringSet(got, want) {
		t.Fatalf("expected values %v, got %v", want, got)
	}
}

func sameStringSet(left, right []string) bool {
	leftSorted := append([]string(nil), left...)
	rightSorted := append([]string(nil), right...)
	slices.Sort(leftSorted)
	slices.Sort(rightSorted)
	return slices.Equal(leftSorted, rightSorted)
}
