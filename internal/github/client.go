package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	gh "github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	ghrepo "github.com/cli/go-gh/v2/pkg/repository"
)

const (
	MarkerComment = "<!-- gh-dep-risk -->"
	apiVersion    = "2026-03-10"
)

var (
	ghExecContext    = gh.ExecContext
	currentGitBranch = resolveCurrentGitBranch
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	Host  string
	Owner string
	Name  string
}

func (r Repo) FullName() string {
	if strings.EqualFold(r.Host, "github.com") || r.Host == "" {
		return fmt.Sprintf("%s/%s", r.Owner, r.Name)
	}
	return fmt.Sprintf("%s/%s/%s", r.Host, r.Owner, r.Name)
}

type PullRequest struct {
	Title       string
	Draft       bool
	Number      int
	BaseSHA     string
	HeadSHA     string
	URL         string
	AuthorLogin string
}

type PullRequestFile struct {
	Filename string
	Status   string
}

type Vulnerability struct {
	Severity string
	GHSAID   string
	Summary  string
	URL      string
}

type DependencyReviewChange struct {
	ChangeType      string
	Manifest        string
	Ecosystem       string
	Name            string
	Version         string
	Vulnerabilities []Vulnerability
}

type IssueComment struct {
	ID        int64
	Body      string
	UserLogin string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Client interface {
	ResolveRepo(ctx context.Context, override string) (Repo, error)
	ViewerLogin(ctx context.Context, repo Repo) (string, error)
	ResolveCurrentPR(ctx context.Context, repo Repo) (int, error)
	GetPullRequest(ctx context.Context, repo Repo, number int) (PullRequest, error)
	ListPullRequestFiles(ctx context.Context, repo Repo, number int) ([]PullRequestFile, error)
	CompareDependencies(ctx context.Context, repo Repo, baseSHA, headSHA string) ([]DependencyReviewChange, error)
	CompareDependenciesForManifest(ctx context.Context, repo Repo, baseSHA, headSHA, manifestPath string) ([]DependencyReviewChange, error)
	ListRepositoryFiles(ctx context.Context, repo Repo, ref string) ([]string, error)
	GetRepositoryFile(ctx context.Context, repo Repo, path, ref string) ([]byte, error)
	ListIssueComments(ctx context.Context, repo Repo, issueNumber int) ([]IssueComment, error)
	CreateIssueComment(ctx context.Context, repo Repo, issueNumber int, body string) (IssueComment, error)
	UpdateIssueComment(ctx context.Context, repo Repo, commentID int64, body string) error
	DeleteIssueComment(ctx context.Context, repo Repo, commentID int64) error
}

type APIClient struct{}

type gitTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type gitTreeResponse struct {
	Tree      []gitTreeEntry `json:"tree"`
	Truncated bool           `json:"truncated"`
}

type gitTreeFetcher func(ctx context.Context, ref string, recursive bool) (gitTreeResponse, error)

func NewClient() *APIClient {
	return &APIClient{}
}

func (c *APIClient) ResolveRepo(_ context.Context, override string) (Repo, error) {
	var repo ghrepo.Repository
	var err error
	if override != "" {
		repo, err = ghrepo.Parse(override)
	} else {
		repo, err = ghrepo.Current()
	}
	if err != nil {
		return Repo{}, err
	}
	return Repo{Host: repo.Host, Owner: repo.Owner, Name: repo.Name}, nil
}

func (c *APIClient) ViewerLogin(ctx context.Context, repo Repo) (string, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return "", classifyAuthError(err)
	}

	var resp struct {
		Login string `json:"login"`
	}
	if err := client.DoWithContext(ctx, "GET", "user", nil, &resp); err != nil {
		if login, ok := actionsIntegrationViewerLogin(err); ok {
			return login, nil
		}
		return "", classifyAuthError(err)
	}
	if resp.Login == "" {
		return "", AuthError{Op: "resolve authenticated viewer"}
	}
	return resp.Login, nil
}

func actionsIntegrationViewerLogin(err error) (string, bool) {
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return "", false
	}
	if httpErr.StatusCode != 403 {
		return "", false
	}
	if !strings.Contains(strings.ToLower(httpErr.Message), "resource not accessible by integration") {
		return "", false
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return "", false
	}
	return "github-actions[bot]", true
}

