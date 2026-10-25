package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const maxResponseBytes = 1 << 20

type ProviderOptions struct {
	PassPath       string
	PassTimeout    time.Duration
	RequestTimeout time.Duration
	HTTPClient     *http.Client
	APIBaseURL     string
	Now            func() time.Time
}

type Provider struct {
	cfg        Config
	opts       ProviderOptions
	mu         sync.Mutex
	refreshing *refreshCall
	token      string
	expires    time.Time
}

type refreshCall struct {
	done  chan struct{}
	token string
	err   error
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
	if opts.PassPath == "" {
		opts.PassPath = "pass"
	}
	if opts.PassTimeout <= 0 {
		opts.PassTimeout = 10 * time.Second
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
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: opts.RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else {
		client := *opts.HTTPClient
		client.Timeout = opts.RequestTimeout
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		opts.HTTPClient = &client
	}
	u, err := url.Parse(opts.APIBaseURL)
	if err != nil || u.Scheme != "https" && opts.APIBaseURL != "http://127.0.0.1" && !strings.HasPrefix(opts.APIBaseURL, "http://127.0.0.1:") {
		return nil, fmt.Errorf("GitHub API URL must use HTTPS")
	}
	return &Provider{cfg: cfg, opts: opts}, nil
}

func (p *Provider) Token(ctx context.Context) (string, error) {
	for {
		p.mu.Lock()
		if p.token != "" && p.opts.Now().Add(5*time.Minute).Before(p.expires) {
			token := p.token
			p.mu.Unlock()
			return token, nil
		}
		if call := p.refreshing; call != nil {
			p.mu.Unlock()
			return awaitRefresh(ctx, call)
		}
		call := &refreshCall{done: make(chan struct{})}
		p.refreshing = call
		p.mu.Unlock()
		return p.refreshToken(ctx, call)
	}
}

func awaitRefresh(ctx context.Context, call *refreshCall) (string, error) {
	select {
	case <-call.done:
		if call.err != nil {
			return "", call.err
		}
		return call.token, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *Provider) refreshToken(ctx context.Context, call *refreshCall) (string, error) {
	token, expiry, err := p.mint(ctx)
	p.mu.Lock()
	if err == nil {
		p.token, p.expires = token, expiry
		call.token = token
	} else {
		call.err = err
	}
	p.refreshing = nil
	close(call.done)
	p.mu.Unlock()
	return token, err
}

func (p *Provider) mint(ctx context.Context) (string, time.Time, error) {
	key, err := p.loadPrivateKey(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	jwt, err := p.createAppJWT(key)
	if err != nil {
		return "", time.Time{}, err
	}
	owner, repo, _ := strings.Cut(p.cfg.Repository, "/")
	if err := p.verifyRepositoryInstallation(ctx, jwt, owner, repo); err != nil {
		return "", time.Time{}, err
	}
	return p.requestInstallationToken(ctx, jwt, repo)
}

func (p *Provider) loadPrivateKey(ctx context.Context) (*rsa.PrivateKey, error) {
	passCtx, cancel := context.WithTimeout(ctx, p.opts.PassTimeout)
	defer cancel()
	cmd := exec.CommandContext(passCtx, p.opts.PassPath, "show", p.cfg.PrivateKeyPassEntry)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = io.Discard
	cmd.Env = securePassEnvironment(os.Environ())
	var keyOutput boundedBuffer
	cmd.Stdout = &keyOutput
	if err := cmd.Run(); err != nil || keyOutput.overflow {
		return nil, fmt.Errorf("pass failed")
	}
	return parsePrivateKey(keyOutput.Bytes())
}

func (p *Provider) createAppJWT(key *rsa.PrivateKey) (string, error) {
	now := p.opts.Now()
	issued := now.Add(-60 * time.Second)
	claims, _ := json.Marshal(map[string]any{"iat": issued.Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": p.cfg.appIDString()})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (p *Provider) verifyRepositoryInstallation(ctx context.Context, jwt, owner, repo string) error {
	verifyURL := strings.TrimRight(p.opts.APIBaseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/installation"
	verifyReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, verifyURL, nil)
	verifyReq.Header.Set("Authorization", "Bearer "+jwt)
	verifyReq.Header.Set("Accept", "application/vnd.github+json")
	verifyReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	verifyResp, err := p.opts.HTTPClient.Do(verifyReq)
	if err != nil {
		return fmt.Errorf("repository installation verification failed")
	}
	defer verifyResp.Body.Close()
	verifyData, err := io.ReadAll(io.LimitReader(verifyResp.Body, maxResponseBytes+1))
	var installation struct {
		ID int64 `json:"id"`
	}
	if err != nil || len(verifyData) > maxResponseBytes || verifyResp.StatusCode != http.StatusOK || json.Unmarshal(verifyData, &installation) != nil || installation.ID != p.cfg.InstallationID {
		return fmt.Errorf("repository installation verification failed")
	}
	return nil
}

func (p *Provider) requestInstallationToken(ctx context.Context, jwt, repo string) (string, time.Time, error) {
	var body bytes.Buffer
	permissions := make(map[string]string, len(p.cfg.Permissions))
	for k, v := range p.cfg.Permissions {
		permissions[k] = v
	}
	if err := json.NewEncoder(&body).Encode(map[string]any{"repositories": []string{repo}, "permissions": permissions}); err != nil {
		return "", time.Time{}, err
	}
	u := strings.TrimRight(p.opts.APIBaseURL, "/") + fmt.Sprintf("/app/installations/%d/access_tokens", p.cfg.InstallationID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, &body)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.opts.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("GitHub token request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("GitHub token request failed")
	}
	var minted struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if json.Unmarshal(data, &minted) != nil || minted.Token == "" || strings.ContainsAny(minted.Token, "\r\n") {
		return "", time.Time{}, fmt.Errorf("invalid GitHub token response")
	}
	expires, err := time.Parse(time.RFC3339, minted.ExpiresAt)
	if err != nil || !expires.After(p.opts.Now().Add(5*time.Minute)) {
		return "", time.Time{}, fmt.Errorf("invalid GitHub token expiry")
	}
	return minted.Token, expires, nil
}

func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("invalid RSA private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid RSA private key")
	}
	return key, nil
}

type boundedBuffer struct {
	bytes.Buffer
	overflow bool
}

func securePassEnvironment(env []string) []string {
	filtered := make([]string, 0, len(env)+2)
	var gpgOptions string
	for _, item := range env {
		if strings.HasPrefix(item, "GPG_TTY=") {
			continue
		}
		if strings.HasPrefix(item, "PASSWORD_STORE_GPG_OPTS=") {
			gpgOptions = strings.TrimPrefix(item, "PASSWORD_STORE_GPG_OPTS=")
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, "GPG_TTY=", "PASSWORD_STORE_GPG_OPTS="+strings.TrimSpace(gpgOptions+" --batch --pinentry-mode error"))
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxResponseBytes {
		b.overflow = true
		return 0, fmt.Errorf("output too large")
	}
	return b.Buffer.Write(p)
}
