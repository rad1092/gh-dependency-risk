package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"gh-dep-risk/internal/analysis"
	ghclient "gh-dep-risk/internal/github"
	"gh-dep-risk/internal/npm"
	pythondeps "gh-dep-risk/internal/python"
	"gh-dep-risk/internal/render"
)

type RunPROptions struct {
	PRArg       string
	Repo        string
	Format      string
	Lang        string
	BundleDir   string
	Comment     bool
	FailLevel   analysis.RiskLevel
	NoRegistry  bool
	Paths       []string
	ListTargets bool
}

type RunPRDependencies struct {
	GitHub   ghclient.Client
	Registry npm.RegistryClient
	Stdout   io.Writer
	Stderr   io.Writer
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func RunPR(ctx context.Context, deps RunPRDependencies, opts RunPROptions) error {
	if deps.GitHub == nil {
		return &ExitError{Code: 1, Err: errors.New("missing GitHub client")}
	}
	if deps.Stdout == nil || deps.Stderr == nil {
		return &ExitError{Code: 1, Err: errors.New("stdout/stderr writers are required")}
	}

	repo, prNumber, err := resolveTarget(ctx, deps.GitHub, opts)
	if err != nil {
		return wrapGitHubError(err)
	}

	pr, err := deps.GitHub.GetPullRequest(ctx, repo, prNumber)
	if err != nil {
		return wrapGitHubError(err)
	}

	cache := newRepoDataCache(deps.GitHub, repo)
	targets, err := discoverTargets(ctx, cache, pr.BaseSHA, pr.HeadSHA)
	if err != nil {
		return wrapGitHubError(err)
	}
	selectedTargets, err := filterTargetsByRequestedPaths(targets, opts.Paths)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if opts.ListTargets {
		if _, err := io.WriteString(deps.Stdout, formatTargets(selectedTargets)); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		return nil
	}
	reviewSnapshot, err := loadDependencyReviewSnapshot(ctx, deps.GitHub, repo, pr.BaseSHA, pr.HeadSHA)
	if err != nil {
		return wrapGitHubError(err)
	}
	targets = mergeDiscoveredTargets(targets, reviewSnapshot.DerivedTargets)
	selectedTargets, err = filterTargetsByRequestedPaths(targets, opts.Paths)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}

	files, err := deps.GitHub.ListPullRequestFiles(ctx, repo, prNumber)
	if err != nil {
		return wrapGitHubError(err)
	}
	resolvedTargets, err := selectChangedTargetsWithReview(selectedTargets, files, reviewSnapshot.TargetChanges)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if len(resolvedTargets) == 0 {
		return &ExitError{Code: 2, Err: errors.New("no supported dependency change found")}
	}

	now := time.Now().UTC()
	inputs := make([]analysis.Input, 0, len(resolvedTargets))
	localInputs := make([]analysis.LocalInput, 0)
	for _, target := range resolvedTargets {
		reviewChanges := append([]analysis.ReviewChange(nil), reviewSnapshot.TargetChanges[target.Key()]...)
		if !reviewSnapshot.Available && !target.LocalFallback {
			return &ExitError{Code: 1, Err: fmt.Errorf("dependency review is unavailable and %s cannot be analyzed with local fallback in this release. Pass a PR from a repository where GitHub dependency review is enabled, or narrow to an npm/pnpm/yarn/python direct target with a supported manifest", target.ManifestPath)}
		}
		if shouldUsePythonLocalFallback(target, reviewSnapshot.Available) {
			input, err := loadPythonLocalInput(ctx, cache, pr.BaseSHA, pr.HeadSHA, target)
			if err != nil {
				return &ExitError{Code: 1, Err: err}
			}
			localInputs = append(localInputs, input)
			continue
		}
		baseManifest, headManifest, baseLockfile, headLockfile, err := loadLocalTargetData(ctx, cache, pr.BaseSHA, pr.HeadSHA, target, reviewSnapshot.Available)
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}

		inputs = append(inputs, analysis.Input{
			Now:                       now,
			Target:                    target,
			DependencyReviewAvailable: reviewSnapshot.Available,
			ReviewChanges:             reviewChanges,
			BaseManifest:              baseManifest,
			HeadManifest:              headManifest,
			BaseLockfile:              baseLockfile,
			HeadLockfile:              headLockfile,
		})
	}

	publishedAt := map[analysis.PackageVersion]time.Time{}
	if !opts.NoRegistry && deps.Registry != nil {
		registryTargets := collectRegistryTargets(inputs)
		for _, target := range registryTargets {
			published, err := deps.Registry.PublishedAt(ctx, target.Name, target.Version)
			if err != nil {
				continue
			}
			publishedAt[target] = published
		}
	}

	targetResults := make([]analysis.TargetAnalysisResult, 0, len(inputs)+len(localInputs))
	unsupportedOnly := false
	for _, input := range inputs {
		result := analysis.Analyze(input, publishedAt)
		if !analysis.HasMeaningfulChange(result) {
			continue
		}
		targetResults = append(targetResults, analysis.TargetResult(input.Target, result))
	}
	for _, input := range localInputs {
		result := analysis.AnalyzeLocalDirectDependencies(input)
		if !analysis.HasMeaningfulChange(result) {
			if hasUnsupportedDependencyNote(result.Notes) {
				unsupportedOnly = true
			}
			continue
		}
		targetResults = append(targetResults, analysis.TargetResult(input.Target, result))
	}
	if len(targetResults) == 0 {
		if unsupportedOnly {
			fmt.Fprintln(deps.Stderr, "unsupported dependency entries were present, but no supported dependency change was found")
		}
		return &ExitError{Code: 2, Err: errors.New("no supported dependency change found")}
	}
	result := analysis.AggregateResults(targetResults)
	if !analysis.HasMeaningfulChange(result) {
		return &ExitError{Code: 2, Err: errors.New("no supported dependency change found")}
	}

	report := render.Report{
		Repo: repo.FullName(),
		PR: render.PullRequestMetadata{
			Number:      pr.Number,
			URL:         pr.URL,
			Title:       pr.Title,
			Draft:       pr.Draft,
			BaseSHA:     pr.BaseSHA,
			HeadSHA:     pr.HeadSHA,
			AuthorLogin: pr.AuthorLogin,
		},
		Analysis: result,
	}

	output, err := render.Render(report, opts.Format, opts.Lang)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if _, err := io.WriteString(deps.Stdout, output); err != nil {
		return &ExitError{Code: 1, Err: err}
	}

	if strings.TrimSpace(opts.BundleDir) != "" {
		if _, err := render.WriteBundle(report, opts.Lang, opts.BundleDir); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}

	if opts.Comment {
		viewerLogin, err := deps.GitHub.ViewerLogin(ctx, repo)
		if err != nil {
			return wrapCommentModeError(repo, err)
		}
		commentBody, err := render.Render(report, "markdown", opts.Lang)
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		if err := ghclient.UpsertMarkerComment(ctx, deps.GitHub, repo, pr.Number, viewerLogin, commentBody, deps.Stderr); err != nil {
			return wrapCommentModeError(repo, err)
		}
	}

	if opts.FailLevel != analysis.RiskLevelNone && result.Score >= opts.FailLevel.Threshold() {
		return &ExitError{
			Code: 3,
			Err:  fmt.Errorf("risk score %d meets fail level %s", result.Score, opts.FailLevel),
		}
	}
	return nil
}

