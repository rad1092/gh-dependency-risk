package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizePRArgsAllowsFlagsAfterPRNumber(t *testing.T) {
	args := []string{"123", "--comment", "--fail-level", "medium"}
	got := normalizePRArgs(args)
	want := []string{"--comment", "--fail-level", "medium", "123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized args: got %#v want %#v", got, want)
	}
}

func TestNormalizePRArgsAllowsRepoFlagBeforePRNumber(t *testing.T) {
	args := []string{"--repo", "owner/repo", "123", "--bundle-dir", "out"}
	got := normalizePRArgs(args)
	want := []string{"--repo", "owner/repo", "--bundle-dir", "out", "123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized args: got %#v want %#v", got, want)
	}
}

func TestFlagConsumesValue(t *testing.T) {
	valueFlags := []string{"--repo", "--format=json", "--lang", "--fail-level", "--bundle-dir", "--path"}
	for _, token := range valueFlags {
		if !flagConsumesValue(token) {
			t.Fatalf("expected %s to consume a value", token)
		}
	}
	booleanFlags := []string{"--comment", "--list-targets", "--no-registry"}
	for _, token := range booleanFlags {
		if flagConsumesValue(token) {
			t.Fatalf("expected %s to be treated as a boolean flag", token)
		}
	}
}

func TestNormalizePRArgsHandlesEveryValueConsumingFlagAfterPRNumber(t *testing.T) {
	args := []string{
		"123",
		"--repo", "owner/repo",
		"--format", "json",
		"--lang", "ko",
		"--fail-level", "high",
		"--bundle-dir", "out",
		"--path", "apps/web",
		"--path", "package.json",
		"--comment",
	}
	got := normalizePRArgs(args)
	want := []string{
		"--repo", "owner/repo",
		"--format", "json",
		"--lang", "ko",
		"--fail-level", "high",
		"--bundle-dir", "out",
		"--path", "apps/web",
		"--path", "package.json",
		"--comment",
		"123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized args: got %#v want %#v", got, want)
	}
}

func TestPrintPRUsageShowsEnglishDefaultLanguage(t *testing.T) {
	var output bytes.Buffer
	printPRUsage(&output)
	if !strings.Contains(output.String(), `output language: ko|en (default "en")`) {
		t.Fatalf("expected help output to mention english default, got %q", output.String())
	}
	if !strings.Contains(output.String(), "skip npm-compatible registry publish-age lookups") {
		t.Fatalf("expected help output to scope registry lookups, got %q", output.String())
	}
}
