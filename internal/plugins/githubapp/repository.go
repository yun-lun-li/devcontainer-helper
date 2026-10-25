package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const repositoryBodyLimit = 1 << 20
const repositoryPageLimit = 100
const repositoryMaxPages = 100

var githubPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var ErrRepositoryUnavailable = errors.New("repository could not be verified")

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}
type GitRemote struct {
	Name       string
	Repository string
}
type RepositoryOptions struct {
	HTTPClient     *http.Client
	APIBaseURL     string
	GHPath         string
	CommandTimeout time.Duration
}

func ParseGitHubRemote(raw string) (string, error) {
	path, ok := remotePath(raw)
	if !ok {
		return "", ErrRepositoryUnavailable
	}
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !validGitHubPart(parts[0]) || !validGitHubPart(parts[1]) || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return "", ErrRepositoryUnavailable
	}
	return parts[0] + "/" + parts[1], nil
}
func remotePath(raw string) (string, bool) {
	if strings.HasPrefix(raw, "git@github.com:") {
		return strings.TrimPrefix(raw, "git@github.com:"), true
	}
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Host != "github.com" || u.Hostname() != "github.com" || u.Port() != "" || strings.Contains(u.EscapedPath(), "%") {
		return "", false
	}
	if u.Scheme == "https" && u.User == nil {
		return strings.TrimPrefix(u.EscapedPath(), "/"), true
	}
	if u.Scheme == "ssh" && u.User != nil && u.User.String() == "git" {
		return strings.TrimPrefix(u.EscapedPath(), "/"), true
	}
	return "", false
}
func validGitHubPart(s string) bool { return s != "" && githubPart.MatchString(s) }

func ReadGitRemotes(ctx context.Context, projectDir, gitPath string) ([]GitRemote, error) {
	if gitPath == "" {
		gitPath = "git"
	}
	names, err := runGit(ctx, gitPath, projectDir, "remote")
	if err != nil {
		return nil, errors.New("could not read project Git remotes")
	}
	remotes := []GitRemote{}
	for _, name := range strings.Fields(string(names)) {
		if !validRemoteName(name) {
			continue
		}
		out, e := runGit(ctx, gitPath, projectDir, "remote", "get-url", "--", name)
		if e != nil {
			continue
		}
		repo, e := ParseGitHubRemote(strings.TrimSpace(string(out)))
		if e == nil {
			remotes = append(remotes, GitRemote{Name: name, Repository: repo})
		}
	}
	return remotes, nil
}
func runGit(ctx context.Context, path, dir string, args ...string) ([]byte, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, path, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	var output repositoryOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil || output.overflow {
		return nil, ErrRepositoryUnavailable
	}
	return output.Bytes(), nil
}
func validRemoteName(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !strings.ContainsAny(s, " \t\r\n")
}
func SelectRemote(remotes []GitRemote, name string) (GitRemote, error) {
	for _, r := range remotes {
		if r.Name == name {
			return r, nil
		}
	}
	return GitRemote{}, ErrRepositoryUnavailable
}

