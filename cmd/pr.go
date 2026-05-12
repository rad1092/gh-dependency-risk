package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
	"github.com/rad1092/gh-dependency-risk/internal/app"
	"github.com/rad1092/gh-dependency-risk/internal/config"
	ghclient "github.com/rad1092/gh-dependency-risk/internal/github"
	"github.com/rad1092/gh-dependency-risk/internal/npm"
)

type multiStringFlag []string

func (f *multiStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *multiStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func runPR(stdout, stderr io.Writer, args []string) int {
	fs := flag.NewFlagSet("pr", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opts app.RunPROptions
	var failLevel string
	var paths multiStringFlag

	defaults := defaultPROptions()

	fs.StringVar(&opts.Repo, "repo", defaults.Repo, "repository in OWNER/REPO form")
	fs.StringVar(&opts.Format, "format", defaults.Format, "output format: human|json|markdown")
	fs.StringVar(&opts.Lang, "lang", defaults.Lang, "output language: ko|en")
	fs.BoolVar(&opts.Comment, "comment", false, "upsert a PR timeline comment")
	fs.StringVar(&failLevel, "fail-level", string(defaults.FailLevel), "fail threshold: low|medium|high|critical|none")
	fs.BoolVar(&opts.NoRegistry, "no-registry", false, "skip npm-compatible registry publish-age lookups")
	fs.StringVar(&opts.BundleDir, "bundle-dir", defaults.BundleDir, "write human/json/markdown bundle files to a directory")
	fs.Var(&paths, "path", "restrict analysis to a repo-relative directory or exact manifest path (repeatable)")
	fs.BoolVar(&opts.ListTargets, "list-targets", defaults.ListTargets, "print detected dependency analysis targets and exit")
	fs.Usage = func() { printPRUsage(stderr) }

	parseArgs := normalizePRArgs(args)
	if err := fs.Parse(parseArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "expected at most one PR argument")
		fs.Usage()
		return 1
	}
	if fs.NArg() == 1 {
		opts.PRArg = fs.Arg(0)
	}

	visited := visitedFlags(fs)
	if mergeErr := applyPRConfig(&opts, &failLevel, &paths, visited, defaults); mergeErr != nil {
		fmt.Fprintln(stderr, mergeErr)
		return 1
	}

	level, err := analysis.ParseRiskLevel(failLevel)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	opts.FailLevel = level
	if opts.Lang != "ko" && opts.Lang != "en" {
		fmt.Fprintf(stderr, "unsupported lang %q\n", opts.Lang)
		return 1
	}
	opts.Format = strings.ToLower(opts.Format)
	switch opts.Format {
	case "human", "json", "markdown":
	default:
		fmt.Fprintf(stderr, "unsupported format %q\n", opts.Format)
		return 1
	}

	runErr := app.RunPR(context.Background(), app.RunPRDependencies{
		GitHub:   ghclient.NewClient(),
		Registry: npm.NewRegistryClient(),
		Stdout:   stdout,
		Stderr:   stderr,
	}, opts)
	if runErr == nil {
		return 0
	}

	exitErr, ok := runErr.(*app.ExitError)
	if !ok {
		fmt.Fprintln(stderr, runErr)
		return 1
	}
	if exitErr.Err != nil {
		fmt.Fprintln(stderr, exitErr.Err)
	}
	return exitErr.Code
}

func normalizePRArgs(args []string) []string {
	reordered := make([]string, 0, len(args))
	var prArg string

	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			reordered = append(reordered, args[i:]...)
			break
		}

		if strings.HasPrefix(token, "-") {
			reordered = append(reordered, token)
			if flagConsumesValue(token) && !strings.Contains(token, "=") && i+1 < len(args) {
				i++
				reordered = append(reordered, args[i])
			}
			continue
		}

		if prArg == "" {
			prArg = token
			continue
		}
		reordered = append(reordered, token)
	}

	if prArg != "" {
		reordered = append(reordered, prArg)
	}
	return reordered
}

