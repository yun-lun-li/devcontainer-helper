package githubapp

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerServesCredentialAndHealthOnlyOverUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "credential.sock")
	if err := os.Chmod(filepath.Dir(sock), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ClientID: "Iv1_example", ProjectPath: t.TempDir(), UserID: 2, Login: "tester", RepositoryID: 3, Repository: "octo/repo"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Serve(ctx, sock, cfg) }()
	waitForSocket(t, sock)
	socketInfo, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if socketInfo.Mode().Perm() != 0600 {
		t.Fatalf("socket mode = %o", socketInfo.Mode().Perm())
	}
	client := unixHTTPClient(sock)
	resp, err := client.Get("http://unix/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	resp, err = client.Get("http://unix/credential")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
		t.Fatalf("credential response status=%d headers=%v body=%q", resp.StatusCode, resp.Header, data)
	}
	resp, err = client.Get("http://unix/other")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other path status = %d", resp.StatusCode)
	}
	request, _ := http.NewRequest(http.MethodPost, "http://unix/health", nil)
	resp, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("health POST status = %d", resp.StatusCode)
	}
	if err := Serve(context.Background(), sock, cfg); err == nil {
		t.Fatal("second server acquired the active socket")
	}
	cancel()
	waitForSocketRemoval(t, sock)
}

func TestServerRecoversStaleSocketAfterCrash(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "credential.sock")
	_ = os.Chmod(filepath.Dir(sock), 0700)
	stale, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ClientID: "Iv1_example", ProjectPath: t.TempDir(), UserID: 2, Login: "tester", RepositoryID: 3, Repository: "octo/repo"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Serve(ctx, sock, cfg) }()
	waitForHealth(t, sock)
	cancel()
	waitForSocketRemoval(t, sock)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDirUsesPrivateProjectSpecificDirectory(t *testing.T) {
	runtime := t.TempDir()
	if err := os.Chmod(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	a, err := RuntimeDir("/workspace/one")
	if err != nil {
		t.Fatal(err)
	}
	b, err := RuntimeDir("/workspace/two")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !strings.HasPrefix(a, runtime) {
		t.Fatalf("unexpected paths %q %q", a, b)
	}
	info, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestCheckCredentialUsesUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "health.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/credential" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("token"))
	})}
	go server.Serve(listener)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := CheckCredential(ctx, sock); err != nil {
		t.Fatal(err)
	}
}

func TestPATCredentialFailureGuidesRecoveryWithoutLeakingAPIError(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sensitive GitHub detail", http.StatusUnauthorized)
	}))
	defer api.Close()
	cfg := Config{AuthMethod: authMethodPAT, ProjectPath: "/tmp/project", UserID: 8, Login: "tester", RepositoryID: 9, Repository: "octo/repo"}
	set := TokenSet{Schema: 2, AuthMethod: authMethodPAT, ProjectPath: cfg.ProjectPath, UserID: cfg.UserID, RepositoryID: cfg.RepositoryID, Repository: cfg.Repository, PersonalAccessToken: "github_pat_private_value"}
	secretPath := filepath.Join(t.TempDir(), "secret.json")
	data, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	pass := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(pass, []byte("#!/bin/sh\ncat \"$PASS_SECRET\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASS_SECRET", secretPath)
	provider, err := NewProvider(cfg, ProviderOptions{PassPath: pass, APIBaseURL: api.URL, Secret: &SecretStore{PassPath: pass, Entry: "entry", LockDir: filepath.Join(t.TempDir(), "locks")}})
	if err != nil {
		t.Fatal(err)
	}
	service := httptest.NewServer(credentialMux(provider))
	defer service.Close()
	response, err := service.Client().Get(service.URL + "/credential")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	message := string(body)
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(message, "reconfigure the PAT in host Project Plugins") || !strings.Contains(message, "unlock host GPG") {
		t.Fatalf("PAT failure response = %d %q", response.StatusCode, message)
	}
	if strings.Contains(message, "sensitive GitHub detail") || strings.Contains(message, "github_pat_private_value") {
		t.Fatalf("PAT failure leaked secret or API detail: %q", message)
	}
}

func TestUnixCredentialClientAllowsPinentryBudget(t *testing.T) {
	client := unixHTTPClient("/tmp/credential.sock")
	if client.Timeout != 125*time.Second {
		t.Fatalf("unix credential client timeout = %s", client.Timeout)
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("socket did not appear")
}

func waitForSocketRemoval(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("socket was not removed")
}

func waitForHealth(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	client := unixHTTPClient(path)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://unix/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("credential server did not become healthy")
}
