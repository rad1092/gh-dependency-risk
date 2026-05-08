package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"gh-dep-risk/internal/npm"
	pythondeps "gh-dep-risk/internal/python"
	"gh-dep-risk/internal/review"
)

type repoDataCache struct {
	client     ghclient.Client
	repo       ghclient.Repo
	rawFiles   map[string][]byte
	manifests  map[string]*npm.PackageManifest
	lockfiles  map[string]*npm.Lockfile
	trees      map[string][]string
	workspaces map[string][]string
}

type discoveredTarget struct {
	ManifestPath string
	DisplayName  string
	Variants     []analysis.AnalysisTarget
}

func newRepoDataCache(client ghclient.Client, repo ghclient.Repo) *repoDataCache {
	return &repoDataCache{
		client:     client,
		repo:       repo,
		rawFiles:   map[string][]byte{},
		manifests:  map[string]*npm.PackageManifest{},
		lockfiles:  map[string]*npm.Lockfile{},
		trees:      map[string][]string{},
		workspaces: map[string][]string{},
	}
}

func (c *repoDataCache) listFiles(ctx context.Context, ref string) ([]string, error) {
	if files, ok := c.trees[ref]; ok {
		return append([]string(nil), files...), nil
	}
	files, err := c.client.ListRepositoryFiles(ctx, c.repo, ref)
	if err != nil {
		return nil, err
	}
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	c.trees[ref] = sorted
	return append([]string(nil), sorted...), nil
}

func (c *repoDataCache) manifest(ctx context.Context, ref, manifestPath string) (*npm.PackageManifest, error) {
	key := cacheKey(ref, manifestPath)
	if manifest, ok := c.manifests[key]; ok {
		return manifest, nil
	}
	data, err := c.file(ctx, ref, manifestPath)
	if err != nil {
		return nil, err
	}
	manifest, err := npm.ParsePackageManifest(data)
	if err != nil {
		return nil, err
	}
	c.manifests[key] = manifest
	return manifest, nil
}

func (c *repoDataCache) lockfile(ctx context.Context, ref, lockfilePath string) (*npm.Lockfile, error) {
	key := cacheKey(ref, lockfilePath)
	if lockfile, ok := c.lockfiles[key]; ok {
		return lockfile, nil
	}
	data, err := c.file(ctx, ref, lockfilePath)
	if err != nil {
		return nil, err
	}
	lockfile, err := npm.ParseLockfile(data)
	if err != nil {
		return nil, err
	}
	c.lockfiles[key] = lockfile
	return lockfile, nil
}

func (c *repoDataCache) pnpmWorkspacePatterns(ctx context.Context, ref, workspacePath string) ([]string, error) {
	key := cacheKey(ref, workspacePath)
	if patterns, ok := c.workspaces[key]; ok {
		return append([]string(nil), patterns...), nil
	}
	data, err := c.file(ctx, ref, workspacePath)
	if err != nil {
		return nil, err
	}
	patterns, err := npm.ParsePNPMWorkspacePatterns(data)
	if err != nil {
		return nil, err
	}
	c.workspaces[key] = append([]string(nil), patterns...)
	return append([]string(nil), patterns...), nil
}

func (c *repoDataCache) file(ctx context.Context, ref, filePath string) ([]byte, error) {
	key := cacheKey(ref, filePath)
	if data, ok := c.rawFiles[key]; ok {
		return append([]byte(nil), data...), nil
	}
	data, err := c.client.GetRepositoryFile(ctx, c.repo, filePath, ref)
	if err != nil {
		if errors.Is(err, ghclient.ErrNotFound) {
			c.rawFiles[key] = nil
			return nil, nil
		}
		return nil, err
	}
	c.rawFiles[key] = append([]byte(nil), data...)
	return append([]byte(nil), data...), nil
}

func discoverTargets(ctx context.Context, cache *repoDataCache, baseRef, headRef string) ([]discoveredTarget, error) {
	baseFiles, err := cache.listFiles(ctx, baseRef)
	if err != nil {
		return nil, err
	}
	headFiles, err := cache.listFiles(ctx, headRef)
	if err != nil {
		return nil, err
	}

	jsTargets, err := discoverJSTargets(ctx, cache, baseRef, headRef, baseFiles, headFiles)
	if err != nil {
		return nil, err
	}
	otherTargets, err := discoverAPIOnlyTargets(ctx, cache, baseRef, headRef, unionPaths(baseFiles, headFiles))
	if err != nil {
		return nil, err
	}
	return mergeDiscoveredTargets(jsTargets, otherTargets), nil
}

