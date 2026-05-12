package analysis

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/rad1092/gh-dependency-risk/internal/npm"
)

type candidateSummary struct {
	Name            string
	Manifest        string
	ChangeType      ChangeType
	Scope           DependencyScope
	Direct          bool
	FromVersion     string
	ToVersion       string
	FromRequirement string
	ToRequirement   string
	Resolved        string
	Vulnerabilities []Vulnerability
	HeadPackage     npm.LockPackage
}

type targetLockViews struct {
	Base                   npm.TargetPackages
	Head                   npm.TargetPackages
	AddedTransitive        int
	AddedTransitivePaths   []string
	ApproximateAttribution bool
}

func Analyze(input Input, publishedAt map[PackageVersion]time.Time) AnalysisResult {
	directNames := targetDirectNames(input)
	views := buildTargetLockViews(input, directNames)
	candidates := collectCandidateSummaries(input, views, directNames)

	changes := make([]DependencyChange, 0, len(candidates))
	notes := make([]Note, 0)
	for _, candidate := range candidates {
		change := DependencyChange{
			Name:                 candidate.Name,
			Manifest:             candidate.Manifest,
			Target:               input.Target.DisplayName,
			ChangeType:           candidate.ChangeType,
			Scope:                candidate.Scope,
			Direct:               candidate.Direct,
			FromVersion:          candidate.FromVersion,
			ToVersion:            candidate.ToVersion,
			FromRequirement:      candidate.FromRequirement,
			ToRequirement:        candidate.ToRequirement,
			Resolved:             candidate.Resolved,
			Vulnerabilities:      append([]Vulnerability(nil), candidate.Vulnerabilities...),
			AddedTransitiveCount: views.AddedTransitive,
		}

		score, drivers := scoreChange(input, candidate, views, publishedAt)

		change.Score = score
		change.Level = LevelForScore(score)
		change.RiskDrivers = uniqueStrings(drivers)
		if candidate.Resolved != "" && !npm.IsRegistrySource(candidate.Resolved) {
			notes = append(notes, Note{
				Code:       NoteNonRegistrySource,
				Dependency: candidate.Name,
				Detail:     npm.DescribeSource(candidate.Resolved),
			})
		}
		changes = append(changes, change)
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Score == changes[j].Score {
			return changes[i].Name < changes[j].Name
		}
		return changes[i].Score > changes[j].Score
	})

	if !input.DependencyReviewAvailable {
		notes = append(notes, Note{Code: NoteDependencyReviewFallback})
	}
	if views.ApproximateAttribution {
		notes = append(notes, Note{
			Code:   NoteApproximateAttribution,
			Detail: input.Target.DisplayName,
		})
	}

	score := aggregateScore(changes)
	return AnalysisResult{
		DependencyReviewAvailable: input.DependencyReviewAvailable,
		Score:                     score,
		Level:                     LevelForScore(score),
		BlastRadius:               deriveBlastRadius(changes, views.AddedTransitive),
		ChangedDependencies:       changes,
		RiskDrivers:               collectDrivers(changes),
		RecommendedActions:        recommendedActions(changes, notes),
		QuickCommands:             quickCommands(input.Target, changes),
		Notes:                     uniqueNotes(notes),
		AddedTransitiveCount:      views.AddedTransitive,
		addedTransitiveKeys:       append([]string{}, views.AddedTransitivePaths...),
	}
}

