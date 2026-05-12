package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rad1092/gh-dependency-risk/internal/analysis"
	"github.com/rad1092/gh-dependency-risk/internal/app"
	"github.com/rad1092/gh-dependency-risk/internal/config"
)

func TestApplyPRConfigUsesConfigWhenFlagsAbsent(t *testing.T) {
	dir := t.TempDir()
	writePRConfig(t, dir, "lang: ko\nfail_level: high\ncomment: true\npath:\n  - apps/web\nno_registry: true\n")
	withWorkingDirectory(t, dir, func() {
		opts := defaultPROptions()
		failLevel := string(opts.FailLevel)
		var paths multiStringFlag

		if err := applyPRConfig(&opts, &failLevel, &paths, map[string]struct{}{}, defaultPROptions()); err != nil {
			t.Fatal(err)
		}

		if opts.Lang != "ko" || !opts.Comment || !opts.NoRegistry {
			t.Fatalf("unexpected merged options %#v", opts)
		}
		if failLevel != "high" {
			t.Fatalf("unexpected fail level %q", failLevel)
		}
		if !reflect.DeepEqual(opts.Paths, []string{"apps/web"}) {
			t.Fatalf("unexpected paths %#v", opts.Paths)
		}
	})
}

func TestApplyPRConfigPrefersExplicitCLIFlags(t *testing.T) {
	dir := t.TempDir()
	writePRConfig(t, dir, "lang: ko\ncomment: true\npath:\n  - apps/web\n")
	withWorkingDirectory(t, dir, func() {
		opts := defaultPROptions()
		opts.Lang = "en"
		opts.Comment = false
		failLevel := string(opts.FailLevel)
		paths := multiStringFlag{"services/api"}
		visited := map[string]struct{}{
			"lang":    {},
			"comment": {},
			"path":    {},
		}

		if err := applyPRConfig(&opts, &failLevel, &paths, visited, defaultPROptions()); err != nil {
			t.Fatal(err)
		}

		if opts.Lang != "en" || opts.Comment {
			t.Fatalf("expected CLI values to win, got %#v", opts)
		}
		if !reflect.DeepEqual(opts.Paths, []string{"services/api"}) {
			t.Fatalf("expected CLI paths to replace config paths, got %#v", opts.Paths)
		}
	})
}

func TestApplyPRConfigAllowsExplicitBooleanFalseOverride(t *testing.T) {
	dir := t.TempDir()
	writePRConfig(t, dir, "comment: true\nno_registry: true\n")
	withWorkingDirectory(t, dir, func() {
		opts := app.RunPROptions{
			Format:     "human",
			Lang:       "en",
			Comment:    false,
			NoRegistry: false,
			FailLevel:  analysis.RiskLevelNone,
		}
		failLevel := string(opts.FailLevel)
		var paths multiStringFlag
		visited := map[string]struct{}{
			"comment":     {},
			"no-registry": {},
		}

		if err := applyPRConfig(&opts, &failLevel, &paths, visited, defaultPROptions()); err != nil {
			t.Fatal(err)
		}

		if opts.Comment || opts.NoRegistry {
			t.Fatalf("expected explicit false flags to override config, got %#v", opts)
		}
	})
}

func TestApplyPRConfigReturnsConfigErrors(t *testing.T) {
	dir := t.TempDir()
	writePRConfig(t, dir, "unknown_key: true\n")
	withWorkingDirectory(t, dir, func() {
		opts := defaultPROptions()
		failLevel := string(opts.FailLevel)
		var paths multiStringFlag

		err := applyPRConfig(&opts, &failLevel, &paths, map[string]struct{}{}, defaultPROptions())
		if err == nil {
			t.Fatalf("expected config error")
		}
		if !strings.Contains(err.Error(), filepath.Join(dir, config.PRConfigFileName)) {
			t.Fatalf("expected config filename in error, got %v", err)
		}
	})
}

func writePRConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, config.PRConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	})
	fn()
}