func discoverJSTargets(ctx context.Context, cache *repoDataCache, baseRef, headRef string, baseFiles, headFiles []string) ([]discoveredTarget, error) {
	manifestPaths := unionPaths(filterPaths(baseFiles, "package.json"), filterPaths(headFiles, "package.json"))
	npmLockfilePaths := pathSet(unionPaths(filterPaths(baseFiles, "package-lock.json"), filterPaths(headFiles, "package-lock.json")))
	pnpmLockfilePaths := pathSet(unionPaths(filterPaths(baseFiles, "pnpm-lock.yaml"), filterPaths(headFiles, "pnpm-lock.yaml")))
	yarnLockfilePaths := pathSet(unionPaths(filterPaths(baseFiles, "yarn.lock"), filterPaths(headFiles, "yarn.lock")))
	yarnRCPaths := pathSet(unionPaths(filterPaths(baseFiles, ".yarnrc.yml"), filterPaths(headFiles, ".yarnrc.yml")))
	bunLockfilePaths := pathSet(unionPaths(filterPaths(baseFiles, "bun.lock"), filterPaths(headFiles, "bun.lock")))
	bunBinaryLockfilePaths := pathSet(unionPaths(filterPaths(baseFiles, "bun.lockb"), filterPaths(headFiles, "bun.lockb")))
	pnpmWorkspacePaths := unionPaths(filterPaths(baseFiles, "pnpm-workspace.yaml"), filterPaths(headFiles, "pnpm-workspace.yaml"))
	manifestCache := map[string][2]*npm.PackageManifest{}
	for _, manifestPath := range manifestPaths {
		baseManifest, err := cache.manifest(ctx, baseRef, manifestPath)
		if err != nil {
			return nil, err
		}
		headManifest, err := cache.manifest(ctx, headRef, manifestPath)
		if err != nil {
			return nil, err
		}
		manifestCache[manifestPath] = [2]*npm.PackageManifest{baseManifest, headManifest}
	}

	npmWorkspaceRoots := map[string]string{}
	for _, manifestPath := range manifestPaths {
		patterns := workspacePatterns(manifestCache[manifestPath][0], manifestCache[manifestPath][1])
		if len(patterns) == 0 {
			continue
		}
		rootDir := manifestDir(manifestPath)
		for _, candidate := range manifestPaths {
			if candidate == manifestPath {
				continue
			}
			if !matchesWorkspaceTarget(rootDir, patterns, candidate) {
				continue
			}
			if _, ok := npmLockfilePaths[lockfilePathForDir(rootDir)]; ok {
				npmWorkspaceRoots[candidate] = rootDir
			}
		}
	}

	pnpmWorkspaceRoots := map[string]string{}
	for _, workspacePath := range pnpmWorkspacePaths {
		rootDir := manifestDir(workspacePath)
		lockfilePath := pnpmLockfilePathForDir(rootDir)
		if _, ok := pnpmLockfilePaths[lockfilePath]; !ok {
			continue
		}
		basePatterns, err := cache.pnpmWorkspacePatterns(ctx, baseRef, workspacePath)
		if err != nil {
			return nil, err
		}
		headPatterns, err := cache.pnpmWorkspacePatterns(ctx, headRef, workspacePath)
		if err != nil {
			return nil, err
		}
		patterns := unionStrings(basePatterns, headPatterns)
		if len(patterns) == 0 {
			continue
		}
		rootManifestPath := manifestPathForDir(rootDir)
		for _, candidate := range manifestPaths {
			if candidate == rootManifestPath {
				continue
			}
			if !matchesWorkspaceTarget(rootDir, patterns, candidate) {
				continue
			}
			pnpmWorkspaceRoots[candidate] = rootDir
		}
	}

	yarnWorkspaceRoots := map[string]string{}
	for _, manifestPath := range manifestPaths {
		patterns := workspacePatterns(manifestCache[manifestPath][0], manifestCache[manifestPath][1])
		if len(patterns) == 0 {
			continue
		}
		rootDir := manifestDir(manifestPath)
		if _, ok := yarnLockfilePaths[yarnLockfilePathForDir(rootDir)]; !ok {
			continue
		}
		for _, candidate := range manifestPaths {
			if candidate == manifestPath {
				continue
			}
			if !matchesWorkspaceTarget(rootDir, patterns, candidate) {
				continue
			}
			yarnWorkspaceRoots[candidate] = rootDir
		}
	}

	bunWorkspaceRoots := map[string]string{}
	for _, manifestPath := range manifestPaths {
		patterns := workspacePatterns(manifestCache[manifestPath][0], manifestCache[manifestPath][1])
		if len(patterns) == 0 {
			continue
		}
		rootDir := manifestDir(manifestPath)
		if !hasBunSignal(manifestCache[manifestPath], rootDir, bunLockfilePaths, bunBinaryLockfilePaths) {
			continue
		}
		for _, candidate := range manifestPaths {
			if candidate == manifestPath {
				continue
			}
			if !matchesWorkspaceTarget(rootDir, patterns, candidate) {
				continue
			}
			bunWorkspaceRoots[candidate] = rootDir
		}
	}

	grouped := map[string][]analysis.AnalysisTarget{}
	for _, manifestPath := range manifestPaths {
		dir := manifestDir(manifestPath)
		npmLockfilePath := lockfilePathForDir(dir)
		pnpmLockfilePath := pnpmLockfilePathForDir(dir)
		yarnLockfilePath := yarnLockfilePathForDir(dir)

		if workspaceRoot, ok := npmWorkspaceRoots[manifestPath]; ok {
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:       displayNameForManifest(manifestPath),
				ManifestPath:      manifestPath,
				LockfilePath:      lockfilePathForDir(workspaceRoot),
				Kind:              analysis.TargetKindWorkspace,
				WorkspaceRootPath: workspaceRoot,
				PackageManager:    "npm",
				Ecosystem:         string(review.EcosystemNPM),
				TargetID:          review.TargetIdentity(manifestPath, review.EcosystemNPM, review.PackageManagerNPM),
				OwningDirectory:   dir,
				LocalFallback:     true,
			})
		} else if _, ok := npmLockfilePaths[npmLockfilePath]; ok {
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:     displayNameForManifest(manifestPath),
				ManifestPath:    manifestPath,
				LockfilePath:    npmLockfilePath,
				Kind:            kindForManifest(manifestPath),
				PackageManager:  "npm",
				Ecosystem:       string(review.EcosystemNPM),
				TargetID:        review.TargetIdentity(manifestPath, review.EcosystemNPM, review.PackageManagerNPM),
				OwningDirectory: dir,
				LocalFallback:   true,
			})
		}

		if workspaceRoot, ok := pnpmWorkspaceRoots[manifestPath]; ok {
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:       displayNameForManifest(manifestPath),
				ManifestPath:      manifestPath,
				LockfilePath:      pnpmLockfilePathForDir(workspaceRoot),
				Kind:              analysis.TargetKindWorkspace,
				WorkspaceRootPath: workspaceRoot,
				PackageManager:    "pnpm",
				Ecosystem:         string(review.EcosystemPNPM),
				TargetID:          review.TargetIdentity(manifestPath, review.EcosystemPNPM, review.PackageManagerPNPM),
				OwningDirectory:   dir,
				LocalFallback:     true,
			})
		} else if _, ok := pnpmLockfilePaths[pnpmLockfilePath]; ok {
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:     displayNameForManifest(manifestPath),
				ManifestPath:    manifestPath,
				LockfilePath:    pnpmLockfilePath,
				Kind:            kindForManifest(manifestPath),
				PackageManager:  "pnpm",
				Ecosystem:       string(review.EcosystemPNPM),
				TargetID:        review.TargetIdentity(manifestPath, review.EcosystemPNPM, review.PackageManagerPNPM),
				OwningDirectory: dir,
				LocalFallback:   true,
			})
		}

		if workspaceRoot, ok := yarnWorkspaceRoots[manifestPath]; ok {
			workspaceRootManifest := manifestPathForDir(workspaceRoot)
			yarnManager := "yarn"
			if hasYarnBerrySignal(manifestCache[manifestPath], dir, yarnRCPaths) || hasYarnBerrySignal(manifestCache[workspaceRootManifest], workspaceRoot, yarnRCPaths) {
				yarnManager = packageManagerYarnBerry
			}
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:       displayNameForManifest(manifestPath),
				ManifestPath:      manifestPath,
				LockfilePath:      yarnLockfilePathForDir(workspaceRoot),
				Kind:              analysis.TargetKindWorkspace,
				WorkspaceRootPath: workspaceRoot,
				PackageManager:    yarnManager,
				Ecosystem:         string(review.EcosystemYarn),
				TargetID:          review.TargetIdentity(manifestPath, review.EcosystemYarn, review.PackageManagerYarn),
				OwningDirectory:   dir,
				LocalFallback:     true,
			})
		} else if _, ok := yarnLockfilePaths[yarnLockfilePath]; ok {
			yarnManager := "yarn"
			if hasYarnBerrySignal(manifestCache[manifestPath], dir, yarnRCPaths) {
				yarnManager = packageManagerYarnBerry
			}
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:     displayNameForManifest(manifestPath),
				ManifestPath:    manifestPath,
				LockfilePath:    yarnLockfilePath,
				Kind:            kindForManifest(manifestPath),
				PackageManager:  yarnManager,
				Ecosystem:       string(review.EcosystemYarn),
				TargetID:        review.TargetIdentity(manifestPath, review.EcosystemYarn, review.PackageManagerYarn),
				OwningDirectory: dir,
				LocalFallback:   true,
			})
		}

		if workspaceRoot, ok := bunWorkspaceRoots[manifestPath]; ok {
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:       displayNameForManifest(manifestPath),
				ManifestPath:      manifestPath,
				LockfilePath:      bunLockfileForDir(workspaceRoot, bunLockfilePaths, bunBinaryLockfilePaths),
				Kind:              analysis.TargetKindWorkspace,
				WorkspaceRootPath: workspaceRoot,
				PackageManager:    packageManagerBun,
				Ecosystem:         string(review.EcosystemNPM),
				TargetID:          review.TargetIdentity(manifestPath, review.EcosystemNPM, review.PackageManager(packageManagerBun)),
				OwningDirectory:   dir,
				LocalFallback:     true,
			})
		} else if hasBunSignal(manifestCache[manifestPath], dir, bunLockfilePaths, bunBinaryLockfilePaths) {
			grouped[manifestPath] = append(grouped[manifestPath], analysis.AnalysisTarget{
				DisplayName:     displayNameForManifest(manifestPath),
				ManifestPath:    manifestPath,
				LockfilePath:    bunLockfileForDir(dir, bunLockfilePaths, bunBinaryLockfilePaths),
				Kind:            kindForManifest(manifestPath),
				PackageManager:  packageManagerBun,
				Ecosystem:       string(review.EcosystemNPM),
				TargetID:        review.TargetIdentity(manifestPath, review.EcosystemNPM, review.PackageManager(packageManagerBun)),
				OwningDirectory: dir,
				LocalFallback:   true,
			})
		}
	}

	return groupedTargets(grouped), nil
}

