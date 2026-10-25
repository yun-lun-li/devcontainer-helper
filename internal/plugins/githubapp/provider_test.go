package githubapp

import (
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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func TestProviderMintsScopedTokenAndCachesIt(t *testing.T) {
	key := testPrivateKey(t)
	t.Setenv("GPG_TTY", "/dev/pts/9")
	t.Setenv("PASSWORD_STORE_GPG_OPTS", "--no-tty --pinentry-mode loopback")
	privateKey, err := parsePrivateKey([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2029, 12, 31, 0, 0, 0, 0, time.UTC)
	var tokenCalls, repoCalls int
	srv := httptest.NewServer(scopedTokenHandler(t, privateKey, now, &tokenCalls, &repoCalls))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	fakePass := "#!/bin/sh\n[ \"$1\" = show ] && [ \"$2\" = key ] || exit 9\n[ -z \"$GPG_TTY\" ] || exit 10\n[ \"$PASSWORD_STORE_GPG_OPTS\" = '--no-tty --pinentry-mode loopback --batch --pinentry-mode error' ] || exit 11\ncat <<'KEY'\n" + key + "\nKEY\n"
	if err := os.WriteFile(pass, []byte(fakePass), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{AppID: 12, InstallationID: 34, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}
	p, err := NewProvider(cfg, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := p.Token(context.Background())
		if err != nil || got != "opaque-token" {
			t.Fatalf("Token() = %q, %v", got, err)
		}
	}
	if tokenCalls != 1 || repoCalls != 1 {
		t.Fatalf("calls token=%d installation=%d", tokenCalls, repoCalls)
	}
}

func TestProviderDefaultsPassEntryForDirectConfig(t *testing.T) {
	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, []byte("#!/bin/sh\nprintf '%s' \"$2\" > \"$PASS_ARG_FILE\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	argFile := filepath.Join(t.TempDir(), "arg")
	t.Setenv("PASS_ARG_FILE", argFile)
	p, err := NewProvider(Config{AppID: 12, InstallationID: 34, Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "dch/github-app/12/private-key") || strings.Contains(err.Error(), "exit") {
		t.Fatalf("Token() error = %v", err)
	}
	entry, readErr := os.ReadFile(argFile)
	if readErr != nil || string(entry) != "dch/github-app/12/private-key" {
		t.Fatalf("pass entry = %q, %v", entry, readErr)
	}
}

func TestProviderRequestsInstallationWideTokenWithoutRepositoryLookup(t *testing.T) {
	key := testPrivateKey(t)
	var lookupCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/installation") {
			lookupCalls.Add(1)
			t.Errorf("unexpected repository installation lookup: %s", r.URL.Path)
			return
		}
		if r.URL.Path != "/app/installations/34/access_tokens" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, hasRepos := body["repositories"]; hasRepos {
			t.Errorf("installation-wide body restricted repositories: %#v", body)
		}
		if body["permissions"].(map[string]any)["contents"] != "read" {
			t.Errorf("unexpected permissions: %#v", body)
		}
		writeScopedTokenResponse(w)
	}))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider(Config{AppID: 1, InstallationID: 34, Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if lookupCalls.Load() != 0 {
		t.Fatalf("repository lookups = %d", lookupCalls.Load())
	}
}

func scopedTokenHandler(t *testing.T, privateKey *rsa.PrivateKey, now time.Time, tokenCalls, repoCalls *int) http.HandlerFunc {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo/repo/installation" {
			*repoCalls++
			assertBearerAuthorization(t, r)
			_, _ = w.Write([]byte(`{"id":34}`))
			return
		}
		if r.URL.Path == "/app/installations/34/access_tokens" {
			*tokenCalls++
			assertScopedTokenRequest(t, r, privateKey, now)
			writeScopedTokenResponse(w)
			return
		}
		http.NotFound(w, r)
	})
}

func assertBearerAuthorization(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		t.Error("installation lookup did not use App JWT")
	}
}