func LookupRepository(ctx context.Context, owner, repo string, opts RepositoryOptions) (Repository, error) {
	if !validGitHubPart(owner) || !validGitHubPart(repo) {
		return Repository{}, ErrRepositoryUnavailable
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	result, err := requestRepository(ctx, http.MethodGet, path, "", opts)
	if err == nil {
		if sameRepositoryName(result.FullName, owner+"/"+repo) {
			return result, nil
		}
		return Repository{}, ErrRepositoryUnavailable
	}
	gh, ghErr := lookupWithGH(ctx, owner, repo, opts)
	if ghErr == nil {
		return gh, nil
	}
	return Repository{}, ErrRepositoryUnavailable
}
func sameRepositoryName(left, right string) bool { return strings.EqualFold(left, right) }

func requestRepository(ctx context.Context, method, path, token string, opts RepositoryOptions) (Repository, error) {
	client, base, err := repositoryClient(opts)
	if err != nil {
		return Repository{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, nil)
	if err != nil {
		return Repository{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Repository{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Repository{}, fmt.Errorf("status")
	}
	var data struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if err := decodeBounded(resp.Body, &data); err != nil || data.ID <= 0 || !validFullName(data.FullName) {
		return Repository{}, ErrRepositoryUnavailable
	}
	return Repository{ID: data.ID, FullName: data.FullName}, nil
}
func repositoryClient(opts RepositoryOptions) (*http.Client, string, error) {
	base := opts.APIBaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
		return nil, "", ErrRepositoryUnavailable
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if opts.HTTPClient != nil {
		copy := *opts.HTTPClient
		copy.Timeout = 10 * time.Second
		copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &copy
	}
	return client, strings.TrimRight(base, "/"), nil
}
func decodeBounded(r io.Reader, dst any) error {
	data, err := io.ReadAll(io.LimitReader(r, repositoryBodyLimit+1))
	if err != nil || len(data) > repositoryBodyLimit {
		return ErrRepositoryUnavailable
	}
	return json.Unmarshal(data, dst)
}
func validFullName(s string) bool {
	p := strings.Split(s, "/")
	return len(p) == 2 && validGitHubPart(p[0]) && validGitHubPart(p[1])
}
func lookupWithGH(ctx context.Context, owner, repo string, opts RepositoryOptions) (Repository, error) {
	path := opts.GHPath
	if path == "" {
		path = "gh"
	}
	timeout := opts.CommandTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(c, path, "api", "--hostname", "github.com", "--method", "GET", "repos/"+owner+"/"+repo)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_PAGER=cat")
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	var out repositoryOutput
	cmd.Stdout = &out
	if cmd.Run() != nil || out.overflow {
		return Repository{}, ErrRepositoryUnavailable
	}
	var got Repository
	if json.Unmarshal(out.Bytes(), &got) != nil || got.ID <= 0 || !sameRepositoryName(got.FullName, owner+"/"+repo) {
		return Repository{}, ErrRepositoryUnavailable
	}
	return got, nil
}

func ListAccessibleRepositories(ctx context.Context, token string, opts RepositoryOptions) ([]Repository, error) {
	installations, err := fetchCollection(ctx, "/user/installations", "installations", token, opts)
	if err != nil {
		return nil, ErrRepositoryUnavailable
	}
	repositories, err := repositoriesFromInstallations(ctx, installations, token, opts)
	if err != nil {
		return nil, ErrRepositoryUnavailable
	}
	return repositories, nil
}
func repositoriesFromInstallations(ctx context.Context, installations []Repository, token string, opts RepositoryOptions) ([]Repository, error) {
	seen := map[int64]Repository{}
	for _, installation := range installations {
		if installation.ID <= 0 {
			return nil, ErrRepositoryUnavailable
		}
		items, err := fetchCollection(ctx, fmt.Sprintf("/user/installations/%d/repositories", installation.ID), "repositories", token, opts)
		if err != nil {
			return nil, err
		}
		if err := addUniqueRepositories(seen, items); err != nil {
			return nil, err
		}
	}
	result := make([]Repository, 0, len(seen))
	for _, repository := range seen {
		result = append(result, repository)
	}
	return result, nil
}
func addUniqueRepositories(seen map[int64]Repository, items []Repository) error {
	for _, repository := range items {
		if repository.ID <= 0 || !validFullName(repository.FullName) {
			return ErrRepositoryUnavailable
		}
		if previous, exists := seen[repository.ID]; exists && !sameRepositoryName(previous.FullName, repository.FullName) {
			return ErrRepositoryUnavailable
		}
		seen[repository.ID] = repository
	}
	return nil
}

type repositoryOutput struct {
	data     []byte
	overflow bool
}

func (b *repositoryOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > repositoryBodyLimit {
		b.overflow = true
		return 0, errors.New("output too large")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *repositoryOutput) Bytes() []byte { return b.data }

type collectionItem struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

func fetchCollection(ctx context.Context, path, key, token string, opts RepositoryOptions) ([]Repository, error) {
	all := []Repository{}
	for page := 1; page <= repositoryMaxPages; page++ {
		items, total, err := fetchCollectionPage(ctx, path, key, token, opts, page)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if total < 0 || len(all) > total {
			return nil, ErrRepositoryUnavailable
		}
		if len(all) == total {
			return all, nil
		}
		if len(items) < repositoryPageLimit {
			return nil, ErrRepositoryUnavailable
		}
	}
	return nil, ErrRepositoryUnavailable
}
func fetchCollectionPage(ctx context.Context, path, key, token string, opts RepositoryOptions, page int) ([]Repository, int, error) {
	client, base, err := repositoryClient(opts)
	if err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf("?per_page=%d&page=%d", repositoryPageLimit, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+query, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	var body struct {
		Installations []collectionItem `json:"installations"`
		Repositories  []collectionItem `json:"repositories"`
		Total         *int             `json:"total_count"`
	}
	err = decodeBounded(resp.Body, &body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || body.Total == nil {
		return nil, 0, ErrRepositoryUnavailable
	}
	items := body.Installations
	if key == "repositories" {
		items = body.Repositories
	}
	result := make([]Repository, 0, len(items))
	for _, item := range items {
		result = append(result, Repository{ID: item.ID, FullName: item.FullName})
	}
	return result, *body.Total, nil
}
func VerifyRepositoryScope(ctx context.Context, token string, selected Repository, opts RepositoryOptions) error {
	if selected.ID <= 0 || !validFullName(selected.FullName) {
		return ErrRepositoryUnavailable
	}
	items, err := ListAccessibleRepositories(ctx, token, opts)
	if err != nil || len(items) != 1 || items[0].ID != selected.ID || !sameRepositoryName(items[0].FullName, selected.FullName) {
		return ErrRepositoryUnavailable
	}
	return nil
}
