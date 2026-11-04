package hostpass

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func runtimeDirectory(project string) (string, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(project, ".devcontainer", "devcontainer.json")); err != nil {
		return "", fmt.Errorf("project requires .devcontainer/devcontainer.json: %w", err)
	}
	path := filepath.Join(project, ".devcontainer", ".dch", "github-auth")
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	if err := privatePath(path, true); err != nil {
		return "", err
	}
	return path, nil
}

func privatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0077 != 0 || info.IsDir() != directory || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be owned by the host user with private permissions", path)
	}
	return nil
}

func lockRuntime(ctx context.Context, path string, wait bool) (func(), error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := privatePath(path, false); err != nil {
		file.Close()
		return nil, err
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = file.Close() }, nil
		}
		if !wait || err != syscall.EWOULDBLOCK {
			file.Close()
			return nil, fmt.Errorf("host pass service is busy: %w", err)
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func socketClient(path string) *http.Client {
	return &http.Client{Timeout: requestTimeout + 5*time.Second, Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(path, "pass.sock"))
		},
	}}
}
