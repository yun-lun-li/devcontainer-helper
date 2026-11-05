package hostpass

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestConcurrentPrepareProcessesShareOneDaemon(t *testing.T) {
	root := sharedTestRoot(t)
	binary := buildTestDCH(t, root)
	first := sharedTestProject(t, root, "a")
	second := sharedTestProject(t, root, "b")
	firstEnv := processPassEnvironment(t, root, "a/store", "ghp_FIRST")
	secondEnv := processPassEnvironment(t, root, "b/store", "ghp_SECOND")
	dir, err := sharedDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopTestDaemon(t, dir) })
	results := make(chan error, 2)
	go func() { results <- prepareProcess(binary, first, firstEnv) }()
	go func() { results <- prepareProcess(binary, second, secondEnv) }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	pid := testDaemonPID(t, dir)
	assertProjectToken(t, first, "/token?entry=other", "ghp_FIRST")
	assertProjectToken(t, second, "/token", "ghp_SECOND")
	if err := prepareProcess(binary, first, firstEnv); err != nil {
		t.Fatal(err)
	}
	if testDaemonPID(t, dir) != pid {
		t.Fatal("prepare replaced shared daemon")
	}
	if err := Stop(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	assertProjectSocketAbsent(t, first)
	assertProjectToken(t, second, "/token", "ghp_SECOND")
	stopTestDaemon(t, dir)
	if err := prepareProcess(binary, second, secondEnv); err != nil {
		t.Fatal(err)
	}
	assertProjectToken(t, second, "/token", "ghp_SECOND")
}

func buildTestDCH(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(root, "dch")
	command := exec.Command("go", "build", "-o", binary, "../..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build dch: %v\n%s", err, output)
	}
	return binary
}

func processPassEnvironment(t *testing.T, root, name, token string) []string {
	t.Helper()
	store := filepath.Join(root, name)
	if err := os.MkdirAll(store, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "entry.gpg"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n/bin/cat \"$PASSWORD_STORE_DIR/$3.gpg\"\n"
	if err := os.WriteFile(filepath.Join(bin, "pass"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(), "PASSWORD_STORE_DIR=store", "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func prepareProcess(binary, project string, env []string) error {
	command := exec.Command(binary, "host-pass", "prepare", "--project", project, "--entry", "entry")
	command.Env = env
	command.Dir = project
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("prepare: %w: %s", err, output)
	}
	return nil
}

func testDaemonPID(t *testing.T, dir string) int {
	t.Helper()
	response, err := unixClient(filepath.Join(dir, "control.sock")).Get("http://localhost/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	pid, err := strconv.Atoi(response.Header.Get("X-DCH-PID"))
	if err != nil || pid <= 0 || response.StatusCode != http.StatusOK {
		t.Fatal("invalid daemon health response")
	}
	return pid
}

func stopTestDaemon(t *testing.T, dir string) {
	t.Helper()
	if !sharedReady(context.Background(), dir) {
		return
	}
	pid := testDaemonPID(t, dir)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Error(err)
		return
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "control.sock")); os.IsNotExist(err) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("daemon did not stop")
}