func assertScopedTokenRequest(t *testing.T, r *http.Request, privateKey *rsa.PrivateKey, now time.Time) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
	if len(parts) != 3 {
		t.Fatalf("invalid JWT: %q", r.Header.Get("Authorization"))
	}
	assertJWTHeader(t, parts[0])
	assertJWTSignature(t, parts, privateKey)
	assertJWTClaims(t, parts[1], now)
	assertTokenScope(t, r)
}

func assertJWTHeader(t *testing.T, encoded string) {
	t.Helper()
	headerJSON, _ := base64.RawURLEncoding.DecodeString(encoded)
	var header map[string]string
	_ = json.Unmarshal(headerJSON, &header)
	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		t.Errorf("unexpected JWT header: %#v", header)
	}
}

func assertJWTSignature(t *testing.T, parts []string, privateKey *rsa.PrivateKey) {
	t.Helper()
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		t.Error("JWT signature did not verify")
	}
}

func assertJWTClaims(t *testing.T, encoded string, now time.Time) {
	t.Helper()
	claimsJSON, _ := base64.RawURLEncoding.DecodeString(encoded)
	var claims map[string]any
	_ = json.Unmarshal(claimsJSON, &claims)
	if claims["iss"] != "12" || claims["iat"] != float64(now.Unix()-60) || claims["exp"] != float64(now.Unix()+9*60) {
		t.Errorf("unexpected claims: %#v", claims)
	}
}

func assertTokenScope(t *testing.T, r *http.Request) {
	t.Helper()
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["repositories"].([]any)[0] != "repo" || body["permissions"].(map[string]any)["contents"] != "read" {
		t.Errorf("unexpected scope: %#v", body)
	}
}

func writeScopedTokenResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"token":"opaque-token","expires_at":"2030-01-01T00:00:00Z"}`))
}

func TestProviderRejectsWrongRepositoryInstallationBeforeMinting(t *testing.T) {
	key := testPrivateKey(t)
	var tokenCalls, lookupCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo/repo/installation" {
			lookupCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":99}`))
			return
		}
		if r.URL.Path == "/app/installations/34/access_tokens" {
			tokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"token":"wrong-installation-token","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		if r.URL.Path == "/repos/octo/repo" {
			// GitHub returns public repository metadata without installation scope.
			_, _ = w.Write([]byte(`{"full_name":"octo/repo"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider(Config{AppID: 1, InstallationID: 34, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if token, err := p.Token(context.Background()); err == nil || token != "" {
		t.Fatalf("Token() = %q, %v; want rejected installation", token, err)
	}
	if tokenCalls.Load() != 0 {
		t.Fatalf("minted %d tokens for wrong installation", tokenCalls.Load())
	}
	if lookupCalls.Load() != 1 {
		t.Fatalf("installation lookup calls=%d; want 1", lookupCalls.Load())
	}
}

func TestProviderRejectsFailedRepositoryInstallationLookup(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"not found", `{"message":"Not Found"}`, http.StatusNotFound}, {"malformed", `{"id":"bad"}`, http.StatusOK}} {
		t.Run(tc.name, func(t *testing.T) {
			assertFailedInstallationLookup(t, tc.status, tc.body)
		})
	}
}

func assertFailedInstallationLookup(t *testing.T, status int, body string) {
	t.Helper()
	key := testPrivateKey(t)
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(failedInstallationHandler(status, body, &tokenCalls))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider(Config{AppID: 1, InstallationID: 34, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if token, err := p.Token(context.Background()); err == nil || token != "" {
		t.Fatalf("Token() = %q, %v; want lookup failure", token, err)
	}
	if tokenCalls.Load() != 0 {
		t.Fatalf("minted %d tokens after failed lookup", tokenCalls.Load())
	}
}

func failedInstallationHandler(status int, body string, tokenCalls *atomic.Int32) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo/repo/installation" {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			tokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"token":"must-not-mint","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		http.NotFound(w, r)
	})
}

func TestProviderRejectsMalformedOrExpiredResponsesAndRetries(t *testing.T) {
	cases := []struct{ name, response string }{{"malformed", `{"token":`}, {"expired", `{"token":"old-token","expires_at":"2000-01-01T00:00:00Z"}`}, {"invalid-expiry", `{"token":"bad-expiry","expires_at":"tomorrow"}`}, {"invalid-token", `{"token":"bad\ntoken","expires_at":"2030-01-01T00:00:00Z"}`}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidResponseRetries(t, tc.response)
		})
	}
}

func assertInvalidResponseRetries(t *testing.T, response string) {
	t.Helper()
	key := testPrivateKey(t)
	var calls atomic.Int32
	srv := httptest.NewServer(invalidResponseHandler(response, &calls))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	_ = os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700)
	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Token(context.Background()); err == nil {
		t.Fatal("invalid response accepted")
	}
	got, err := p.Token(context.Background())
	if err != nil || got != "retry-token" {
		t.Fatalf("retry = %q, %v", got, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("API calls=%d", calls.Load())
	}
}

func invalidResponseHandler(response string, calls *atomic.Int32) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			if calls.Add(1) == 1 {
				_, _ = w.Write([]byte(response))
				return
			}
			_, _ = w.Write([]byte(`{"token":"retry-token","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":2}`))
	})
}

func TestProviderSanitizesAPIErrorAndRetriesNextRequest(t *testing.T) {
	key := testPrivateKey(t)
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			if tokenCalls.Add(1) == 1 {
				http.Error(w, "sensitive github body", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"token":"retry-token","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":2}`))
	}))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	_ = os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700)
	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Token(context.Background())
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unsafe error: %v", err)
	}
	got, err := p.Token(context.Background())
	if err != nil || got != "retry-token" {
		t.Fatalf("retry Token() = %q, %v", got, err)
	}
}

