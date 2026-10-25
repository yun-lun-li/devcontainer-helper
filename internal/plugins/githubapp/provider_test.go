package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSecretStoreWritesAndReadsCompleteJSON(t *testing.T) {
	pass := filepath.Join(t.TempDir(), "pass")
	_ = os.WriteFile(pass, []byte("#!/bin/sh\nif [ \"$1\" = insert ]; then cat > \"$PASS_SECRET\"; exit; fi\ncat \"$PASS_SECRET\"\n"), 0700)
	cfg := Config{ClientID: "Iv1_example", ProjectPath: "/tmp/p", UserID: 8, Login: "tester", RepositoryID: 9, Repository: "octo/repo"}
	now := time.Now()
	set := TokenSet{Schema: 1, ClientID: cfg.ClientID, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, AccessToken: "access", RefreshToken: "refresh", AccessTokenExpiresAt: now.Add(time.Hour), RefreshTokenExpiresAt: now.Add(24 * time.Hour)}
	entry := filepath.Join(t.TempDir(), "secret.json")
	store := &SecretStore{PassPath: pass, Entry: "entry"}
	t.Setenv("PASS_SECRET", entry)
	if err := store.Write(context.Background(), set); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != set.AccessToken || got.RefreshToken != set.RefreshToken || !got.AccessTokenExpiresAt.Equal(set.AccessTokenExpiresAt) || !got.RefreshTokenExpiresAt.Equal(set.RefreshTokenExpiresAt) {
		t.Fatalf("got %#v want %#v", got, set)
	}
}

func TestContextFormClientAddsRepositoryIDOnlyToDevicePollAndHonorsCancellation(t *testing.T) {
	var poll, request string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if strings.Contains(string(data), "device_code") {
			poll = string(data)
		} else {
			request = string(data)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := contextFormClient{ctx: context.Background(), client: srv.Client(), repositoryID: 41}
	_, _ = client.PostForm(srv.URL, url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}})
	_, _ = client.PostForm(srv.URL, url.Values{"grant_type": {"refresh_token"}, "scope": {"offline_access"}})
	if !strings.Contains(poll, "repository_id=41") || strings.Contains(request, "repository_id") {
		t.Fatalf("poll=%q request=%q", poll, request)
	}
}