func AggregateResults(targets []TargetAnalysisResult) AnalysisResult {
	if len(targets) == 0 {
		return AnalysisResult{
			DependencyReviewAvailable: true,
			Level:                     RiskLevelLow,
			BlastRadius:               BlastRadiusLow,
		}
	}

	sortedTargets := append([]TargetAnalysisResult(nil), targets...)
	sort.Slice(sortedTargets, func(i, j int) bool {
		if sortedTargets[i].Score == sortedTargets[j].Score {
			return sortedTargets[i].Target.DisplayName < sortedTargets[j].Target.DisplayName
		}
		return sortedTargets[i].Score > sortedTargets[j].Score
	})

	changes := make([]DependencyChange, 0)
	notes := make([]Note, 0)
	drivers := make([]string, 0)
	actions := make([]string, 0)
	commands := make([]string, 0)
	scores := make([]int, 0, len(sortedTargets))
	dependencyReviewAvailable := true
	addedTransitiveKeys := map[string]struct{}{}
	addedTransitiveFallback := 0

	for _, target := range sortedTargets {
		scores = append(scores, target.Score)
		changes = append(changes, append([]DependencyChange(nil), target.ChangedDependencies...)...)
		notes = append(notes, append([]Note(nil), target.Notes...)...)
		drivers = append(drivers, target.RiskDrivers...)
		actions = append(actions, target.RecommendedActions...)
		commands = append(commands, target.QuickCommands...)
		dependencyReviewAvailable = dependencyReviewAvailable && target.DependencyReviewAvailable
		if target.addedTransitiveKeys != nil {
			for _, key := range target.addedTransitiveKeys {
				addedTransitiveKeys[key] = struct{}{}
			}
			continue
		}
		addedTransitiveFallback += target.AddedTransitiveCount
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Score == changes[j].Score {
			if changes[i].Target == changes[j].Target {
				return changes[i].Name < changes[j].Name
			}
			return changes[i].Target < changes[j].Target
		}
		return changes[i].Score > changes[j].Score
	})

	score := aggregateTargetScore(scores)
	addedTransitive := len(addedTransitiveKeys) + addedTransitiveFallback
	return AnalysisResult{
		DependencyReviewAvailable: dependencyReviewAvailable,
		Score:                     score,
		Level:                     LevelForScore(score),
		BlastRadius:               deriveBlastRadius(changes, addedTransitive),
		ChangedDependencies:       changes,
		RiskDrivers:               uniqueStrings(drivers),
		RecommendedActions:        uniqueStrings(actions),
		QuickCommands:             uniqueStrings(commands),
		Notes:                     uniqueNotes(notes),
		AddedTransitiveCount:      addedTransitive,
		Targets:                   sortedTargets,
		addedTransitiveKeys:       sortedKeys(addedTransitiveKeys),
	}
}

func TargetResult(target AnalysisTarget, result AnalysisResult) TargetAnalysisResult {
	return TargetAnalysisResult{
		Target:                    target,
		DependencyReviewAvailable: result.DependencyReviewAvailable,
		Score:                     result.Score,
		Level:                     result.Level,
		BlastRadius:               result.BlastRadius,
		ChangedDependencies:       append([]DependencyChange(nil), result.ChangedDependencies...),
		RiskDrivers:               append([]string(nil), result.RiskDrivers...),
		RecommendedActions:        append([]string(nil), result.RecommendedActions...),
		QuickCommands:             append([]string(nil), result.QuickCommands...),
		Notes:                     append([]Note(nil), result.Notes...),
		AddedTransitiveCount:      result.AddedTransitiveCount,
		addedTransitiveKeys:       append([]string{}, result.addedTransitiveKeys...),
	}
}

