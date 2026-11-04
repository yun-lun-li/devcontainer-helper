package hostpass_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dch/internal/hostpass"
)

const fakePAT = "github_pat_TEST_ONLY_not_a_real_token"

func TestServiceProvidesOnlyConfiguredPATAndReadsChanges(t *testing.T) {
	project, tokenPath := setupHost(t)
	cancel, done := startService(t, project, "github/project")
	defer finishService(t, cancel, done)
	client := serviceClient(project)
	response := request(t, client, "GET", "/token")
	if response != fakePAT {
		t.Fatalf("unexpected token response length %d", len(response))
	}
	if err := os.WriteFile(tokenPath, []byte("ghp_REPLACEMENT_TEST_ONLY\nnotes are not credentials\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if response := request(t, client, "GET", "/token"); response != "ghp_REPLACEMENT_TEST_ONLY" {
		t.Fatal("service did not reread pass")
	}
	args, err := os.ReadFile(filepath.Join(project, "pass-args"))
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "show\n--\ngithub/project\n" {
		t.Fatalf("pass arguments = %q", args)
	}
	req, _ := http.NewRequest("GET", "http://localhost/other-entry", nil)
	result, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusNotFound {
		t.Fatalf("other entry status = %d", result.StatusCode)
	}
}

func TestServiceHidesPassErrorsAndRejectsInvalidTokens(t *testing.T) {
	project, tokenPath := setupHost(t)
	cancel, done := startService(t, project, "github/project")
	defer finishService(t, cancel, done)
	for _, contents := range []string{"not-a-token\n", "FAIL"} {
		if err := os.WriteFile(tokenPath, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest("GET", "http://localhost/token", nil)
		result, err := serviceClient(project).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(result.Body)
		result.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if result.StatusCode != http.StatusServiceUnavailable || strings.Contains(string(data), fakePAT) {
			t.Fatalf("unsafe error response: status %d", result.StatusCode)
		}
	}
}

func TestServiceStopsAndRemovesSocket(t *testing.T) {
	project, _ := setupHost(t)
	cancel, done := startService(t, project, "github/project")
	defer cancel()
	if err := hostpass.Stop(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	awaitService(t, done)
	if _, err := os.Stat(socketPath(project)); !os.IsNotExist(err) {
		t.Fatalf("socket remains: %v", err)
	}
}

func TestServiceRejectsUnsafeEntryAndSocketPaths(t *testing.T) {
	project, _ := setupHost(t)
	for _, entry := range []string{"../other", "/absolute", "-option", "line\nbreak"} {
		if err := hostpass.Serve(context.Background(), project, entry); err == nil {
			t.Fatalf("accepted unsafe entry %q", entry)
		}
	}
	dir := filepath.Dir(socketPath(project))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(socketPath(project), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := hostpass.Serve(context.Background(), project, "github/project"); err == nil {
		t.Fatal("replaced a regular file")
	}
	data, err := os.ReadFile(socketPath(project))
	if err != nil || string(data) != "preserve" {
		t.Fatal("existing file changed")
	}
}

func TestServiceRejectsDuplicateAndInsecureRuntime(t *testing.T) {
	project, _ := setupHost(t)
	cancel, done := startService(t, project, "github/project")
	defer finishService(t, cancel, done)
	if err := hostpass.Serve(context.Background(), project, "other"); err == nil {
		t.Fatal("second service replaced active socket")
	}
	if got := request(t, serviceClient(project), "GET", "/token"); got != fakePAT {
		t.Fatal("active service was disturbed")
	}
	if err := os.Chmod(filepath.Dir(socketPath(project)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := hostpass.Prepare(context.Background(), project, "github/project", nil, io.Discard, io.Discard); err == nil {
		t.Fatal("accepted insecure runtime")
	}
}

func TestPrepareReusesServiceAndDoesNotExposePAT(t *testing.T) {
	project, _ := setupHost(t)
	cancel, done := startService(t, project, "github/project")
	defer finishService(t, cancel, done)
	if err := hostpass.Prepare(context.Background(), project, "github/project", nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := hostpass.Prepare(context.Background(), project, "github/other", nil, io.Discard, io.Discard); err == nil {
		t.Fatal("silently switched service entry")
	}
	if got := request(t, serviceClient(project), "GET", "/health"); strings.Contains(got, fakePAT) || len(got) != 64 {
		t.Fatal("health response exposed credential")
	}
}

func setupHost(t *testing.T) (string, string) {
	t.Helper()
	project, err := os.MkdirTemp("", "dch-pass-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(project) })
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(project, "token-fixture")
	if err := os.WriteFile(tokenPath, []byte(fakePAT+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\n' "$@" > "$DCH_TEST_PROJECT/pass-args"
case "$(cat "$DCH_TEST_TOKEN")" in
 FAIL) printf '%s' 'github_pat_TEST_ONLY_not_a_real_token' >&2; exit 1 ;;
esac
cat "$DCH_TEST_TOKEN"
`
	if err := os.WriteFile(filepath.Join(project, "pass"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(project, "store", "github")
	if err := os.MkdirAll(store, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "project.gpg"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", project+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PASSWORD_STORE_DIR", filepath.Dir(store))
	t.Setenv("DCH_TEST_PROJECT", project)
	t.Setenv("DCH_TEST_TOKEN", tokenPath)
	return project, tokenPath
}

func startService(t *testing.T, project, entry string) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- hostpass.Serve(ctx, project, entry) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath(project)); err == nil {
			return cancel, done
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	t.Fatal("service did not start")
	return nil, nil
}

func finishService(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	awaitService(t, done)
}

func awaitService(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(3 * time.Second):
		t.Error("service did not stop")
	}
}

func socketPath(project string) string {
	return filepath.Join(project, ".devcontainer", ".dch", "github-auth", "pass.sock")
}

func serviceClient(project string) *http.Client {
	return &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath(project))
		},
	}}
}

func request(t *testing.T, client *http.Client, method, path string) string {
	t.Helper()
	req, err := http.NewRequest(method, "http://localhost"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("request status = %d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRequestCancellationStopsPassAndReleasesNextRequest(t *testing.T) {
	project, tokenPath := setupHost(t)
	script := `#!/bin/sh
if [ "$(cat "$DCH_TEST_TOKEN")" = WAIT ]; then
 touch "$DCH_TEST_PROJECT/waiting"
 sleep 30 &
 wait
fi
cat "$DCH_TEST_TOKEN"
`
	if err := os.WriteFile(filepath.Join(project, "pass"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("WAIT"), 0600); err != nil {
		t.Fatal(err)
	}
	cancel, done := startService(t, project, "github/project")
	defer finishService(t, cancel, done)
	ctx, stopRequest := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		response, err := serviceClient(project).Do(req)
		if response != nil {
			response.Body.Close()
		}
		result <- err
	}()
	awaitFile(t, filepath.Join(project, "waiting"))
	stopRequest()
	if err := <-result; err == nil {
		t.Fatal("canceled request succeeded")
	}
	if err := os.WriteFile(tokenPath, []byte(fakePAT), 0600); err != nil {
		t.Fatal(err)
	}
	if got := request(t, serviceClient(project), "GET", "/token"); got != fakePAT {
		t.Fatal("pass request did not recover after cancellation")
	}
}

func TestPrepareStoresMissingEntryThroughPassInsert(t *testing.T) {
	project, _ := setupHost(t)
	cancel, done := startService(t, project, "github/project")
	defer finishService(t, cancel, done)
	store := os.Getenv("PASSWORD_STORE_DIR")
	if err := os.Remove(filepath.Join(store, "github", "project.gpg")); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
 insert) [ "$2" = -- ] && [ "$3" = github/project ] || exit 1
         read -r token
         printf '%s' "$token" > "$DCH_TEST_TOKEN"
         touch "$PASSWORD_STORE_DIR/github/project.gpg"
         touch "$DCH_TEST_PROJECT/inserted"
         ;;
 show) cat "$DCH_TEST_TOKEN" ;;
 *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(project, "pass"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := hostpass.Prepare(context.Background(), project, "github/project", strings.NewReader(fakePAT+"\n"), &output, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), fakePAT) {
		t.Fatal("prepare printed PAT")
	}
	if _, err := os.Stat(filepath.Join(project, "inserted")); err != nil {
		t.Fatal("did not delegate storage to pass insert")
	}
}

func awaitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("command did not start")
}
