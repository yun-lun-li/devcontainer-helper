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
	privateKey, err := parsePrivateKey([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2029, 12, 31, 0, 0, 0, 0, time.UTC)
	var tokenCalls, repoCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/34/access_tokens" {
			tokenCalls++
			jwt := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			parts := strings.Split(jwt, ".")
			if len(parts) != 3 {
				t.Fatalf("invalid JWT: %q", jwt)
			}
			headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
			var header map[string]string
			_ = json.Unmarshal(headerJSON, &header)
			if header["alg"] != "RS256" || header["typ"] != "JWT" {
				t.Errorf("unexpected JWT header: %#v", header)
			}
			unsigned := parts[0] + "." + parts[1]
			digest := sha256.Sum256([]byte(unsigned))
			sig, decodeErr := base64.RawURLEncoding.DecodeString(parts[2])
			if decodeErr != nil || rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest[:], sig) != nil {
				t.Error("JWT signature did not verify")
			}
			claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims map[string]any
			_ = json.Unmarshal(claimsJSON, &claims)
			if claims["iss"] != "12" || claims["iat"] != float64(now.Unix()-60) || claims["exp"] != float64(now.Unix()+9*60) {
				t.Errorf("unexpected claims: %#v", claims)
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["repositories"].([]any)[0] != "repo" || body["permissions"].(map[string]any)["contents"] != "read" {
				t.Errorf("unexpected scope: %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"opaque-token","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		if r.URL.Path == "/repos/octo/repo" {
			repoCalls++
			if r.Header.Get("Authorization") != "Bearer opaque-token" {
				t.Errorf("missing scoped token")
			}
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
		t.Fatalf("calls token=%d repo=%d", tokenCalls, repoCalls)
	}
}

func TestProviderRejectsMalformedOrExpiredResponsesAndRetries(t *testing.T) {
	cases := []struct{ name, response string }{{"malformed", `{"token":`}, {"expired", `{"token":"old-token","expires_at":"2000-01-01T00:00:00Z"}`}, {"invalid-expiry", `{"token":"bad-expiry","expires_at":"tomorrow"}`}, {"invalid-token", `{"token":"bad\ntoken","expires_at":"2030-01-01T00:00:00Z"}`}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := testPrivateKey(t)
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/app/installations/2/access_tokens" {
					if calls.Add(1) == 1 {
						_, _ = w.Write([]byte(tc.response))
						return
					}
					_, _ = w.Write([]byte(`{"token":"retry-token","expires_at":"2030-01-01T00:00:00Z"}`))
					return
				}
				_, _ = w.Write([]byte(`{"full_name":"octo/repo"}`))
			}))
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
		})
	}
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
		_, _ = w.Write([]byte(`{"full_name":"octo/repo"}`))
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
		_, _ = w.Write([]byte(`{"full_name":"octo/repo"}`))
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
		_, _ = w.Write([]byte(`{"full_name":"octo/repo"}`))
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
		_, _ = w.Write([]byte(`{"full_name":"octo/repo"}`))
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