func discoverAPIOnlyTargets(ctx context.Context, cache *repoDataCache, baseRef, headRef string, files []string) ([]discoveredTarget, error) {
	grouped := map[string][]analysis.AnalysisTarget{}
	poetryLockfiles := pathSet(filterPaths(files, "poetry.lock"))
	uvLockfiles := pathSet(filterPaths(files, "uv.lock"))
	goSumFiles := pathSet(filterPaths(files, "go.sum"))
	for _, filePath := range files {
		cleaned := normalizeRepoPath(filePath)
		base := path.Base(cleaned)
		var ecosystem review.Ecosystem
		var manager review.PackageManager
		localFallback := false
		switch base {
		case "Cargo.toml":
			ecosystem, manager = review.EcosystemCargo, review.PackageManagerCargo
		case "composer.json":
			ecosystem, manager = review.EcosystemComposer, review.PackageManagerComposer
		case "go.mod":
			ecosystem, manager = review.EcosystemGoModules, review.PackageManagerGo
			localFallback = true
		case "pom.xml":
			ecosystem, manager = review.EcosystemMaven, review.PackageManagerMaven
		case "requirements.txt":
			ecosystem, manager = review.EcosystemPip, review.PackageManagerPip
			localFallback = true
		case "Gemfile":
			ecosystem, manager = review.EcosystemRubyGems, review.PackageManagerBundler
		case "Package.swift":
			ecosystem, manager = review.EcosystemSwiftPM, review.PackageManagerSwiftPM
		case "pyproject.toml":
			ok, err := detectPEP621PythonManifest(ctx, cache, baseRef, headRef, cleaned)
			if err != nil {
				return nil, err
			}
			if ok {
				ecosystem, manager = review.EcosystemPip, review.PackageManagerPyProject
				localFallback = true
				break
			}
			ok, err = detectPoetryManifest(ctx, cache, baseRef, headRef, cleaned, poetryLockfiles)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			ecosystem, manager = review.EcosystemPoetry, review.PackageManagerPoetry
			localFallback = true
		default:
			continue
		}
		lockfilePath := ""
		if manager == review.PackageManagerPoetry {
			candidate := poetryLockfilePathForDir(manifestDir(cleaned))
			if _, ok := poetryLockfiles[candidate]; ok {
				lockfilePath = candidate
			}
		}
		if manager == review.PackageManagerPyProject {
			candidate := uvLockfilePathForDir(manifestDir(cleaned))
			if _, ok := uvLockfiles[candidate]; ok {
				lockfilePath = candidate
			}
		}
		if manager == review.PackageManagerGo {
			candidate := goSumPathForDir(manifestDir(cleaned))
			if _, ok := goSumFiles[candidate]; ok {
				lockfilePath = candidate
			}
		}
		fallbackReason := ""
		if !localFallback {
			fallbackReason = fallbackUnavailableReason(ecosystem)
		}
		grouped[cleaned] = append(grouped[cleaned], analysis.AnalysisTarget{
			DisplayName:     displayNameForManifest(cleaned),
			ManifestPath:    cleaned,
			LockfilePath:    lockfilePath,
			Kind:            kindForManifest(cleaned),
			PackageManager:  string(manager),
			Ecosystem:       string(ecosystem),
			TargetID:        review.TargetIdentity(cleaned, ecosystem, manager),
			OwningDirectory: manifestDir(cleaned),
			LocalFallback:   localFallback,
			FallbackReason:  fallbackReason,
		})
	}
	return groupedTargets(grouped), nil
}