func flagConsumesValue(token string) bool {
	name := strings.TrimLeft(token, "-")
	if index := strings.Index(name, "="); index >= 0 {
		name = name[:index]
	}
	switch name {
	case "repo", "format", "lang", "fail-level", "bundle-dir", "path":
		return true
	default:
		return false
	}
}

func printPRUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gh dep-risk pr [<number>|<url>] [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gh dep-risk pr 123")
	fmt.Fprintln(w, "  gh dep-risk pr https://github.com/OWNER/REPO/pull/123")
	fmt.Fprintln(w, "  gh dep-risk pr --format json")
	fmt.Fprintln(w, "  gh dep-risk pr 123 --list-targets")
	fmt.Fprintln(w, "  gh dep-risk pr 123 --path apps/web")
	fmt.Fprintln(w, "  gh dep-risk pr --comment=false")
	fmt.Fprintln(w, "  gh dep-risk pr --no-registry=false")
	fmt.Fprintln(w, "  gh dep-risk pr --bundle-dir ./dep-risk-bundle")
	fmt.Fprintln(w, "  gh dep-risk pr --comment")
	fmt.Fprintln(w, "  gh dep-risk pr --fail-level high")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  -repo string")
	fmt.Fprintln(w, "    \trepository in OWNER/REPO form")
	fmt.Fprintln(w, "  -format string")
	fmt.Fprintln(w, "    \toutput format: human|json|markdown (default \"human\")")
	fmt.Fprintln(w, "  -lang string")
	fmt.Fprintln(w, "    \toutput language: ko|en (default \"en\")")
	fmt.Fprintln(w, "  -comment")
	fmt.Fprintln(w, "    \tupsert a PR timeline comment")
	fmt.Fprintln(w, "  -fail-level string")
	fmt.Fprintln(w, "    \tfail threshold: low|medium|high|critical|none (default \"none\")")
	fmt.Fprintln(w, "  -no-registry")
	fmt.Fprintln(w, "    \tskip npm-compatible registry publish-age lookups")
	fmt.Fprintln(w, "  -bundle-dir string")
	fmt.Fprintln(w, "    \twrite human/json/markdown bundle files to a directory")
	fmt.Fprintln(w, "  -path value")
	fmt.Fprintln(w, "    \trestrict analysis to a repo-relative directory or exact manifest path (repeatable)")
	fmt.Fprintln(w, "  -list-targets")
	fmt.Fprintln(w, "    \tprint detected dependency analysis targets and exit")
	fmt.Fprintf(w, "\nConfig:\n  Reads %s from the current working directory when present. CLI flags override config values.\n", config.PRConfigFileName)
}

func defaultPROptions() app.RunPROptions {
	return app.RunPROptions{
		Format:    "human",
		Lang:      "en",
		FailLevel: analysis.RiskLevelNone,
	}
}

func visitedFlags(fs *flag.FlagSet) map[string]struct{} {
	visited := map[string]struct{}{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = struct{}{}
	})
	return visited
}

func applyPRConfig(opts *app.RunPROptions, failLevel *string, paths *multiStringFlag, visited map[string]struct{}, defaults app.RunPROptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, _, err := config.LoadPRConfig(cwd)
	if err != nil {
		return err
	}

	if _, ok := visited["lang"]; !ok && cfg.Lang != nil {
		opts.Lang = *cfg.Lang
	}
	if _, ok := visited["comment"]; !ok && cfg.Comment != nil {
		opts.Comment = *cfg.Comment
	}
	if _, ok := visited["fail-level"]; !ok && cfg.FailLevel != nil {
		*failLevel = *cfg.FailLevel
	}
	if _, ok := visited["no-registry"]; !ok && cfg.NoRegistry != nil {
		opts.NoRegistry = *cfg.NoRegistry
	}
	if _, ok := visited["path"]; ok {
		opts.Paths = append([]string(nil), (*paths)...)
	} else if cfg.Paths.Set {
		opts.Paths = append([]string(nil), cfg.Paths.Values...)
	} else {
		opts.Paths = append([]string(nil), defaults.Paths...)
	}
	return nil
}