func resolveTarget(ctx context.Context, client ghclient.Client, opts RunPROptions) (ghclient.Repo, int, error) {
	repo, number, repoFromArg, err := parsePRArg(opts.PRArg)
	if err != nil {
		return ghclient.Repo{}, 0, err
	}
	if opts.Repo != "" {
		repo, err = client.ResolveRepo(ctx, opts.Repo)
		if err != nil {
			return ghclient.Repo{}, 0, err
		}
	} else if !repoFromArg {
		repo, err = client.ResolveRepo(ctx, "")
		if err != nil {
			return ghclient.Repo{}, 0, err
		}
	}
	if number == 0 {
		number, err = client.ResolveCurrentPR(ctx, repo)
		if err != nil {
			return ghclient.Repo{}, 0, fmt.Errorf("could not resolve a pull request for the current branch in %s: %w. Pass a PR number, a full PR URL, or --repo OWNER/REPO explicitly", repo.FullName(), err)
		}
	}
	return repo, number, nil
}

func parsePRArg(arg string) (ghclient.Repo, int, bool, error) {
	if strings.TrimSpace(arg) == "" {
		return ghclient.Repo{}, 0, false, nil
	}
	if number, err := strconv.Atoi(arg); err == nil {
		return ghclient.Repo{}, number, false, nil
	}
	parsed, err := url.Parse(arg)
	if err != nil {
		return ghclient.Repo{}, 0, false, fmt.Errorf("invalid PR argument %q", arg)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ghclient.Repo{}, 0, false, fmt.Errorf("unsupported PR URL %q", arg)
	}
	if parsed.Host == "" {
		return ghclient.Repo{}, 0, false, fmt.Errorf("unsupported PR URL %q", arg)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return ghclient.Repo{}, 0, false, fmt.Errorf("unsupported PR URL %q", arg)
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil {
		return ghclient.Repo{}, 0, false, fmt.Errorf("invalid PR number in URL %q", arg)
	}
	return ghclient.Repo{
		Host:  parsed.Host,
		Owner: parts[0],
		Name:  parts[1],
	}, number, true, nil
}

func collectRegistryTargets(inputs []analysis.Input) []analysis.PackageVersion {
	seen := map[analysis.PackageVersion]struct{}{}
	for _, input := range inputs {
		for _, target := range analysis.CollectRegistryTargets(input) {
			seen[target] = struct{}{}
		}
	}
	targets := make([]analysis.PackageVersion, 0, len(seen))
	for target := range seen {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Name == targets[j].Name {
			return targets[i].Version < targets[j].Version
		}
		return targets[i].Name < targets[j].Name
	})
	return targets
}