func detectPEP621PythonManifest(ctx context.Context, cache *repoDataCache, baseRef, headRef, manifestPath string) (bool, error) {
	for _, ref := range []string{baseRef, headRef} {
		data, err := cache.file(ctx, ref, manifestPath)
		if err != nil {
			return false, err
		}
		ok, err := pythondeps.HasPEP621Dependencies(data)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func detectPoetryManifest(ctx context.Context, cache *repoDataCache, baseRef, headRef, manifestPath string, poetryLockfiles map[string]struct{}) (bool, error) {
	for _, ref := range []string{baseRef, headRef} {
		data, err := cache.file(ctx, ref, manifestPath)
		if err != nil {
			return false, err
		}
		if pythondeps.HasPoetryProject(data) {
			return true, nil
		}
	}
	if _, ok := poetryLockfiles[poetryLockfilePathForDir(manifestDir(manifestPath))]; ok {
		return true, nil
	}
	return false, nil
}

func groupedTargets(grouped map[string][]analysis.AnalysisTarget) []discoveredTarget {
	targets := make([]discoveredTarget, 0, len(grouped))
	for manifestPath, variants := range grouped {
		sort.Slice(variants, func(i, j int) bool {
			if variants[i].PackageManager == variants[j].PackageManager {
				if variants[i].Ecosystem == variants[j].Ecosystem {
					return variants[i].LockfilePath < variants[j].LockfilePath
				}
				return variants[i].Ecosystem < variants[j].Ecosystem
			}
			return variants[i].PackageManager < variants[j].PackageManager
		})
		targets = append(targets, discoveredTarget{
			ManifestPath: manifestPath,
			DisplayName:  displayNameForManifest(manifestPath),
			Variants:     variants,
		})
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].ManifestPath < targets[j].ManifestPath
	})
	return targets
}