func collectCandidateSummaries(input Input, views targetLockViews, directNames []string) []candidateSummary {
	reviewMap := map[string]candidateSummary{}
	for _, change := range input.ReviewChanges {
		if change.Name == "" {
			continue
		}
		current := reviewMap[change.Name]
		current.Name = change.Name
		current.Manifest = change.Manifest
		switch change.ChangeType {
		case ChangeAdded:
			current.ToVersion = change.Version
			if current.ChangeType == ChangeRemoved {
				current.ChangeType = ChangeUpdated
			} else if current.ChangeType == "" {
				current.ChangeType = ChangeAdded
			}
			current.Vulnerabilities = append(current.Vulnerabilities, change.Vulnerabilities...)
		case ChangeRemoved:
			current.FromVersion = change.Version
			if current.ChangeType == ChangeAdded {
				current.ChangeType = ChangeUpdated
			} else if current.ChangeType == "" {
				current.ChangeType = ChangeRemoved
			}
		}
		reviewMap[change.Name] = current
	}

	candidatesByName := map[string]candidateSummary{}
	if len(reviewMap) > 0 {
		for name, current := range reviewMap {
			fillCandidateFromState(&current, input, views, directNames)
			candidatesByName[name] = current
		}
	} else {
		for name, current := range collectManifestAndLockCandidates(input, views, directNames) {
			fillCandidateFromState(&current, input, views, directNames)
			candidatesByName[name] = current
		}
	}

	candidates := make([]candidateSummary, 0, len(candidatesByName))
	for _, candidate := range candidatesByName {
		if candidate.ChangeType == "" {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	return candidates
}

func collectManifestAndLockCandidates(input Input, views targetLockViews, directNames []string) map[string]candidateSummary {
	candidates := map[string]candidateSummary{}
	allNames := map[string]struct{}{}
	for _, name := range directNames {
		allNames[name] = struct{}{}
	}
	for name := range allNames {
		candidate := candidateSummary{Name: name}
		fillCandidateFromState(&candidate, input, views, directNames)
		if candidate.ChangeType != "" {
			candidates[name] = candidate
		}
	}
	for name, candidate := range collectTransitiveCandidates(views) {
		if _, ok := candidates[name]; !ok {
			candidates[name] = candidate
		}
	}
	return candidates
}

func collectTransitiveCandidates(views targetLockViews) map[string]candidateSummary {
	candidates := map[string]candidateSummary{}
	headPaths := sortedLockPaths(views.Head.Transitive)
	basePaths := views.Base.Transitive
	for _, pkgPath := range headPaths {
		headPkg := views.Head.Transitive[pkgPath]
		basePkg, ok := basePaths[pkgPath]
		if ok && basePkg.Version == headPkg.Version {
			continue
		}
		current := candidates[headPkg.Name]
		current.Name = headPkg.Name
		current.Scope = ScopeTransitive
		current.Direct = false
		current.ToVersion = headPkg.Version
		current.Resolved = headPkg.Resolved
		current.HeadPackage = headPkg
		if ok {
			current.ChangeType = ChangeUpdated
			current.FromVersion = basePkg.Version
		} else {
			current.ChangeType = ChangeAdded
		}
		candidates[headPkg.Name] = current
	}
	for _, pkgPath := range sortedLockPaths(basePaths) {
		basePkg := basePaths[pkgPath]
		if _, ok := views.Head.Transitive[pkgPath]; ok {
			continue
		}
		current := candidates[basePkg.Name]
		current.Name = basePkg.Name
		current.Scope = ScopeTransitive
		current.Direct = false
		current.ChangeType = ChangeRemoved
		current.FromVersion = basePkg.Version
		candidates[basePkg.Name] = current
	}
	return candidates
}

func fillCandidateFromState(candidate *candidateSummary, input Input, views targetLockViews, directNames []string) {
	baseScope, baseDirect := manifestScope(candidate.Name, input.BaseManifest)
	headScope, headDirect := manifestScope(candidate.Name, input.HeadManifest)
	if !baseDirect {
		_, baseDirect = views.Base.Direct[candidate.Name]
	}
	if !headDirect {
		_, headDirect = views.Head.Direct[candidate.Name]
	}
	if headDirect {
		candidate.Scope = headScope
		candidate.Direct = true
	} else if baseDirect {
		candidate.Scope = baseScope
		candidate.Direct = true
	}
	if candidate.Scope == "" && candidate.Direct {
		candidate.Scope = ScopeUnknown
	}
	if candidate.Scope == "" {
		candidate.Scope = ScopeTransitive
	}

	if candidate.Manifest == "" {
		if candidate.Direct {
			candidate.Manifest = input.Target.ManifestPath
		} else {
			candidate.Manifest = input.Target.LockfilePath
		}
	}

	baseRequirement := input.BaseManifest.Requirement(candidate.Name)
	headRequirement := input.HeadManifest.Requirement(candidate.Name)
	basePkg, basePkgOK := directOrAnyPackage(views.Base, candidate.Name, candidate.Direct)
	headPkg, headPkgOK := directOrAnyPackage(views.Head, candidate.Name, candidate.Direct)

	if candidate.FromRequirement == "" {
		candidate.FromRequirement = baseRequirement
	}
	if candidate.ToRequirement == "" {
		candidate.ToRequirement = headRequirement
	}
	if candidate.FromVersion == "" && basePkgOK {
		candidate.FromVersion = basePkg.Version
	}
	if candidate.ToVersion == "" && headPkgOK {
		candidate.ToVersion = headPkg.Version
	}
	if candidate.Resolved == "" && headPkgOK {
		candidate.Resolved = headPkg.Resolved
	}
	if headPkgOK {
		candidate.HeadPackage = headPkg
	}

	if candidate.ChangeType == "" {
		switch {
		case !baseDirect && !basePkgOK && (headDirect || headPkgOK):
			candidate.ChangeType = ChangeAdded
		case (baseDirect || basePkgOK) && !headDirect && !headPkgOK:
			candidate.ChangeType = ChangeRemoved
		case valuesDiffer(candidate.FromVersion, candidate.ToVersion) || valuesDiffer(candidate.FromRequirement, candidate.ToRequirement):
			candidate.ChangeType = ChangeUpdated
		}
	}
}

func valuesDiffer(left, right string) bool {
	return strings.TrimSpace(left) != strings.TrimSpace(right)
}

func manifestScope(name string, manifest *npm.PackageManifest) (DependencyScope, bool) {
	scope, ok := manifest.Scope(name)
	if !ok {
		return "", false
	}
	switch scope {
	case "runtime":
		return ScopeRuntime, true
	case "dev":
		return ScopeDev, true
	case "optional":
		return ScopeOptional, true
	default:
		return ScopeUnknown, true
	}
}

func targetDirectNames(input Input) []string {
	set := map[string]struct{}{}
	for _, name := range input.BaseManifest.DirectNames() {
		set[name] = struct{}{}
	}
	for _, name := range input.HeadManifest.DirectNames() {
		set[name] = struct{}{}
	}
	if len(set) == 0 {
		for name := range input.BaseLockfile.TargetRootDependencies(input.Target.Directory()) {
			set[name] = struct{}{}
		}
		for name := range input.HeadLockfile.TargetRootDependencies(input.Target.Directory()) {
			set[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func buildTargetLockViews(input Input, directNames []string) targetLockViews {
	views := targetLockViews{}
	targetDir := input.Target.Directory()
	if input.BaseLockfile != nil {
		views.Base = input.BaseLockfile.CollectTargetPackages(targetDir, directNames)
	}
	if input.HeadLockfile != nil {
		views.Head = input.HeadLockfile.CollectTargetPackages(targetDir, directNames)
	}
	if input.HeadLockfile != nil {
		views.AddedTransitivePaths, views.ApproximateAttribution = input.HeadLockfile.AddedTransitivePathsForTarget(input.BaseLockfile, targetDir, directNames)
		views.AddedTransitive = len(views.AddedTransitivePaths)
	} else {
		views.ApproximateAttribution = views.Base.Approximate
	}
	views.ApproximateAttribution = views.ApproximateAttribution || views.Base.Approximate || views.Head.Approximate
	return views
}

func directOrAnyPackage(view npm.TargetPackages, name string, direct bool) (npm.LockPackage, bool) {
	if direct {
		pkg, ok := view.Direct[name]
		return pkg, ok
	}
	return packageByName(view.All, name)
}

func packageByName(packages map[string]npm.LockPackage, name string) (npm.LockPackage, bool) {
	paths := sortedLockPaths(packages)
	for _, pkgPath := range paths {
		pkg := packages[pkgPath]
		if pkg.Name == name {
			return pkg, true
		}
	}
	return npm.LockPackage{}, false
}

func sortedLockPaths(packages map[string]npm.LockPackage) []string {
	paths := make([]string, 0, len(packages))
	for pkgPath := range packages {
		paths = append(paths, pkgPath)
	}
	sort.Strings(paths)
	return paths
}

func isMajorBump(fromVersion, toVersion, fromRequirement, toRequirement string) bool {
	left := fromVersion
	if left == "" {
		left = fromRequirement
	}
	right := toVersion
	if right == "" {
		right = toRequirement
	}
	fromMajor, fromOK := npm.MajorVersion(left)
	toMajor, toOK := npm.MajorVersion(right)
	return fromOK && toOK && toMajor > fromMajor
}

func deriveBlastRadius(changes []DependencyChange, addedTransitiveCount int) BlastRadius {
	hasRuntime := false
	hasManyChanges := len(changes) >= 3
	for _, change := range changes {
		if change.Scope == ScopeRuntime || change.Scope == ScopeOptional {
			hasRuntime = true
		}
		if (change.Scope == ScopeRuntime || change.Scope == ScopeOptional) && change.ChangeType == ChangeAdded && change.Score >= 40 {
			return BlastRadiusHigh
		}
	}
	switch {
	case hasRuntime && addedTransitiveCount >= 15:
		return BlastRadiusHigh
	case hasRuntime || addedTransitiveCount >= 5 || hasManyChanges:
		return BlastRadiusMedium
	default:
		return BlastRadiusLow
	}
}

func collectDrivers(changes []DependencyChange) []string {
	set := map[string]struct{}{}
	for _, change := range changes {
		for _, driver := range change.RiskDrivers {
			set[driver] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func recommendedActions(changes []DependencyChange, notes []Note) []string {
	set := map[string]struct{}{}
	for _, change := range changes {
		for _, driver := range change.RiskDrivers {
			switch driver {
			case DriverKnownVulnerabilities:
				set[ActionReviewAdvisories] = struct{}{}
			case DriverInstallScript:
				set[ActionInspectInstall] = struct{}{}
			case DriverMajorVersionBump:
				set[ActionReviewChangelog] = struct{}{}
				set[ActionRunTargetedTests] = struct{}{}
			case DriverTransitiveFive, DriverTransitiveFifteen:
				set[ActionInspectTree] = struct{}{}
			default:
				if change.Direct {
					set[ActionRunTargetedTests] = struct{}{}
				}
			}
		}
	}
	for _, note := range notes {
		if note.Code == NoteNonRegistrySource {
			set[ActionValidateSources] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func quickCommands(target AnalysisTarget, changes []DependencyChange) []string {
	prefix := ""
	if dir := target.Directory(); dir != "" {
		prefix = "cd " + dir + " && "
	}
	listCommand := ""
	packageCommand := ""
	viewCommand := ""
	switch target.PackageManager {
	case "npm":
		listCommand = "npm ls --all"
		packageCommand = "npm ls "
		viewCommand = "npm view "
	case "pnpm":
		listCommand = "pnpm list --depth Infinity"
		packageCommand = "pnpm why "
		viewCommand = "pnpm view "
	case "yarn":
		listCommand = "yarn list --depth=9999"
		packageCommand = "yarn why "
	default:
		return nil
	}
	commands := []string{prefix + listCommand}
	if len(changes) > 0 {
		top := changes[0]
		commands = append(commands, prefix+packageCommand+top.Name)
		if viewCommand != "" && top.ToVersion != "" {
			commands = append(commands, prefix+viewCommand+top.Name+"@"+top.ToVersion+" time --json")
		}
	}
	return uniqueStrings(commands)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func uniqueStrings(values []string) []string {
	set := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueNotes(notes []Note) []Note {
	seen := map[string]struct{}{}
	result := make([]Note, 0, len(notes))
	for _, note := range notes {
		key := note.Code + "|" + note.Dependency + "|" + note.Detail
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, note)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].Code + result[i].Dependency + result[i].Detail
		right := result[j].Code + result[j].Dependency + result[j].Detail
		return left < right
	})
	return result
}

func normalizeTargetDisplayName(target AnalysisTarget) string {
	if target.DisplayName != "" {
		return target.DisplayName
	}
	if dir := target.Directory(); dir != "" {
		return dir
	}
	return "root"
}

func targetHeading(target AnalysisTarget) string {
	label := normalizeTargetDisplayName(target)
	switch target.Kind {
	case TargetKindWorkspace:
		return fmt.Sprintf("%s (%s)", label, path.Base(target.WorkspaceRootPath))
	default:
		return label
	}
}
