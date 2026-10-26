package githubapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	oauth "github.com/cli/oauth"
)

const maxResponseBytes = 1 << 20
const credentialRequestTimeout = 120 * time.Second

type TokenSet struct {
	Schema                int       `json:"schema"`
	ClientID              string    `json:"client_id"`
	ProjectPath           string    `json:"project_path"`
	UserID                int64     `json:"user_id"`
	RepositoryID          int64     `json:"repository_id"`
	Repository            string    `json:"repository"`
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type ProviderOptions struct {
	PassPath       string
	PassTimeout    time.Duration
	RequestTimeout time.Duration
	HTTPClient     *http.Client
	APIBaseURL     string
	Now            func() time.Time
	Secret         *SecretStore
	VerifyScope    func(context.Context, string, Config) error
	VerifyUser     func(context.Context, string, int64) error
}

type Provider struct {
	cfg    Config
	opts   ProviderOptions
	gate   chan struct{}
	cached TokenSet
}

func NewProvider(cfg Config, options ...ProviderOptions) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if len(options) > 1 {
		return nil, fmt.Errorf("only one provider options value is supported")
	}
	opts := ProviderOptions{}
	if len(options) == 1 {
		opts = options[0]
	}
	normalized, err := normalizeProviderOptions(cfg, opts)
	if err != nil {
		return nil, err
	}
	return &Provider{cfg: cfg, opts: normalized, gate: make(chan struct{}, 1)}, nil
}

func normalizeProviderOptions(cfg Config, opts ProviderOptions) (ProviderOptions, error) {
	if opts.PassPath == "" {
		opts.PassPath = "pass"
	}
	if opts.PassTimeout <= 0 {
		opts.PassTimeout = credentialRequestTimeout
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 15 * time.Second
	}
	if opts.APIBaseURL == "" {
		opts.APIBaseURL = "https://api.github.com"
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	opts.HTTPClient = configuredHTTPClient(opts.HTTPClient, opts.RequestTimeout)
	if _, _, err := repositoryClient(RepositoryOptions{HTTPClient: opts.HTTPClient, APIBaseURL: opts.APIBaseURL}); err != nil {
		return opts, fmt.Errorf("invalid GitHub API endpoint")
	}
	opts.Secret = configuredSecret(cfg, opts)
	if opts.VerifyScope == nil {
		opts.VerifyScope = defaultScopeVerifier(opts)
	}
	if opts.VerifyUser == nil {
		opts.VerifyUser = func(ctx context.Context, token string, expected int64) error {
			return verifyGitHubUser(ctx, token, expected, opts)
		}
	}
	return opts, nil
}

func configuredHTTPClient(input *http.Client, timeout time.Duration) *http.Client {
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if input != nil {
		*client = *input
		client.Timeout = timeout
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return client
}

func configuredSecret(cfg Config, opts ProviderOptions) *SecretStore {
	if opts.Secret == nil {
		return &SecretStore{PassPath: opts.PassPath, Timeout: opts.PassTimeout, Entry: secretEntry(cfg), Now: opts.Now}
	}
	secret := *opts.Secret
	if secret.Now == nil {
		secret.Now = opts.Now
	}
	return &secret
}

func defaultScopeVerifier(opts ProviderOptions) func(context.Context, string, Config) error {
	return func(ctx context.Context, token string, config Config) error {
		return VerifyRepositoryScope(ctx, token, Repository{ID: config.RepositoryID, FullName: config.Repository}, RepositoryOptions{HTTPClient: opts.HTTPClient, APIBaseURL: opts.APIBaseURL})
	}
}

func (p *Provider) Token(ctx context.Context) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, credentialRequestTimeout)
	defer cancel()
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-requestCtx.Done():
		return "", requestCtx.Err()
	}
	if p.cached.AccessToken != "" && p.cached.AccessTokenExpiresAt.After(p.opts.Now().Add(5*time.Minute)) {
		return p.cached.AccessToken, nil
	}
	var token string
	err := p.opts.Secret.WithLock(requestCtx, func() error {
		var err error
		token, err = p.tokenLocked(requestCtx)
		return err
	})
	return token, err
}

func (p *Provider) tokenLocked(ctx context.Context) (string, error) {
	set, err := p.opts.Secret.Read(ctx, p.cfg)
	if err != nil {
		return "", err
	}
	if set.AccessToken == "" || set.RefreshToken == "" {
		return "", fmt.Errorf("authorization required")
	}
	if set.AccessTokenExpiresAt.After(p.opts.Now().Add(5 * time.Minute)) {
		p.cached = set
		return set.AccessToken, nil
	}
	if !set.RefreshTokenExpiresAt.After(p.opts.Now()) {
		return "", fmt.Errorf("authorization required")
	}
	fresh, err := p.refresh(ctx, set)
	if err != nil {
		return "", err
	}
	if p.opts.VerifyScope == nil {
		return "", fmt.Errorf("repository scope cannot be verified")
	}
	if err := p.opts.VerifyScope(ctx, fresh.AccessToken, p.cfg); err != nil {
		return "", fmt.Errorf("authorization required: repository scope verification failed")
	}
	if err := p.opts.VerifyUser(ctx, fresh.AccessToken, p.cfg.UserID); err != nil {
		return "", fmt.Errorf("authorization required: account verification failed")
	}
	if err := p.opts.Secret.Write(ctx, fresh); err != nil {
		return "", fmt.Errorf("authorization required: rotated credential could not be saved")
	}
	p.cached = fresh
	return fresh.AccessToken, nil
}