func mergeDiscoveredTargets(left, right []discoveredTarget) []discoveredTarget {
	grouped := map[string]discoveredTarget{}
	for _, source := range append(append([]discoveredTarget(nil), left...), right...) {
		current, ok := grouped[source.ManifestPath]
		if !ok {
			grouped[source.ManifestPath] = source
			continue
		}
		current.Variants = mergeVariants(current.Variants, source.Variants)
		if current.DisplayName == "" {
			current.DisplayName = source.DisplayName
		}
		grouped[source.ManifestPath] = current
	}
	merged := make([]discoveredTarget, 0, len(grouped))
	for _, target := range grouped {
		merged = append(merged, target)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].ManifestPath < merged[j].ManifestPath
	})
	return merged
}

func mergeVariants(left, right []analysis.AnalysisTarget) []analysis.AnalysisTarget {
	merged := map[string]analysis.AnalysisTarget{}
	for _, variant := range append(append([]analysis.AnalysisTarget(nil), left...), right...) {
		key := variant.Key()
		if existing, ok := merged[key]; ok {
			merged[key] = preferRicherVariant(existing, variant)
			continue
		}
		merged[key] = variant
	}
	variants := make([]analysis.AnalysisTarget, 0, len(merged))
	for _, variant := range merged {
		variants = append(variants, variant)
	}
	sort.Slice(variants, func(i, j int) bool {
		if variants[i].PackageManager == variants[j].PackageManager {
			return variants[i].Ecosystem < variants[j].Ecosystem
		}
		return variants[i].PackageManager < variants[j].PackageManager
	})
	return variants
}

func preferRicherVariant(left, right analysis.AnalysisTarget) analysis.AnalysisTarget {
	result := left
	if result.DisplayName == "" {
		result.DisplayName = right.DisplayName
	}
	if result.LockfilePath == "" {
		result.LockfilePath = right.LockfilePath
	}
	if result.Kind != analysis.TargetKindWorkspace && right.Kind == analysis.TargetKindWorkspace {
		result.Kind = right.Kind
		result.WorkspaceRootPath = right.WorkspaceRootPath
	}
	if result.PackageManager == "" {
		result.PackageManager = right.PackageManager
	}
	if result.Ecosystem == "" {
		result.Ecosystem = right.Ecosystem
	}
	if result.TargetID == "" {
		result.TargetID = right.TargetID
	}
	if result.OwningDirectory == "" {
		result.OwningDirectory = right.OwningDirectory
	}
	if !result.LocalFallback && right.LocalFallback {
		result.LocalFallback = true
		result.FallbackReason = ""
	}
	if result.FallbackReason == "" {
		result.FallbackReason = right.FallbackReason
	}
	return result
}

