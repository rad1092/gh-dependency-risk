package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestResolveCurrentPRUsesCurrentBranch(t *testing.T) {
	originalExec := ghExecContext
	originalBranch := currentGitBranch
	t.Cleanup(func() {
		ghExecContext = originalExec
		currentGitBranch = originalBranch
	})

	currentGitBranch = func(context.Context) (string, error) {
		return "feature/test-branch", nil
	}

	var gotArgs []string
	ghExecContext = func(_ context.Context, args ...string) (bytes.Buffer, bytes.Buffer, error) {
		gotArgs = append([]string{}, args...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		stdout.WriteString(`{"number": 17}`)
		return stdout, stderr, nil
	}

	client := NewClient()
	number, err := client.ResolveCurrentPR(context.Background(), Repo{Owner: "owner", Name: "repo"})
	if err != nil {
		t.Fatalf("ResolveCurrentPR returned error: %v", err)
	}
	if number != 17 {
		t.Fatalf("expected PR number 17, got %d", number)
	}

	expected := []string{"pr", "view", "feature/test-branch", "--json", "number", "--repo", "owner/repo"}
	if len(gotArgs) != len(expected) {
		t.Fatalf("unexpected gh args: %#v", gotArgs)
	}
	for i, want := range expected {
		if gotArgs[i] != want {
			t.Fatalf("unexpected gh arg at %d: got %q want %q", i, gotArgs[i], want)
		}
	}
}

func TestResolveCurrentPRBranchError(t *testing.T) {
	originalExec := ghExecContext
	originalBranch := currentGitBranch
	t.Cleanup(func() {
		ghExecContext = originalExec
		currentGitBranch = originalBranch
	})

	currentGitBranch = func(context.Context) (string, error) {
		return "", errors.New("determine current branch: empty branch name")
	}

	client := NewClient()
	_, err := client.ResolveCurrentPR(context.Background(), Repo{Owner: "owner", Name: "repo"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "resolve current PR: determine current branch: empty branch name" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRepositoryFilesFromTreeFallsBackWhenRecursiveTreeTruncated(t *testing.T) {
	calls := make([]string, 0)
	responses := map[string]gitTreeResponse{
		"head|true": {
			Truncated: true,
			Tree: []gitTreeEntry{
				{Path: "README.md", Type: "blob"},
				{Path: "apps", Type: "tree", SHA: "sha-apps"},
			},
		},
		"head|false": {
			Tree: []gitTreeEntry{
				{Path: "README.md", Type: "blob"},
				{Path: "apps", Type: "tree", SHA: "sha-apps"},
				{Path: "services", Type: "tree", SHA: "sha-services"},
			},
		},
		"sha-services|false": {
			Tree: []gitTreeEntry{
				{Path: "api", Type: "tree", SHA: "sha-api"},
			},
		},
		"sha-api|false": {
			Tree: []gitTreeEntry{
				{Path: "package-lock.json", Type: "blob"},
				{Path: "package.json", Type: "blob"},
				{Path: "package.json", Type: "blob"},
			},
		},
		"sha-apps|false": {
			Tree: []gitTreeEntry{
				{Path: "ui", Type: "tree", SHA: "sha-ui"},
				{Path: "web", Type: "tree", SHA: "sha-web"},
			},
		},
		"sha-ui|false": {
			Tree: []gitTreeEntry{
				{Path: "package.json", Type: "blob"},
			},
		},
		"sha-web|false": {
			Tree: []gitTreeEntry{
				{Path: "package.json", Type: "blob"},
				{Path: "src", Type: "tree", SHA: "sha-web-src"},
			},
		},
		"sha-web-src|false": {
			Tree: []gitTreeEntry{
				{Path: "index.ts", Type: "blob"},
			},
		},
	}

	files, err := listRepositoryFilesFromTree(context.Background(), "head", func(_ context.Context, ref string, recursive bool) (gitTreeResponse, error) {
		key := ref + "|" + strconvFormatBool(recursive)
		calls = append(calls, key)
		resp, ok := responses[key]
		if !ok {
			return gitTreeResponse{}, errors.New("unexpected tree fetch: " + key)
		}
		return resp, nil
	})
	if err != nil {
		t.Fatalf("listRepositoryFilesFromTree returned error: %v", err)
	}

	want := []string{
		"README.md",
		"apps/ui/package.json",
		"apps/web/package.json",
		"apps/web/src/index.ts",
		"services/api/package-lock.json",
		"services/api/package.json",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected files: got %#v want %#v", files, want)
	}
	if len(calls) < 2 || calls[0] != "head|true" || calls[1] != "head|false" {
		t.Fatalf("expected recursive fetch followed by non-recursive fallback, got %#v", calls)
	}
}

func TestListRepositoryFilesFromTreeFailsWhenSubtreeRemainsTruncated(t *testing.T) {
	_, err := listRepositoryFilesFromTree(context.Background(), "head", func(_ context.Context, ref string, recursive bool) (gitTreeResponse, error) {
		switch ref + "|" + strconvFormatBool(recursive) {
		case "head|true":
			return gitTreeResponse{Truncated: true}, nil
		case "head|false":
			return gitTreeResponse{
				Tree: []gitTreeEntry{{Path: "apps", Type: "tree", SHA: "sha-apps"}},
			}, nil
		case "sha-apps|false":
			return gitTreeResponse{Truncated: true}, nil
		default:
			return gitTreeResponse{}, errors.New("unexpected tree fetch")
		}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "truncated even without recursion") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyAuthErrorWrapsHTTP401AsAuthError(t *testing.T) {
	err := classifyAuthError(&api.HTTPError{StatusCode: 401, Message: "bad credentials"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsAuthError(err) {
		t.Fatalf("expected AuthError, got %T", err)
	}
}

func TestActionsIntegrationViewerLoginFallsBackToActionsBot(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")

	login, ok := actionsIntegrationViewerLogin(&api.HTTPError{
		StatusCode: 403,
		Message:    "Resource not accessible by integration",
	})
	if !ok {
		t.Fatal("expected Actions integration fallback")
	}
	if login != "github-actions[bot]" {
		t.Fatalf("unexpected login: got %q", login)
	}
}

func TestActionsIntegrationViewerLoginRequiresActionsEnvironment(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")

	_, ok := actionsIntegrationViewerLogin(&api.HTTPError{
		StatusCode: 403,
		Message:    "Resource not accessible by integration",
	})
	if ok {
		t.Fatal("expected no fallback outside GitHub Actions")
	}
}

func TestDecodeRepositoryContentSupportsLargeBlobFallback(t *testing.T) {
	want := []byte("hello from blob fallback")
	content, err := decodeRepositoryContent("yarn.lock", "", "none", "blob-sha", func(sha string) (string, string, error) {
		if sha != "blob-sha" {
			t.Fatalf("unexpected blob sha %q", sha)
		}
		return base64.StdEncoding.EncodeToString(want), "base64", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, want) {
		t.Fatalf("unexpected decoded content: got %q want %q", string(content), string(want))
	}
}

func TestDecodeRepositoryContentRejectsUnsupportedEncodingWithoutBlobSHA(t *testing.T) {
	_, err := decodeRepositoryContent("pnpm-lock.yaml", "", "none", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `unsupported content encoding "none"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func strconvFormatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
