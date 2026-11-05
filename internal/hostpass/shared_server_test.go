package hostpass

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sharedTestRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "dch-shared-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("XDG_RUNTIME_DIR", root)
	return root
}

func sharedTestProject(t *testing.T, root, name string) string {
	t.Helper()
	project := filepath.Join(root, name)
	dir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	return project
}

func runSharedTestServer(t *testing.T) (context.Context, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeShared(ctx) }()
	dir, err := sharedDirectory()
	if err != nil {
		t.Fatal(err)
	}
	readyCtx, readyCancel := context.WithTimeout(ctx, 3*time.Second)
	defer readyCancel()
	if err := waitForShared(readyCtx, dir); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("shared server did not stop")
		}
	})
	return ctx, dir
}

func projectResponse(t *testing.T, project, method, path string) (int, string) {
	t.Helper()
	dir, err := runtimeDirectory(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := socketClient(dir).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(data)
}

func registerTestProject(t *testing.T, ctx context.Context, dir, project, entry, executable, store string) {
	t.Helper()
	config := registration{Project: project, Entry: entry, Executable: executable, Environment: []string{"PASSWORD_STORE_DIR=" + store}}
	if err := sharedRequest(ctx, dir, "POST", "/register", config); err != nil {
		t.Fatal(err)
	}
}

func TestSharedServerSeparatesEntriesAndProjectEnvironments(t *testing.T) {
	root := sharedTestRoot(t)
	first := sharedTestProject(t, root, "a")
	second := sharedTestProject(t, root, "b")
	executable := filepath.Join(root, "pass")
	script := "#!/bin/sh\nprintf 'ghp_%s_%s' \"$3\" \"$PASSWORD_STORE_DIR\"\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, dir := runSharedTestServer(t)
	registerTestProject(t, ctx, dir, first, "first", executable, "storeA")
	registerTestProject(t, ctx, dir, second, "second", executable, "storeB")
	assertProjectToken(t, first, "/token?entry=second", "ghp_first_storeA")
	assertProjectToken(t, second, "/token", "ghp_second_storeB")
	if err := ServeShared(ctx); err == nil {
		t.Fatal("started a second shared server")
	}
	assertProjectToken(t, first, "/token", "ghp_first_storeA")
}

func assertProjectToken(t *testing.T, project, path, want string) {
	t.Helper()
	status, token := projectResponse(t, project, "GET", path)
	if status != http.StatusOK || token != want {
		t.Fatalf("unexpected token response: status %d", status)
	}
}

func TestSharedProjectCannotControlRegistrations(t *testing.T) {
	root := sharedTestRoot(t)
	project := sharedTestProject(t, root, "a")
	ctx, dir := runSharedTestServer(t)
	registerTestProject(t, ctx, dir, project, "entry", "/bin/false", "store")
	for _, path := range []string{"/register", "/unregister", "/shutdown"} {
		status, _ := projectResponse(t, project, "POST", path)
		if status != http.StatusNotFound {
			t.Fatalf("exposed control route %s", path)
		}
	}
}

func TestSharedStopOnlyUnregistersRequestedProject(t *testing.T) {
	root := sharedTestRoot(t)
	first := sharedTestProject(t, root, "a")
	second := sharedTestProject(t, root, "b")
	ctx, dir := runSharedTestServer(t)
	registerTestProject(t, ctx, dir, first, "first", "/bin/false", "storeA")
	registerTestProject(t, ctx, dir, second, "second", "/bin/false", "storeB")
	if err := sharedRequest(ctx, dir, "POST", "/register", registration{Project: first, Entry: "other", Executable: "/bin/false"}); err == nil {
		t.Fatal("changed an active project's entry")
	}
	if err := Stop(ctx, first); err != nil {
		t.Fatal(err)
	}
	assertProjectSocketAbsent(t, first)
	status, _ := projectResponse(t, second, "GET", "/health")
	if status != http.StatusOK || !sharedReady(ctx, dir) {
		t.Fatal("stopping first project disrupted shared server")
	}
	registerTestProject(t, ctx, dir, first, "other", "/bin/false", "storeA")
	if err := StopIfRunning(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := StopIfRunning(ctx, first); err != nil {
		t.Fatal(err)
	}
}

func assertProjectSocketAbsent(t *testing.T, project string) {
	t.Helper()
	path := filepath.Join(project, ".devcontainer", ".dch", "github-auth", "pass.sock")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket remains: %v", err)
	}
}

func TestIdleRetirementSerializesWithPrepare(t *testing.T) {
	sharedTestRoot(t)
	dir, err := sharedDirectory()
	if err != nil {
		t.Fatal(err)
	}
	s := &sharedService{projects: map[string]projectListener{}, idleSince: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	done := make(chan bool, 1)
	go func() { done <- s.stopWhenIdle(ctx, cancel, dir, stopped, time.Now().Add(sharedIdleTimeout)) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("idle server did not retire")
	}
	unlock, err := lockRuntime(context.Background(), filepath.Join(dir, "start.lock"), false)
	if err == nil {
		unlock()
		t.Error("prepare could race with retirement")
	}
	close(stopped)
	if !<-done {
		t.Fatal("server did not retire")
	}
	unlock, err = lockRuntime(context.Background(), filepath.Join(dir, "start.lock"), false)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestSharedServerRejectsRegistrationAfterShutdown(t *testing.T) {
	root := sharedTestRoot(t)
	project := sharedTestProject(t, root, "a")
	s := &sharedService{projects: map[string]projectListener{}, gate: make(chan struct{}, 1)}
	s.close()
	if err := s.register(context.Background(), registration{Project: project, Entry: "entry", Executable: "/bin/false"}); err == nil {
		t.Fatal("registered a project after shutdown")
	}
	assertProjectSocketAbsent(t, project)
}

func TestSharedUnregisterCancelsPassAndReleasesOtherProjects(t *testing.T) {
	root := sharedTestRoot(t)
	slow := sharedTestProject(t, root, "slow")
	fast := sharedTestProject(t, root, "fast")
	executable := filepath.Join(root, "pass")
	flag := filepath.Join(root, "waiting")
	script := "#!/bin/sh\nif [ \"$3\" = slow ]; then\n /bin/touch \"$PASSWORD_STORE_DIR\"\n /bin/sleep 30 &\n wait\nelse\n printf ghp_FAST\nfi\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, dir := runSharedTestServer(t)
	registerTestProject(t, ctx, dir, slow, "slow", executable, flag)
	registerTestProject(t, ctx, dir, fast, "fast", executable, "unused")
	done := requestSlowProject(t, slow)
	awaitSharedFile(t, flag)
	if err := Stop(ctx, slow); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("unregister left token request running")
	}
	assertProjectToken(t, fast, "/token", "ghp_FAST")
}

func requestSlowProject(t *testing.T, project string) <-chan struct{} {
	t.Helper()
	dir, err := runtimeDirectory(project)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, _ := socketClient(dir).Get("http://localhost/token")
		if response != nil {
			response.Body.Close()
		}
	}()
	return done
}

func awaitSharedFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("pass command did not start")
}
