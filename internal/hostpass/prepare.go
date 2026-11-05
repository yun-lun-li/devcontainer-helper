package hostpass

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Prepare stores a missing PAT through pass's hidden prompt and ensures a detached
// service is ready. It does not change an existing entry or initialize GPG/pass.
func Prepare(ctx context.Context, project, entry string, stdin io.Reader, stdout, stderr io.Writer) error {
	if err := validateEntry(entry); err != nil {
		return err
	}
	if _, err := exec.LookPath("pass"); err != nil {
		return fmt.Errorf("install and initialize pass and GPG on the host first")
	}
	dir, err := runtimeDirectory(project)
	if err != nil {
		return err
	}
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	unlock, err := lockRuntime(lockCtx, filepath.Join(dir, "prepare.lock"), true)
	if err != nil {
		return err
	}
	defer unlock()
	identity, running := serviceIdentity(ctx, dir)
	if running && identity != entryIdentity(entry) {
		return fmt.Errorf("another pass entry is active; run dch host-pass stop --project PATH before changing entries")
	}
	if err := ensureToken(ctx, entry, stdin, stdout, stderr); err != nil {
		return err
	}
	if running {
		return nil
	}
	if err := startService(project, entry); err != nil {
		return err
	}
	return waitForService(ctx, dir, entry)
}

func startService(project, entry string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	sink, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer sink.Close()
	cmd := exec.Command(executable, "host-pass", "serve", "--project", project, "--entry", entry)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = sink, sink, sink
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start host pass service: %w", err)
	}
	return cmd.Process.Release()
}

func serviceIdentity(ctx context.Context, dir string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/health", nil)
	if err != nil {
		return "", false
	}
	response, err := socketClient(dir).Do(request)
	if err != nil {
		return "", false
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65))
	return string(data), err == nil && response.StatusCode == http.StatusOK && len(data) == 64
}

func waitForService(ctx context.Context, dir, entry string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if identity, ok := serviceIdentity(ctx, dir); ok && identity == entryIdentity(entry) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("host pass service did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// Stop shuts down the project's pass service. It leaves the encrypted PAT in pass.
func Stop(ctx context.Context, project string) error {
	dir, err := runtimeDirectory(project)
	if err != nil {
		return err
	}
	if _, ok := serviceIdentity(ctx, dir); !ok {
		return fmt.Errorf("host pass service is not running")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/shutdown", nil)
	if err != nil {
		return err
	}
	response, err := socketClient(dir).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("could not stop host pass service")
	}
	return nil
}

// StopIfRunning stops an active project service and succeeds when none is running.
func StopIfRunning(ctx context.Context, project string) error {
	dir, err := runtimeDirectory(project)
	if err != nil {
		return err
	}
	if _, running := serviceIdentity(ctx, dir); !running {
		return nil
	}
	return Stop(ctx, project)
}
