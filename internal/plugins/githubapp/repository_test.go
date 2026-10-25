package githubapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubRemoteSupportsSupportedForms(t *testing.T) {
	cases := []struct{ raw, want string }{{"https://github.com/Acme/widget.git", "Acme/widget"}, {"ssh://git@github.com/Acme/widget.git", "Acme/widget"}, {"git@github.com:Acme/widget.git", "Acme/widget"}}
	for _, tc := range cases {
		got, err := ParseGitHubRemote(tc.raw)
		if err != nil || got != tc.want {
			t.Fatalf("ParseGitHubRemote(%q) = %q, %v", tc.raw, got, err)
		}
	}
}

func TestParseGitHubRemoteRejectsCredentialsHostAndTraversal(t *testing.T) {
	for _, raw := range []string{"https://user:pass@github.com/a/b", "https://evil.test/a/b", "https://github.com/a/../b", "https://github.com/a/b?x=y", "git@github.com:a/b#frag"} {
		if _, err := ParseGitHubRemote(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestReadGitRemotesDoesNotInvokeNetworkCommands(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, "git")
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + log + "\"\ncase \"$1 $2\" in 'remote ') echo origin;; 'remote get-url') echo git@github.com:acme/widget.git;; esac\n"
	if err := os.WriteFile(git, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	remotes, err := ReadGitRemotes(context.Background(), dir, git)
	if err != nil || len(remotes) != 1 || remotes[0].Repository != "acme/widget" {
		t.Fatalf("got %#v %v", remotes, err)
	}
	calls, _ := os.ReadFile(log)
	if string(calls) != "remote\nremote get-url -- origin\n" {
		t.Fatalf("unexpected commands %q", calls)
	}
}

func TestLookupRepositoryUsesAnonymousRESTAndRejectsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("anonymous request carried credentials")
		}
		w.Header().Set("Location", "https://github.com/other/repo")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()
	_, err := LookupRepository(context.Background(), "acme", "widget", RepositoryOptions{APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err == nil {
		t.Fatal("redirect accepted")
	}
}

func TestVerifyRepositoryScopeRejectsAdditionalRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user/installations":
			w.Write([]byte(`{"installations":[{"id":3}]}`))
		case "/user/installations/3/repositories":
			w.Write([]byte(`{"total_count":2,"repositories":[{"id":1,"full_name":"a/b"},{"id":2,"full_name":"a/c"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	err := VerifyRepositoryScope(context.Background(), "secret", Repository{ID: 1, FullName: "a/b"}, RepositoryOptions{APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err == nil {
		t.Fatal("scope expansion accepted")
	}
}

func TestLookupRepositoryRejectsDifferentRepositoryIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9,"full_name":"other/repo"}`))
	}))
	defer server.Close()
	_, err := LookupRepository(context.Background(), "acme", "widget", RepositoryOptions{APIBaseURL: server.URL, GHPath: filepath.Join(t.TempDir(), "missing-gh")})
	if err == nil {
		t.Fatal("accepted a different repository")
	}
}

func TestRepositoryScopeRejectsIncompletePagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/user/installations" {
			_, _ = w.Write([]byte(`{"total_count":1,"installations":[{"id":3}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"total_count":2,"repositories":[{"id":1,"full_name":"a/b"}]}`))
	}))
	defer server.Close()
	err := VerifyRepositoryScope(context.Background(), "secret", Repository{ID: 1, FullName: "a/b"}, RepositoryOptions{APIBaseURL: server.URL})
	if err == nil {
		t.Fatal("incomplete repository enumeration accepted")
	}
}

func TestRepositoryClientDoesNotFollowRedirectWithToken(t *testing.T) {
	leaked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/installations" {
			http.Redirect(w, r, "/leak", http.StatusFound)
			return
		}
		leaked = r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":0,"installations":[]}`))
	}))
	defer server.Close()
	_, err := ListAccessibleRepositories(context.Background(), "secret", RepositoryOptions{APIBaseURL: server.URL})
	if err == nil || leaked {
		t.Fatalf("redirect result err=%v, token leaked=%v", err, leaked)
	}
}

func TestReadGitRemotesRejectsOversizedOutput(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\nhead -c 1100000 /dev/zero\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadGitRemotes(context.Background(), dir, git); err == nil {
		t.Fatal("accepted oversized git output")
	}
}