func (c *APIClient) ResolveCurrentPR(ctx context.Context, repo Repo) (int, error) {
	branch, err := currentGitBranch(ctx)
	if err != nil {
		if isAuthMessage(err.Error()) {
			return 0, AuthError{Op: "resolve current PR", Err: err}
		}
		return 0, fmt.Errorf("resolve current PR: %w", err)
	}

	stdout, stderr, err := ghExecContext(ctx, "pr", "view", branch, "--json", "number", "--repo", repo.FullName())
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		if isAuthMessage(message) {
			return 0, AuthError{Op: "resolve current PR", Err: fmt.Errorf("%s", message)}
		}
		return 0, fmt.Errorf("resolve current PR: %s", message)
	}

	var resp struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return 0, fmt.Errorf("decode current PR response: %w", err)
	}
	if resp.Number == 0 {
		return 0, errors.New("unable to resolve current PR for the current branch")
	}
	return resp.Number, nil
}

func resolveCurrentGitBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("determine current branch: %s", message)
	}

	branch := strings.TrimSpace(stdout.String())
	if branch == "" {
		return "", errors.New("determine current branch: empty branch name")
	}
	return branch, nil
}

func (c *APIClient) GetPullRequest(ctx context.Context, repo Repo, number int) (PullRequest, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return PullRequest{}, classifyAuthError(err)
	}

	var resp struct {
		Title   string `json:"title"`
		Draft   bool   `json:"draft"`
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Base    struct {
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", repo.Owner, repo.Name, number)
	if err := client.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return PullRequest{}, classifyAuthError(err)
	}

	return PullRequest{
		Title:       resp.Title,
		Draft:       resp.Draft,
		Number:      resp.Number,
		BaseSHA:     resp.Base.SHA,
		HeadSHA:     resp.Head.SHA,
		URL:         resp.HTMLURL,
		AuthorLogin: resp.User.Login,
	}, nil
}

func (c *APIClient) ListPullRequestFiles(ctx context.Context, repo Repo, number int) ([]PullRequestFile, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return nil, classifyAuthError(err)
	}

	files := []PullRequestFile{}
	page := 1
	for {
		var batch []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
		}
		path := fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100&page=%d", repo.Owner, repo.Name, number, page)
		if err := client.DoWithContext(ctx, "GET", path, nil, &batch); err != nil {
			return nil, classifyAuthError(err)
		}
		for _, file := range batch {
			files = append(files, PullRequestFile{Filename: file.Filename, Status: file.Status})
		}
		if len(batch) < 100 {
			break
		}
		page++
	}
	return files, nil
}

func (c *APIClient) CompareDependencies(ctx context.Context, repo Repo, baseSHA, headSHA string) ([]DependencyReviewChange, error) {
	return c.compareDependencies(ctx, repo, baseSHA, headSHA, "")
}

func (c *APIClient) CompareDependenciesForManifest(ctx context.Context, repo Repo, baseSHA, headSHA, manifestPath string) ([]DependencyReviewChange, error) {
	return c.compareDependencies(ctx, repo, baseSHA, headSHA, manifestPath)
}

