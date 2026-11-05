package hostpass

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func sharedRequest(ctx context.Context, dir, method, path string, config registration) error {
	if config.Project != "" {
		project, err := filepath.Abs(config.Project)
		if err != nil {
			return err
		}
		config.Project = project
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	response, err := unixClient(filepath.Join(dir, "control.sock")).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("host pass control request: %s", bytes.TrimSpace(message))
	}
	return nil
}

func sharedReady(ctx context.Context, dir string) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/health", nil)
	if err != nil {
		return false
	}
	response, err := unixClient(filepath.Join(dir, "control.sock")).Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 128))
	return err == nil && response.StatusCode == http.StatusOK && string(data) == sharedProtocol
}

func registerShared(ctx context.Context, project, entry string) error {
	dir, err := sharedDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	unlock, err := lockRuntime(ctx, filepath.Join(dir, "start.lock"), true)
	if err != nil {
		return err
	}
	defer unlock()
	if !sharedReady(ctx, dir) {
		if err := startSharedService(); err != nil {
			return err
		}
		if err := waitForShared(ctx, dir); err != nil {
			return err
		}
	}
	executable, err := exec.LookPath("pass")
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	return sharedRequest(ctx, dir, "POST", "/register", registration{
		Project: project, Entry: entry, Executable: executable, Environment: passEnvironment(), Directory: directory,
	})
}

func waitForShared(ctx context.Context, dir string) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if sharedReady(ctx, dir) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("shared host pass service did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func unregisterShared(ctx context.Context, project string) error {
	dir, err := sharedDirectory()
	if err != nil {
		return err
	}
	if !sharedReady(ctx, dir) {
		return fmt.Errorf("shared host pass service is not running")
	}
	return sharedRequest(ctx, dir, "POST", "/unregister", registration{Project: project})
}