func (p *Provider) refresh(ctx context.Context, current TokenSet) (TokenSet, error) {
	host := oauth.GitHubHost("https://github.com")
	client := contextFormClient{ctx: ctx, client: p.opts.HTTPClient}
	pair, err := oauth.Refresh(oauth.RefreshOptions{Host: host, ClientID: p.cfg.ClientID, RefreshToken: current.RefreshToken, HTTPClient: client})
	if err != nil {
		if errors.Is(err, oauth.ErrRefreshTokenInvalid) {
			return TokenSet{}, fmt.Errorf("authorization required")
		}
		return TokenSet{}, fmt.Errorf("GitHub token refresh failed")
	}
	if pair.Token == "" || pair.RefreshToken == "" || pair.RefreshToken == current.RefreshToken || pair.ExpiresIn <= 0 || pair.RefreshTokenExpiresIn <= 0 {
		return TokenSet{}, fmt.Errorf("invalid GitHub refresh response")
	}
	current.AccessToken = pair.Token
	current.RefreshToken = pair.RefreshToken
	now := p.opts.Now()
	current.AccessTokenExpiresAt = now.Add(pair.ExpiresIn)
	current.RefreshTokenExpiresAt = now.Add(pair.RefreshTokenExpiresIn)
	return current, nil
}

type contextFormClient struct {
	ctx          context.Context
	client       *http.Client
	repositoryID int64
}

func (c contextFormClient) PostForm(target string, values url.Values) (*http.Response, error) {
	parsed, err := url.Parse(target)
	if err != nil || !allowedOAuthEndpoint(parsed) {
		return nil, fmt.Errorf("unsupported GitHub OAuth endpoint")
	}
	if values.Get("grant_type") == "urn:ietf:params:oauth:grant-type:device_code" && c.repositoryID > 0 {
		values.Set("repository_id", fmt.Sprint(c.repositoryID))
	}
	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, target, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := readBoundedResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GitHub OAuth response unavailable")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func allowedOAuthEndpoint(endpoint *url.URL) bool {
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return false
	}
	if endpoint.Scheme == "https" && endpoint.Host == "github.com" && (endpoint.Path == "/login/oauth/access_token" || endpoint.Path == "/login/device/code") {
		return true
	}
	return endpoint.Scheme == "http" && (endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost")
}

func readBoundedResponse(body io.ReadCloser) ([]byte, error) {
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return nil, fmt.Errorf("response unavailable")
	}
	return data, nil
}

func verifyGitHubUser(ctx context.Context, token string, expectedID int64, opts ProviderOptions) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(opts.APIBaseURL, "/")+"/user", nil)
	if err != nil {
		return ErrRepositoryUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return ErrRepositoryUnavailable
	}
	defer resp.Body.Close()
	var user struct {
		ID int64 `json:"id"`
	}
	if resp.StatusCode != http.StatusOK || decodeBounded(resp.Body, &user) != nil || user.ID != expectedID {
		return ErrRepositoryUnavailable
	}
	return nil
}

func secretEntry(cfg Config) string {
	return "dch/github-app/users/" + projectDigest(cfg.ProjectPath) + "/" + clientDigest(cfg.ClientID)
}
func projectDigest(path string) string  { return pathHash(path) }
func clientDigest(client string) string { return pathHash(client) }
func pathHash(value string) string      { sum := sha256sum([]byte(value)); return hexsum(sum[:12]) }
func sha256sum(b []byte) []byte         { sum := sha256.Sum256(b); return sum[:] }
func hexsum(b []byte) string            { return fmt.Sprintf("%x", b) }

func passCommand(ctx context.Context, timeout time.Duration, path string, args ...string) *exec.Cmd {
	c, cancel := context.WithTimeout(ctx, timeout)
	_ = cancel
	return exec.CommandContext(c, path, args...)
}
func securePassEnvironment(env []string) []string {
	filtered := make([]string, 0, len(env)+3)
	for _, value := range env {
		if strings.HasPrefix(value, "GPG_TTY=") || strings.HasPrefix(value, "PASSWORD_STORE_GPG_OPTS=") || strings.HasPrefix(value, "LC_ALL=") {
			continue
		}
		filtered = append(filtered, value)
	}
	return append(filtered, "PASSWORD_STORE_GPG_OPTS=--pinentry-mode ask", "LC_ALL=C")
}
func cleanPassOutput(data []byte) string        { return strings.TrimSpace(string(data)) }
func tokenSetJSON(set TokenSet) ([]byte, error) { return json.Marshal(set) }
func passEnv() []string                         { return securePassEnvironment(os.Environ()) }

type boundedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > maxResponseBytes {
		b.overflow = true
		return 0, fmt.Errorf("output too large")
	}
	return b.Buffer.Write(data)
}