func filterTargetsByRequestedPaths(targets []discoveredTarget, requested []string) ([]discoveredTarget, error) {
	if len(requested) == 0 {
		return append([]discoveredTarget(nil), targets...), nil
	}

	manifestIndex := map[string]discoveredTarget{}
	directoryIndex := map[string][]discoveredTarget{}
	for _, target := range targets {
		manifestIndex[target.ManifestPath] = target
		directory := target.directory()
		directoryIndex[directory] = append(directoryIndex[directory], target)
	}
	selected := make([]discoveredTarget, 0, len(requested))
	seen := map[string]struct{}{}
	for _, raw := range requested {
		normalized := normalizeRepoPath(raw)
		target, ok := manifestIndex[normalized]
		if !ok {
			matches := directoryIndex[normalized]
			switch len(matches) {
			case 1:
				target = matches[0]
				ok = true
			case 0:
				return nil, fmt.Errorf("unknown dependency target path %q. Run --list-targets to inspect detected targets, or try one of: %s", raw, targetPathExamples(targets))
			default:
				return nil, fmt.Errorf("target path %q is ambiguous. Use an exact manifest path instead: %s", raw, strings.Join(manifestPaths(matches), ", "))
			}
		}
		if _, ok := seen[target.ManifestPath]; ok {
			continue
		}
		seen[target.ManifestPath] = struct{}{}
		selected = append(selected, target)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].ManifestPath < selected[j].ManifestPath
	})
	return selected, nil
}

func selectChangedTargets(targets []discoveredTarget, files []ghclient.PullRequestFile) ([]analysis.AnalysisTarget, error) {
	return selectChangedTargetsWithReview(targets, files, nil)
}

func selectChangedTargetsWithReview(targets []discoveredTarget, files []ghclient.PullRequestFile, reviewChanges map[string][]analysis.ReviewChange) ([]analysis.AnalysisTarget, error) {
	changed := map[string]struct{}{}
	for _, file := range files {
		changed[normalizeRepoPath(file.Filename)] = struct{}{}
	}
	selected := make([]analysis.AnalysisTarget, 0, len(targets))
	for _, target := range targets {
		if !targetIsChanged(target, changed, reviewChanges) {
			continue
		}
		resolved, err := resolveTargetVariant(target, changed, reviewChanges)
		if err != nil {
			return nil, err
		}
		selected = append(selected, resolved)
	}
	return selected, nil
}

func formatTargets(targets []discoveredTarget) string {
	if len(targets) == 0 {
		return "Detected dependency targets:\n- none\n"
	}
	lines := make([]string, 0, len(targets)+1)
	lines = append(lines, "Detected dependency targets:")
	for _, target := range targets {
		if target.ambiguous() {
			lines = append(lines, fmt.Sprintf("- %s [ambiguous]\n  manifest: %s\n  ecosystems: %s\n  package managers: %s\n  lockfiles: %s\n  note: analysis will use the single changed manifest/lockfile combination if the PR makes this target unambiguous", displayTargetName(target), target.ManifestPath, strings.Join(target.ecosystems(), ", "), strings.Join(target.packageManagers(), ", "), strings.Join(target.lockfiles(), ", ")))
			continue
		}
		variant := target.Variants[0]
		line := fmt.Sprintf("- %s [%s, ecosystem=%s, manager=%s]\n  manifest: %s", displayTargetName(target), variant.Kind, variant.Ecosystem, managerLabel(variant), target.ManifestPath)
		if variant.LockfilePath != "" {
			line += fmt.Sprintf("\n  lockfile: %s", variant.LockfilePath)
		}
		if variant.WorkspaceRootPath != "" {
			line += fmt.Sprintf("\n  workspace root: %s", variant.WorkspaceRootPath)
		}
		if !variant.LocalFallback && variant.FallbackReason != "" {
			line += fmt.Sprintf("\n  fallback: %s", variant.FallbackReason)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
}

func targetPathExamples(targets []discoveredTarget) string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, target.ManifestPath)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return "package.json"
	}
	if len(paths) > 4 {
		paths = paths[:4]
	}
	return strings.Join(paths, ", ")
}

func displayTargetName(target discoveredTarget) string {
	if target.DisplayName != "" {
		return target.DisplayName
	}
	if dir := target.directory(); dir != "" {
		return dir
	}
	return "root"
}

func cacheKey(ref, filePath string) string {
	return ref + "@" + filePath
}

func filterPaths(paths []string, base string) []string {
	filtered := make([]string, 0)
	for _, filePath := range paths {
		if path.Base(filePath) == base {
			filtered = append(filtered, normalizeRepoPath(filePath))
		}
	}
	sort.Strings(filtered)
	return filtered
}