func TestSecretStoreAllowsExpiredAccessWhileRefreshTokenIsValid(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := testProviderConfig()
	set := testTokenSet(cfg, now.Add(-time.Minute), now.Add(time.Hour))
	store := newTestSecretStore(t, set)
	store.Now = func() time.Time { return now }
	got, err := store.Read(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessTokenExpiresAt.After(now) {
		t.Fatalf("expected expired access token, got %s", got.AccessTokenExpiresAt)
	}
}

func TestProviderCachesValidTokenInMemory(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := testProviderConfig()
	set := testTokenSet(cfg, now.Add(time.Hour), now.Add(24*time.Hour))
	store := newTestSecretStore(t, set)
	store.Now = func() time.Time { return now }
	provider, err := NewProvider(cfg, ProviderOptions{Now: func() time.Time { return now }, Secret: store, VerifyScope: noVerifyScope, VerifyUser: noVerifyUser})
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != set.AccessToken || second != first {
		t.Fatalf("tokens = %q, %q", first, second)
	}
}

func TestProviderRefreshVerifiesUserAndScopeBeforeSavingRotatedPair(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := testProviderConfig()
	previous := testTokenSet(cfg, now.Add(-time.Minute), now.Add(time.Hour))
	store := newTestSecretStore(t, previous)
	store.Now = func() time.Time { return now }
	transport := providerTransport(func(req *http.Request) *http.Response {
		if req.URL.String() != "https://github.com/login/oauth/access_token" {
			return providerResponse(req, http.StatusNotFound, "")
		}
		return providerResponse(req, http.StatusOK, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"refresh_token_expires_in":86400}`)
	})
	scopes, users := 0, 0
	provider, err := NewProvider(cfg, ProviderOptions{Now: func() time.Time { return now }, Secret: store, HTTPClient: &http.Client{Transport: transport}, VerifyScope: func(_ context.Context, token string, got Config) error {
		scopes++
		if token != "fresh" || got.RepositoryID != cfg.RepositoryID {
			t.Fatal("scope verification received wrong binding")
		}
		return nil
	}, VerifyUser: func(_ context.Context, token string, userID int64) error {
		users++
		if token != "fresh" || userID != cfg.UserID {
			t.Fatal("identity verification received wrong binding")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "fresh" || scopes != 1 || users != 1 {
		t.Fatalf("token %q, scope checks %d, user checks %d", token, scopes, users)
	}
	saved, err := store.Read(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != "rotated" || saved.AccessToken != "fresh" {
		t.Fatalf("saved %#v", saved)
	}
}

func TestProviderDoesNotSaveOrPublishRefreshWhenScopeFails(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := testProviderConfig()
	previous := testTokenSet(cfg, now.Add(-time.Minute), now.Add(time.Hour))
	store := newTestSecretStore(t, previous)
	store.Now = func() time.Time { return now }
	transport := providerTransport(func(req *http.Request) *http.Response {
		return providerResponse(req, http.StatusOK, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"refresh_token_expires_in":86400}`)
	})
	provider, err := NewProvider(cfg, ProviderOptions{Now: func() time.Time { return now }, Secret: store, HTTPClient: &http.Client{Transport: transport}, VerifyScope: func(context.Context, string, Config) error { return fmt.Errorf("scope mismatch") }, VerifyUser: noVerifyUser})
	if err != nil {
		t.Fatal(err)
	}
	if token, err := provider.Token(context.Background()); err == nil || token != "" {
		t.Fatalf("got token %q, err %v", token, err)
	}
	saved, err := store.Read(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != previous.RefreshToken || saved.AccessToken != previous.AccessToken {
		t.Fatalf("rotated pair was saved: %#v", saved)
	}
}

type providerRoundTripFunc func(*http.Request) *http.Response

func (f providerRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}
func providerTransport(fn providerRoundTripFunc) http.RoundTripper { return fn }
func providerResponse(req *http.Request, status int, body string) *http.Response {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}
func noVerifyScope(context.Context, string, Config) error { return nil }
func noVerifyUser(context.Context, string, int64) error   { return nil }
func testProviderConfig() Config {
	return Config{ClientID: "Iv1_test", ProjectPath: "/tmp/project", UserID: 8, Login: "tester", RepositoryID: 9, Repository: "octo/repo"}
}
func testTokenSet(cfg Config, accessExpiry, refreshExpiry time.Time) TokenSet {
	return TokenSet{Schema: 1, ClientID: cfg.ClientID, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, AccessToken: "access", RefreshToken: "refresh", AccessTokenExpiresAt: accessExpiry, RefreshTokenExpiresAt: refreshExpiry}
}
func newTestSecretStore(t *testing.T, initial TokenSet) *SecretStore {
	t.Helper()
	pass := filepath.Join(t.TempDir(), "pass")
	entry := filepath.Join(t.TempDir(), "entry.json")
	script := "#!/bin/sh\necho \"$1\" >> \"$PASS_LOG\"\ncase \"$1\" in\ninsert) cat > \"$PASS_SECRET\" ;;\nshow) cat \"$PASS_SECRET\" ;;\n*) exit 2 ;;\nesac\n"
	if err := os.WriteFile(pass, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASS_SECRET", entry)
	t.Setenv("PASS_LOG", filepath.Join(t.TempDir(), "commands"))
	store := &SecretStore{PassPath: pass, Entry: "entry", LockDir: filepath.Join(t.TempDir(), "locks")}
	raw, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestProvidersShareOneCrossProcessRefresh(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := testProviderConfig()
	previous := testTokenSet(cfg, now.Add(-time.Minute), now.Add(time.Hour))
	store := newTestSecretStore(t, previous)
	store.Now = func() time.Time { return now }
	var requests atomic.Int32
	transport := providerRoundTripFunc(func(req *http.Request) *http.Response {
		requests.Add(1)
		time.Sleep(30 * time.Millisecond)
		return providerResponse(req, http.StatusOK, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"refresh_token_expires_in":86400}`)
	})
	options := ProviderOptions{Now: func() time.Time { return now }, Secret: store, HTTPClient: &http.Client{Transport: transport}, VerifyScope: noVerifyScope, VerifyUser: noVerifyUser}
	first, err := NewProvider(cfg, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewProvider(cfg, options)
	if err != nil {
		t.Fatal(err)
	}
	providers := []*Provider{first, second}
	var wait sync.WaitGroup
	failures := make(chan error, len(providers))
	for _, provider := range providers {
		wait.Add(1)
		go func(provider *Provider) {
			defer wait.Done()
			token, err := provider.Token(context.Background())
			if err != nil {
				failures <- err
				return
			}
			if token != "fresh" {
				failures <- fmt.Errorf("unexpected token")
			}
		}(provider)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("refresh requests = %d, want 1", got)
	}
}

func TestProviderValidMemoryCacheAvoidsSecondSecretRead(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := testProviderConfig()
	store := newTestSecretStore(t, testTokenSet(cfg, now.Add(time.Hour), now.Add(24*time.Hour)))
	store.Now = func() time.Time { return now }
	provider, err := NewProvider(cfg, ProviderOptions{Now: func() time.Time { return now }, Secret: store, VerifyScope: noVerifyScope, VerifyUser: noVerifyUser})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := provider.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	log, err := os.ReadFile(os.Getenv("PASS_LOG"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(log), "show\n"); got != 1 {
		t.Fatalf("pass show calls = %d, want 1; log %q", got, log)
	}
}

func TestProviderVerifiesTheBoundGitHubUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fresh" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"id":8}`)
	}))
	defer server.Close()
	provider, err := NewProvider(testProviderConfig(), ProviderOptions{APIBaseURL: server.URL, VerifyScope: noVerifyScope})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.opts.VerifyUser(context.Background(), "fresh", 8); err != nil {
		t.Fatal(err)
	}
	if err := provider.opts.VerifyUser(context.Background(), "fresh", 9); err == nil {
		t.Fatal("accepted a different GitHub user")
	}
}

func TestSecretStoreSnapshotRestoreAndDelete(t *testing.T) {
	dir := t.TempDir()
	pass := filepath.Join(dir, "pass")
	entry := filepath.Join(dir, "entry")
	script := "#!/bin/sh\ncase \"$1\" in\nshow) if [ -f \"$PASS_SECRET\" ]; then cat \"$PASS_SECRET\"; else echo 'Error: not in the password store' >&2; exit 1; fi ;;\ninsert) cat > \"$PASS_SECRET\" ;;\nrm) rm -f \"$PASS_SECRET\" ;;\n*) exit 2 ;;\nesac\n"
	if err := os.WriteFile(pass, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASS_SECRET", entry)
	store := &SecretStore{PassPath: pass, Entry: "entry"}
	if _, err := store.Snapshot(context.Background()); err != ErrSecretMissing {
		t.Fatalf("missing snapshot err = %v", err)
	}
	cfg := testProviderConfig()
	now := time.Now()
	set := testTokenSet(cfg, now.Add(time.Hour), now.Add(24*time.Hour))
	raw, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreRaw(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	got, err := store.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), raw) {
		t.Fatal("snapshot did not preserve the full credential JSON")
	}
	if err := store.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSecretStoreRejectsSymlinkLockDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "locks")
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}
	store := &SecretStore{Entry: "entry", LockDir: lockPath}
	if err := store.WithLock(context.Background(), func() error { return nil }); err == nil {
		t.Fatal("accepted a symlink lock directory")
	}
}

func TestContextFormClientCancelsInflightRequest(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	client := contextFormClient{ctx: ctx, client: &http.Client{Transport: blockingTransport{started: started}}}
	done := make(chan error, 1)
	go func() {
		_, err := client.PostForm("https://github.com/login/oauth/access_token", url.Values{"grant_type": {"refresh_token"}})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("request ignored context cancellation")
	}
}

type blockingTransport struct{ started chan struct{} }

func (b blockingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	close(b.started)
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestContextFormClientAllowsOnlyGitHubDeviceAndTokenEndpoints(t *testing.T) {
	for _, target := range []string{"https://github.com/login/device/code", "https://github.com/login/oauth/access_token"} {
		parsed, err := url.Parse(target)
		if err != nil || !allowedOAuthEndpoint(parsed) {
			t.Errorf("rejected endpoint %q", target)
		}
	}
	for _, target := range []string{"https://github.com/other", "https://user@github.com/login/device/code", "https://github.com/login/device/code?x=y"} {
		parsed, err := url.Parse(target)
		if err != nil || allowedOAuthEndpoint(parsed) {
			t.Errorf("accepted endpoint %q", target)
		}
	}
}
