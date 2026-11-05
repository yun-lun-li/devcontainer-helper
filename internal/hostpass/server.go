package hostpass

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func entryIdentity(entry string) string {
	sum := sha256.Sum256([]byte(entry))
	return hex.EncodeToString(sum[:])
}

// Serve provides only the specified pass entry until canceled or explicitly stopped.
func Serve(ctx context.Context, project, entry string) error {
	if err := validateEntry(entry); err != nil {
		return err
	}
	dir, err := runtimeDirectory(project)
	if err != nil {
		return err
	}
	unlock, err := lockRuntime(ctx, filepath.Join(dir, "serve.lock"), false)
	if err != nil {
		return err
	}
	defer unlock()
	socket := filepath.Join(dir, "pass.sock")
	if err := removeSocket(socket); err != nil {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := newServer(ctx, entry, cancel)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-done:
		}
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func removeSocket(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := privatePath(path, false); err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to replace a non-socket at %s", path)
	}
	return os.Remove(path)
}

func newServer(ctx context.Context, entry string, cancel context.CancelFunc) *http.Server {
	gate := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, entryIdentity(entry))
	})
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		serveToken(w, r, gate, func(ctx context.Context) (string, error) { return readToken(ctx, entry) })
	})
	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.(http.Flusher).Flush()
		cancel()
	})
	return &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: requestTimeout + 5*time.Second,
		MaxHeaderBytes: 4096, BaseContext: func(net.Listener) context.Context { return ctx }}
}

func serveToken(w http.ResponseWriter, r *http.Request, gate chan struct{}, read func(context.Context) (string, error)) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		http.Error(w, "host pass request timed out", http.StatusGatewayTimeout)
		return
	}
	token, err := read(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, token)
}
