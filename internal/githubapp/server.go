package githubapp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Serve(ctx context.Context, socketPath string, cfg Config) error {
	listener, provider, releaseLock, err := prepareCredentialSocket(socketPath, cfg)
	if err != nil {
		return err
	}
	defer releaseLock()
	server := &http.Server{Handler: credentialMux(provider), ReadHeaderTimeout: 5 * time.Second}
	return serveUntilCancelled(ctx, server, listener, socketPath)
}

func prepareCredentialSocket(socketPath string, cfg Config) (net.Listener, *Provider, func(), error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, nil, err
	}
	if len(socketPath) > 100 {
		return nil, nil, nil, fmt.Errorf("socket path is too long")
	}
	parentInfo, err := os.Lstat(filepath.Dir(socketPath))
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(parentInfo) || parentInfo.Mode().Perm() != 0700 {
		return nil, nil, nil, fmt.Errorf("socket directory is unsafe")
	}
	releaseLock, err := acquireServerLock(socketPath + ".lock")
	if err != nil {
		return nil, nil, nil, err
	}
	listener, provider, err := listenCredentialSocket(socketPath, cfg)
	if err != nil {
		releaseLock()
		return nil, nil, nil, err
	}
	return listener, provider, releaseLock, nil
}

func listenCredentialSocket(socketPath string, cfg Config) (net.Listener, *Provider, error) {
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, nil, fmt.Errorf("socket path is unsafe")
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, nil, fmt.Errorf("cannot remove stale socket")
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot listen on credential socket")
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		listener.Close()
		os.Remove(socketPath)
		return nil, nil, fmt.Errorf("cannot secure credential socket")
	}
	provider, err := NewProvider(cfg)
	if err != nil {
		listener.Close()
		os.Remove(socketPath)
		return nil, nil, err
	}
	return listener, provider, nil
}

func credentialMux(provider *Provider) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/credential", credentialHandler(provider))
	return mux
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func credentialHandler(provider *Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token, err := provider.Token(r.Context())
		if err != nil {
			http.Error(w, "credential unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, token)
	}
}

func serveUntilCancelled(ctx context.Context, server *http.Server, listener net.Listener, socketPath string) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-done:
		}
	}()
	err := server.Serve(listener)
	close(done)
	_ = os.Remove(socketPath)
	if err == http.ErrServerClosed {
		return nil
	}
	return fmt.Errorf("credential server failed")
}

func acquireServerLock(path string) (func(), error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot open credential server lock")
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
		file.Close()
		return nil, fmt.Errorf("credential server lock is unsafe")
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, fmt.Errorf("cannot secure credential server lock")
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("credential server is already running")
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = file.Close() }, nil
}

func CheckCredential(ctx context.Context, socketPath string) error {
	client := unixHTTPClient(socketPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/credential", nil)
	if err != nil {
		return fmt.Errorf("cannot check credential service")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("credential service unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("credential service rejected check")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxResponseBytes || strings.ContainsAny(string(data), "\r\n") {
		return fmt.Errorf("credential service returned invalid token")
	}
	return nil
}

func unixHTTPClient(path string) *http.Client {
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