func (c *APIClient) compareDependencies(ctx context.Context, repo Repo, baseSHA, headSHA, manifestPath string) ([]DependencyReviewChange, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return nil, classifyAuthError(err)
	}

	var resp []struct {
		ChangeType      string `json:"change_type"`
		Manifest        string `json:"manifest"`
		Ecosystem       string `json:"ecosystem"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		Vulnerabilities []struct {
			Severity string `json:"severity"`
			GHSAID   string `json:"advisory_ghsa_id"`
			Summary  string `json:"advisory_summary"`
			URL      string `json:"advisory_url"`
		} `json:"vulnerabilities"`
	}

	baseHead := url.PathEscape(fmt.Sprintf("%s...%s", baseSHA, headSHA))
	path := fmt.Sprintf("repos/%s/%s/dependency-graph/compare/%s", repo.Owner, repo.Name, baseHead)
	if strings.TrimSpace(manifestPath) != "" {
		path += "?name=" + url.QueryEscape(manifestPath)
	}
	if err := client.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, classifyAuthError(err)
	}

	changes := make([]DependencyReviewChange, 0, len(resp))
	for _, item := range resp {
		vulns := make([]Vulnerability, 0, len(item.Vulnerabilities))
		for _, vuln := range item.Vulnerabilities {
			vulns = append(vulns, Vulnerability{
				Severity: vuln.Severity,
				GHSAID:   vuln.GHSAID,
				Summary:  vuln.Summary,
				URL:      vuln.URL,
			})
		}
		changes = append(changes, DependencyReviewChange{
			ChangeType:      item.ChangeType,
			Manifest:        item.Manifest,
			Ecosystem:       item.Ecosystem,
			Name:            item.Name,
			Version:         item.Version,
			Vulnerabilities: vulns,
		})
	}
	return changes, nil
}

func (c *APIClient) ListRepositoryFiles(ctx context.Context, repo Repo, ref string) ([]string, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return nil, classifyAuthError(err)
	}

	files, err := listRepositoryFilesFromTree(ctx, ref, func(ctx context.Context, treeRef string, recursive bool) (gitTreeResponse, error) {
		resp, err := c.fetchGitTree(ctx, client, repo, treeRef, recursive)
		return resp, classifyAuthError(err)
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (c *APIClient) GetRepositoryFile(ctx context.Context, repo Repo, path, ref string) ([]byte, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return nil, classifyAuthError(err)
	}

	var resp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}

	contentPath := fmt.Sprintf("repos/%s/%s/contents/%s?ref=%s", repo.Owner, repo.Name, escapeContentPath(path), url.QueryEscape(ref))
	if err := client.DoWithContext(ctx, "GET", contentPath, nil, &resp); err != nil {
		if IsHTTPStatus(err, 404) {
			return nil, ErrNotFound
		}
		return nil, classifyAuthError(err)
	}
	return decodeRepositoryContent(path, resp.Content, resp.Encoding, resp.SHA, func(sha string) (string, string, error) {
		var blob struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		blobPath := fmt.Sprintf("repos/%s/%s/git/blobs/%s", repo.Owner, repo.Name, url.PathEscape(sha))
		if err := client.DoWithContext(ctx, "GET", blobPath, nil, &blob); err != nil {
			if IsHTTPStatus(err, 404) {
				return "", "", ErrNotFound
			}
			return "", "", classifyAuthError(err)
		}
		return blob.Content, blob.Encoding, nil
	})
}

func (c *APIClient) ListIssueComments(ctx context.Context, repo Repo, issueNumber int) ([]IssueComment, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return nil, classifyAuthError(err)
	}

	comments := []IssueComment{}
	page := 1
	for {
		var batch []struct {
			ID        int64     `json:"id"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		path := fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100&page=%d", repo.Owner, repo.Name, issueNumber, page)
		if err := client.DoWithContext(ctx, "GET", path, nil, &batch); err != nil {
			return nil, classifyAuthError(err)
		}
		for _, item := range batch {
			comments = append(comments, IssueComment{
				ID:        item.ID,
				Body:      item.Body,
				UserLogin: item.User.Login,
				CreatedAt: item.CreatedAt,
				UpdatedAt: item.UpdatedAt,
			})
		}
		if len(batch) < 100 {
			break
		}
		page++
	}
	return comments, nil
}

func (c *APIClient) CreateIssueComment(ctx context.Context, repo Repo, issueNumber int, body string) (IssueComment, error) {
	client, err := c.restClient(repo)
	if err != nil {
		return IssueComment{}, classifyAuthError(err)
	}

	payload, _ := json.Marshal(map[string]string{"body": body})
	var resp struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", repo.Owner, repo.Name, issueNumber)
	if err := client.DoWithContext(ctx, "POST", path, bytes.NewReader(payload), &resp); err != nil {
		return IssueComment{}, classifyAuthError(err)
	}
	return IssueComment{
		ID:        resp.ID,
		Body:      resp.Body,
		UserLogin: resp.User.Login,
		CreatedAt: resp.CreatedAt,
		UpdatedAt: resp.UpdatedAt,
	}, nil
}

func (c *APIClient) UpdateIssueComment(ctx context.Context, repo Repo, commentID int64, body string) error {
	client, err := c.restClient(repo)
	if err != nil {
		return classifyAuthError(err)
	}
	payload, _ := json.Marshal(map[string]string{"body": body})
	path := fmt.Sprintf("repos/%s/%s/issues/comments/%d", repo.Owner, repo.Name, commentID)
	return classifyAuthError(client.DoWithContext(ctx, "PATCH", path, bytes.NewReader(payload), &struct{}{}))
}