func unionPaths(left, right []string) []string {
	set := map[string]struct{}{}
	for _, filePath := range append(append([]string(nil), left...), right...) {
		set[normalizeRepoPath(filePath)] = struct{}{}
	}
	paths := make([]string, 0, len(set))
	for filePath := range set {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	return paths
}

func unionStrings(left, right []string) []string {
	set := map[string]struct{}{}
	for _, value := range append(append([]string(nil), left...), right...) {
		if strings.TrimSpace(value) == "" {
			continue
		}
		set[value] = struct{}{}
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func pathSet(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, filePath := range paths {
		set[filePath] = struct{}{}
	}
	return set
}

func manifestDir(manifestPath string) string {
	cleaned := normalizeRepoPath(manifestPath)
	if cleaned == "package.json" || cleaned == "pnpm-workspace.yaml" {
		return ""
	}
	dir := path.Dir(cleaned)
	if dir == "." {
		return ""
	}
	return dir
}

func manifestPathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "package.json"
	}
	return cleaned + "/package.json"
}

func lockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "package-lock.json"
	}
	return cleaned + "/package-lock.json"
}

func pnpmLockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "pnpm-lock.yaml"
	}
	return cleaned + "/pnpm-lock.yaml"
}

func yarnLockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "yarn.lock"
	}
	return cleaned + "/yarn.lock"
}

func bunLockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "bun.lock"
	}
	return cleaned + "/bun.lock"
}

func bunBinaryLockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "bun.lockb"
	}
	return cleaned + "/bun.lockb"
}

func bunLockfileForDir(dir string, bunLockfilePaths, bunBinaryLockfilePaths map[string]struct{}) string {
	text := bunLockfilePathForDir(dir)
	if _, ok := bunLockfilePaths[text]; ok {
		return text
	}
	binary := bunBinaryLockfilePathForDir(dir)
	if _, ok := bunBinaryLockfilePaths[binary]; ok {
		return binary
	}
	return ""
}

func poetryLockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "poetry.lock"
	}
	return cleaned + "/poetry.lock"
}

func uvLockfilePathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "uv.lock"
	}
	return cleaned + "/uv.lock"
}

func goSumPathForDir(dir string) string {
	cleaned := normalizeRepoPath(dir)
	if cleaned == "" {
		return "go.sum"
	}
	return cleaned + "/go.sum"
}