func TestProviderRefreshesWhenTokenHasFiveMinutesLeft(t *testing.T) {
	testProviderRefresh(t, time.Date(2029, 12, 31, 23, 54, 0, 0, time.UTC), 2)
}

func TestProviderCoalescesConcurrentMints(t *testing.T) {
	key := testPrivateKey(t)
	var calls atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			calls.Add(1)
			<-release
			_, _ = w.Write([]byte(`{"token":"shared-token","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":2}`))
	}))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	_ = os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700)
	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := p.Token(context.Background())
			if err == nil && token != "shared-token" {
				err = fmt.Errorf("token=%q", token)
			}
			errs <- err
		}()
	}
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("mint calls=%d", calls.Load())
	}
}

func TestProviderCoalescesConcurrentFailureAndAllowsLaterRetry(t *testing.T) {
	key := testPrivateKey(t)
	var calls atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			if calls.Add(1) == 1 {
				<-release
				http.Error(w, "private API details", http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(`{"token":"recovered-token","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":2}`))
	}))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	_ = os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700)
	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := p.Token(context.Background()); errs <- err }()
	}
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err == nil {
			t.Error("expected shared failed refresh")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent failed mint calls=%d", calls.Load())
	}
	got, err := p.Token(context.Background())
	if err != nil || got != "recovered-token" {
		t.Fatalf("retry = %q, %v", got, err)
	}
}

func testProviderRefresh(t *testing.T, now time.Time, expectedCalls int32) {
	t.Helper()
	key := testPrivateKey(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			count := calls.Add(1)
			if count == 1 {
				_, _ = w.Write([]byte(`{"token":"refresh-token","expires_at":"2030-01-01T00:00:00Z"}`))
			} else {
				_, _ = w.Write([]byte(`{"token":"refresh-token-2","expires_at":"2030-01-01T01:00:00Z"}`))
			}
			return
		}
		_, _ = w.Write([]byte(`{"id":2}`))
	}))
	defer srv.Close()
	pass := filepath.Join(t.TempDir(), "pass")
	_ = os.WriteFile(pass, []byte("#!/bin/sh\ncat <<'KEY'\n"+key+"\nKEY\n"), 0700)
	current := now
	p, err := NewProvider(Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}}, ProviderOptions{PassPath: pass, APIBaseURL: srv.URL, HTTPClient: srv.Client(), Now: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	_, err = p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != expectedCalls {
		t.Fatalf("mint calls=%d want %d", calls.Load(), expectedCalls)
	}
}