func (c *APIClient) DeleteIssueComment(ctx context.Context, repo Repo, commentID int64) error {
	client, err := c.restClient(repo)
	if err != nil {
		return classifyAuthError(err)
	}
	path := fmt.Sprintf("repos/%s/%s/issues/comments/%d", repo.Owner, repo.Name, commentID)
	return classifyAuthError(client.DoWithContext(ctx, "DELETE", path, nil, &struct{}{}))
}

func (c *APIClient) restClient(repo Repo) (*api.RESTClient, error) {
	return api.NewRESTClient(api.ClientOptions{
		Host: repo.Host,
		Headers: map[string]string{
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": apiVersion,
		},
	})
}

func (c *APIClient) fetchGitTree(ctx context.Context, client *api.RESTClient, repo Repo, ref string, recursive bool) (gitTreeResponse, error) {
	var resp gitTreeResponse
	treePath := fmt.Sprintf("repos/%s/%s/git/trees/%s", repo.Owner, repo.Name, url.PathEscape(ref))
	if recursive {
		treePath += "?recursive=1"
	}
	if err := client.DoWithContext(ctx, "GET", treePath, nil, &resp); err != nil {
		return gitTreeResponse{}, err
	}
	return resp, nil
}

func listRepositoryFilesFromTree(ctx context.Context, ref string, fetch gitTreeFetcher) ([]string, error) {
	recursiveTree, err := fetch(ctx, ref, true)
	if err != nil {
		return nil, err
	}
	if !recursiveTree.Truncated {
		return sortedUniqueBlobPaths(recursiveTree.Tree), nil
	}

	rootTree, err := fetch(ctx, ref, false)
	if err != nil {
		return nil, err
	}
	if rootTree.Truncated {
		return nil, fmt.Errorf("repository tree %q is truncated even without recursion", ref)
	}

	files := make([]string, 0, len(rootTree.Tree))
	queue := make([]gitTreeEntry, 0, len(rootTree.Tree))
	for _, entry := range rootTree.Tree {
		switch entry.Type {
		case "blob":
			files = appendJoinedBlobPath(files, "", entry.Path)
		case "tree":
			queue = append(queue, entry)
		}
	}

	for len(queue) > 0 {
		entry := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		if strings.TrimSpace(entry.SHA) == "" {
			return nil, fmt.Errorf("repository subtree %q is missing a tree SHA", entry.Path)
		}

		subtree, err := fetch(ctx, entry.SHA, false)
		if err != nil {
			return nil, err
		}
		if subtree.Truncated {
			return nil, fmt.Errorf("repository subtree %q (%s) is truncated even without recursion", entry.Path, entry.SHA)
		}

		for _, child := range subtree.Tree {
			joinedPath := joinRepoPath(entry.Path, child.Path)
			switch child.Type {
			case "blob":
				files = appendJoinedBlobPath(files, entry.Path, child.Path)
			case "tree":
				queue = append(queue, gitTreeEntry{Path: joinedPath, Type: child.Type, SHA: child.SHA})
			}
		}
	}

	return sortedUniquePaths(files), nil
}

func appendJoinedBlobPath(files []string, prefix, entryPath string) []string {
	joinedPath := joinRepoPath(prefix, entryPath)
	if joinedPath == "" {
		return files
	}
	return append(files, joinedPath)
}

func joinRepoPath(prefix, entryPath string) string {
	trimmedEntry := strings.TrimSpace(entryPath)
	if trimmedEntry == "" {
		return ""
	}
	if strings.TrimSpace(prefix) == "" {
		return path.Clean(trimmedEntry)
	}
	return path.Clean(path.Join(prefix, trimmedEntry))
}

func sortedUniqueBlobPaths(entries []gitTreeEntry) []string {
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		files = appendJoinedBlobPath(files, "", entry.Path)
	}
	return sortedUniquePaths(files)
}

func sortedUniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, filePath := range paths {
		if strings.TrimSpace(filePath) == "" {
			continue
		}
		if _, ok := seen[filePath]; ok {
			continue
		}
		seen[filePath] = struct{}{}
		unique = append(unique, filePath)
	}
	sort.Strings(unique)
	return unique
}

func escapeContentPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func decodeRepositoryContent(path, content, encoding, sha string, fetchBlob func(sha string) (string, string, error)) ([]byte, error) {
	switch encoding {
	case "base64":
		return decodeBase64RepositoryContent(path, content)
	case "none":
		if strings.TrimSpace(sha) == "" {
			return nil, fmt.Errorf("unsupported content encoding %q for %s", encoding, path)
		}
		if fetchBlob == nil {
			return nil, fmt.Errorf("unsupported content encoding %q for %s", encoding, path)
		}
		blobContent, blobEncoding, err := fetchBlob(sha)
		if err != nil {
			return nil, err
		}
		if blobEncoding != "base64" {
			return nil, fmt.Errorf("unsupported blob encoding %q for %s", blobEncoding, path)
		}
		return decodeBase64RepositoryContent(path, blobContent)
	default:
		return nil, fmt.Errorf("unsupported content encoding %q for %s", encoding, path)
	}
}

func decodeBase64RepositoryContent(path, content string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return decoded, nil
}

type AuthError struct {
	Op  string
	Err error
}

func (e AuthError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s requires GitHub authentication or additional permissions", e.Op)
	}
	return fmt.Sprintf("%s requires GitHub authentication or additional permissions: %v", e.Op, e.Err)
}

func (e AuthError) Unwrap() error {
	return e.Err
}

func classifyAuthError(err error) error {
	if err == nil {
		return nil
	}
	if IsHTTPStatus(err, 401) || IsHTTPStatus(err, 403) || isAuthMessage(err.Error()) {
		return AuthError{Op: "GitHub request", Err: err}
	}
	return err
}

func IsAuthError(err error) bool {
	var authErr AuthError
	return errors.As(err, &authErr)
}

func IsHTTPStatus(err error, status int) bool {
	var httpErr *api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == status
}

func IsPermissionError(err error) bool {
	return IsHTTPStatus(err, 401) || IsHTTPStatus(err, 403) || IsAuthError(err)
}

func IsDependencyReviewUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return IsHTTPStatus(err, 403) || IsHTTPStatus(err, 404)
}

func UpsertMarkerComment(ctx context.Context, client Client, repo Repo, issueNumber int, viewerLogin string, body string, stderr io.Writer) error {
	comments, err := client.ListIssueComments(ctx, repo, issueNumber)
	if err != nil {
		return err
	}

	own := make([]IssueComment, 0)
	foreign := make([]IssueComment, 0)
	for _, comment := range comments {
		if !strings.Contains(comment.Body, MarkerComment) {
			continue
		}
		if comment.UserLogin == viewerLogin {
			own = append(own, comment)
		} else {
			foreign = append(foreign, comment)
		}
	}

	if len(foreign) > 0 && stderr != nil {
		authors := map[string]struct{}{}
		for _, comment := range foreign {
			authors[comment.UserLogin] = struct{}{}
		}
		names := make([]string, 0, len(authors))
		for login := range authors {
			names = append(names, login)
		}
		sort.Strings(names)
		fmt.Fprintf(stderr, "warning: found marker comment owned by %s; only managing comments by %s\n", strings.Join(names, ", "), viewerLogin)
	}

	sort.SliceStable(own, func(i, j int) bool {
		left := own[i].CreatedAt
		if left.IsZero() {
			left = own[i].UpdatedAt
		}
		right := own[j].CreatedAt
		if right.IsZero() {
			right = own[j].UpdatedAt
		}
		if left.Equal(right) {
			return own[i].ID > own[j].ID
		}
		return left.After(right)
	})

	if len(own) == 0 {
		_, err := client.CreateIssueComment(ctx, repo, issueNumber, body)
		return err
	}

	if err := client.UpdateIssueComment(ctx, repo, own[0].ID, body); err != nil {
		return err
	}
	for _, duplicate := range own[1:] {
		if err := client.DeleteIssueComment(ctx, repo, duplicate.ID); err != nil {
			return err
		}
	}
	return nil
}

func isAuthMessage(message string) bool {
	lower := strings.ToLower(message)
	for _, candidate := range []string{
		"authentication token not found",
		"gh auth login",
		"not logged into any github hosts",
		"authentication required",
		"requires authentication",
		"http 401",
		"http 403",
		"insufficient permissions",
		"resource not accessible",
	} {
		if strings.Contains(lower, candidate) {
			return true
		}
	}
	return false
}