func wrapGitHubError(err error) error {
	if err == nil {
		return nil
	}
	if ghclient.IsPermissionError(err) || ghclient.IsAuthError(err) {
		return &ExitError{
			Code: 4,
			Err:  fmt.Errorf("%w. Run `gh auth login` or provide GH_TOKEN/GITHUB_TOKEN with repository access", err),
		}
	}
	return &ExitError{Code: 1, Err: err}
}

func wrapCommentModeError(repo ghclient.Repo, err error) error {
	if err == nil {
		return nil
	}
	if ghclient.IsPermissionError(err) || ghclient.IsAuthError(err) {
		return &ExitError{
			Code: 4,
			Err:  fmt.Errorf("comment mode requires permission to read the authenticated GitHub user and write PR issue comments in %s: %w. Check repo access, token scopes, and cross-repo workflow comment limits", repo.FullName(), err),
		}
	}
	return wrapGitHubError(err)
}

func loadLocalTargetData(ctx context.Context, cache *repoDataCache, baseSHA, headSHA string, target analysis.AnalysisTarget, dependencyReviewAvailable bool) (*npm.PackageManifest, *npm.PackageManifest, *npm.Lockfile, *npm.Lockfile, error) {
	var baseManifest, headManifest *npm.PackageManifest
	if path.Base(target.ManifestPath) == "package.json" {
		var err error
		baseManifest, err = cache.manifest(ctx, baseSHA, target.ManifestPath)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		headManifest, err = cache.manifest(ctx, headSHA, target.ManifestPath)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if !target.LocalFallback || strings.TrimSpace(target.LockfilePath) == "" {
		return baseManifest, headManifest, nil, nil, nil
	}

	baseLockfile, err := cache.lockfile(ctx, baseSHA, target.LockfilePath)
	if err != nil {
		if dependencyReviewAvailable && npm.IsUnsupportedYarnFallback(err) {
			return baseManifest, headManifest, nil, nil, nil
		}
		return nil, nil, nil, nil, err
	}
	headLockfile, err := cache.lockfile(ctx, headSHA, target.LockfilePath)
	if err != nil {
		if dependencyReviewAvailable && npm.IsUnsupportedYarnFallback(err) {
			return baseManifest, headManifest, nil, nil, nil
		}
		return nil, nil, nil, nil, err
	}
	return baseManifest, headManifest, baseLockfile, headLockfile, nil
}

func shouldUsePythonLocalFallback(target analysis.AnalysisTarget, dependencyReviewAvailable bool) bool {
	if dependencyReviewAvailable {
		return false
	}
	switch target.PackageManager {
	case "pip", "pyproject":
		return true
	default:
		return false
	}
}

func loadPythonLocalInput(ctx context.Context, cache *repoDataCache, baseSHA, headSHA string, target analysis.AnalysisTarget) (analysis.LocalInput, error) {
	baseData, err := cache.file(ctx, baseSHA, target.ManifestPath)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	headData, err := cache.file(ctx, headSHA, target.ManifestPath)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	baseResult, err := parsePythonManifest(target.ManifestPath, baseData)
	if err != nil {
		return analysis.LocalInput{}, err
	}
	headResult, err := parsePythonManifest(target.ManifestPath, headData)
	if err != nil {
		return analysis.LocalInput{}, err
	}

	return analysis.LocalInput{
		Target:                    target,
		DependencyReviewAvailable: false,
		BaseDependencies:          convertPythonDependencies(baseResult.Dependencies),
		HeadDependencies:          convertPythonDependencies(headResult.Dependencies),
		Unsupported:               convertPythonUnsupported(target.ManifestPath, baseResult.Unsupported, headResult.Unsupported),
	}, nil
}

func parsePythonManifest(manifestPath string, data []byte) (pythondeps.ParseResult, error) {
	switch path.Base(manifestPath) {
	case "requirements.txt":
		return pythondeps.ParseRequirements(data)
	case "pyproject.toml":
		return pythondeps.ParsePyProject(data)
	default:
		return pythondeps.ParseResult{}, fmt.Errorf("unsupported Python manifest %s", manifestPath)
	}
}

func convertPythonDependencies(dependencies []pythondeps.Dependency) []analysis.LocalDependency {
	converted := make([]analysis.LocalDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		converted = append(converted, analysis.LocalDependency{
			Name:        dependency.Name,
			Requirement: dependency.Requirement,
			Version:     dependency.Version,
			Source:      dependency.Source,
			Scope:       pythonScope(dependency.Scope),
		})
	}
	return converted
}

func pythonScope(scope pythondeps.Scope) analysis.DependencyScope {
	switch scope {
	case pythondeps.ScopeOptional:
		return analysis.ScopeOptional
	default:
		return analysis.ScopeRuntime
	}
}

func convertPythonUnsupported(manifestPath string, groups ...[]pythondeps.UnsupportedEntry) []analysis.LocalUnsupportedEntry {
	converted := []analysis.LocalUnsupportedEntry{}
	for _, group := range groups {
		for _, entry := range group {
			converted = append(converted, analysis.LocalUnsupportedEntry{
				Manifest: manifestPath,
				Line:     entry.Line,
				Text:     entry.Text,
				Reason:   entry.Reason,
			})
		}
	}
	return converted
}

func hasUnsupportedDependencyNote(notes []analysis.Note) bool {
	for _, note := range notes {
		if note.Code == analysis.NoteUnsupportedDependency {
			return true
		}
	}
	return false
}