func normalizeRepoPath(value string) string {
	cleaned := path.Clean(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	switch cleaned {
	case ".", "/":
		return ""
	default:
		return strings.TrimPrefix(cleaned, "./")
	}
}

func workspacePatterns(base, head *npm.PackageManifest) []string {
	set := map[string]struct{}{}
	for _, manifest := range []*npm.PackageManifest{base, head} {
		if manifest == nil {
			continue
		}
		for _, pattern := range manifest.Workspaces {
			set[pattern] = struct{}{}
		}
	}
	patterns := make([]string, 0, len(set))
	for pattern := range set {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	return patterns
}

func matchesWorkspaceTarget(rootDir string, patterns []string, manifestPath string) bool {
	dir := manifestDir(manifestPath)
	if dir == "" {
		return false
	}
	relative, ok := relativeToRoot(rootDir, dir)
	if !ok || relative == "" {
		return false
	}
	return npm.MatchWorkspacePatternSet(patterns, relative)
}

func relativeToRoot(rootDir, targetDir string) (string, bool) {
	root := normalizeRepoPath(rootDir)
	target := normalizeRepoPath(targetDir)
	if root == "" {
		return target, true
	}
	if target == root {
		return "", true
	}
	prefix := root + "/"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}
	return strings.TrimPrefix(target, prefix), true
}

func displayNameForManifest(manifestPath string) string {
	return displayNameForDirectory(manifestDir(manifestPath))
}

func displayNameForDirectory(dir string) string {
	if dir != "" {
		return dir
	}
	return "root"
}

func kindForManifest(manifestPath string) analysis.TargetKind {
	if manifestDir(manifestPath) == "" {
		return analysis.TargetKindRoot
	}
	return analysis.TargetKindStandalone
}

func (t discoveredTarget) ambiguous() bool {
	return len(t.Variants) > 1
}

func (t discoveredTarget) lockfiles() []string {
	lockfiles := make([]string, 0, len(t.Variants))
	for _, variant := range t.Variants {
		if variant.LockfilePath == "" {
			continue
		}
		lockfiles = append(lockfiles, variant.LockfilePath)
	}
	sort.Strings(lockfiles)
	return lockfiles
}

func (t discoveredTarget) packageManagers() []string {
	seen := map[string]struct{}{}
	managers := make([]string, 0, len(t.Variants))
	for _, variant := range t.Variants {
		label := managerLabel(variant)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		managers = append(managers, label)
	}
	sort.Strings(managers)
	return managers
}

func (t discoveredTarget) ecosystems() []string {
	seen := map[string]struct{}{}
	ecosystems := make([]string, 0, len(t.Variants))
	for _, variant := range t.Variants {
		if _, ok := seen[variant.Ecosystem]; ok {
			continue
		}
		seen[variant.Ecosystem] = struct{}{}
		ecosystems = append(ecosystems, variant.Ecosystem)
	}
	sort.Strings(ecosystems)
	return ecosystems
}

func (t discoveredTarget) directory() string {
	if len(t.Variants) > 0 && t.Variants[0].OwningDirectory != "" {
		return t.Variants[0].OwningDirectory
	}
	return manifestDir(t.ManifestPath)
}

func targetIsChanged(target discoveredTarget, changed map[string]struct{}, reviewChanges map[string][]analysis.ReviewChange) bool {
	for _, variant := range target.Variants {
		for _, key := range reviewChangeKeysForTarget(variant) {
			if len(reviewChanges[key]) > 0 {
				return true
			}
		}
	}
	if _, ok := changed[target.ManifestPath]; ok {
		return true
	}
	for _, variant := range target.Variants {
		if _, ok := changed[variant.LockfilePath]; ok {
			return true
		}
	}
	return false
}

func resolveTargetVariant(target discoveredTarget, changed map[string]struct{}, reviewChanges map[string][]analysis.ReviewChange) (analysis.AnalysisTarget, error) {
	if len(target.Variants) == 1 {
		return target.Variants[0], nil
	}

	reviewVariants := make([]analysis.AnalysisTarget, 0, len(target.Variants))
	for _, variant := range target.Variants {
		if len(reviewChanges[variant.Key()]) == 0 {
			continue
		}
		reviewVariants = append(reviewVariants, variant)
	}
	if len(reviewVariants) == 1 {
		return reviewVariants[0], nil
	}
	if len(reviewVariants) > 1 {
		return analysis.AnalysisTarget{}, fmt.Errorf("target %q is ambiguous because multiple supported ecosystems or package managers changed for the same manifest (%s). Keep only one lockfile/manager per target or narrow the PR before rerunning", target.ManifestPath, strings.Join(target.packageManagers(), ", "))
	}

	for _, variant := range target.Variants {
		if len(reviewChangesForTarget(reviewChanges, variant)) == 0 {
			continue
		}
		reviewVariants = append(reviewVariants, variant)
	}
	if len(reviewVariants) == 1 {
		return reviewVariants[0], nil
	}
	if len(reviewVariants) > 1 {
		return analysis.AnalysisTarget{}, fmt.Errorf("target %q is ambiguous because multiple supported ecosystems or package managers changed for the same manifest (%s). Keep only one lockfile/manager per target or narrow the PR before rerunning", target.ManifestPath, strings.Join(target.packageManagers(), ", "))
	}

	changedVariants := make([]analysis.AnalysisTarget, 0, len(target.Variants))
	for _, variant := range target.Variants {
		if variant.LockfilePath == "" {
			continue
		}
		if _, ok := changed[variant.LockfilePath]; ok {
			changedVariants = append(changedVariants, variant)
		}
	}
	switch len(changedVariants) {
	case 1:
		return changedVariants[0], nil
	case 0:
		return analysis.AnalysisTarget{}, fmt.Errorf("target %q is ambiguous because multiple supported lockfiles are present (%s). Change exactly one lockfile in the PR, remove the unused lockfile, or pass an exact manifest path that resolves to a single target", target.ManifestPath, strings.Join(target.lockfiles(), ", "))
	default:
		return analysis.AnalysisTarget{}, fmt.Errorf("target %q is ambiguous because multiple supported lockfiles changed in the same directory (%s). Keep only one package manager lockfile per target or narrow the PR before rerunning", target.ManifestPath, strings.Join(target.lockfiles(), ", "))
	}
}

func reviewChangesForTarget(reviewChanges map[string][]analysis.ReviewChange, target analysis.AnalysisTarget) []analysis.ReviewChange {
	var result []analysis.ReviewChange
	for _, key := range reviewChangeKeysForTarget(target) {
		result = append(result, reviewChanges[key]...)
	}
	return result
}

func reviewChangeKeysForTarget(target analysis.AnalysisTarget) []string {
	keys := []string{target.Key()}
	if target.PackageManager == packageManagerBun {
		npmKey := review.TargetIdentity(target.ManifestPath, review.EcosystemNPM, review.PackageManagerNPM)
		if npmKey != target.Key() {
			keys = append(keys, npmKey)
		}
	}
	return keys
}

func managerLabel(target analysis.AnalysisTarget) string {
	if target.PackageManager != "" {
		return target.PackageManager
	}
	if target.Ecosystem != "" {
		return target.Ecosystem
	}
	return "unknown"
}

func manifestPaths(targets []discoveredTarget) []string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, target.ManifestPath)
	}
	sort.Strings(paths)
	return paths
}

func fallbackUnavailableReason(ecosystem review.Ecosystem) string {
	switch ecosystem {
	case review.EcosystemCargo, review.EcosystemComposer, review.EcosystemMaven, review.EcosystemRubyGems, review.EcosystemSwiftPM:
		return "dependency review is required for this ecosystem in this release"
	default:
		return ""
	}
}
