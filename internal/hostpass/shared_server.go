package hostpass

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const sharedProtocol = "dch-host-pass-v1"
const sharedIdleTimeout = 30 * time.Second

type registration struct {
	Project     string
	Entry       string
	Executable  string
	Environment []string
	Directory   string
}

type projectListener struct {
	entry string
	close func()
}

type sharedService struct {
	mu        sync.Mutex
	projects  map[string]projectListener
	gate      chan struct{}
	idleSince time.Time
	closed    bool
}

// ServeShared serves project-specific sockets from one process for the host user.
// It exits after thirty seconds without registered projects.
func ServeShared(ctx context.Context) error {
	dir, err := sharedDirectory()
	if err != nil {
		return err
	}
	stopped := make(chan struct{})
	defer close(stopped)
	unlock, err := lockRuntime(ctx, filepath.Join(dir, "serve.lock"), false)
	if err != nil {
		return err
	}
	defer unlock()
	listener, err := privateListener(filepath.Join(dir, "control.sock"))
	if err != nil {
		return err
	}
	defer listener.Close()
	service := &sharedService{projects: map[string]projectListener{}, gate: make(chan struct{}, 1), idleSince: time.Now()}
	defer service.close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{Handler: service.controlHandler(ctx), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 4096}
	go service.watchIdle(ctx, cancel, dir, stopped)
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func privateListener(path string) (net.Listener, error) {
	if err := removeSocket(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func (s *sharedService) controlHandler(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-DCH-PID", strconv.Itoa(os.Getpid()))
		fmt.Fprint(w, sharedProtocol)
	})
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		var config registration
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&config); err != nil {
			http.Error(w, "invalid project registration", http.StatusBadRequest)
			return
		}
		if err := s.register(ctx, config); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /unregister", func(w http.ResponseWriter, r *http.Request) {
		var config registration
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&config); err != nil {
			http.Error(w, "invalid project registration", http.StatusBadRequest)
			return
		}
		if err := s.unregister(config.Project); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func (s *sharedService) register(ctx context.Context, config registration) error {
	if err := validateEntry(config.Entry); err != nil {
		return err
	}
	if !filepath.IsAbs(config.Executable) {
		return fmt.Errorf("pass executable must be absolute")
	}
	dir, err := runtimeDirectory(config.Project)
	if err != nil {
		return err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil {
		return fmt.Errorf("shared host pass service is stopping; retry prepare")
	}
	if active, exists := s.projects[dir]; exists {
		if active.entry != config.Entry {
			return fmt.Errorf("another pass entry is active; stop the project service first")
		}
		return nil
	}
	project, err := s.listenProject(ctx, dir, config)
	if err != nil {
		return err
	}
	s.projects[dir] = project
	return nil
}

func (s *sharedService) listenProject(ctx context.Context, dir string, config registration) (projectListener, error) {
	unlock, err := lockRuntime(ctx, filepath.Join(dir, "serve.lock"), false)
	if err != nil {
		return projectListener{}, err
	}
	listener, err := privateListener(filepath.Join(dir, "pass.sock"))
	if err != nil {
		unlock()
		return projectListener{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	server := s.projectServer(ctx, config)
	go func() { _ = server.Serve(listener) }()
	return projectListener{entry: config.Entry, close: func() {
		cancel()
		_ = server.Close()
		_ = listener.Close()
		unlock()
	}}, nil
}

func (s *sharedService) projectServer(ctx context.Context, config registration) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, entryIdentity(config.Entry))
	})
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		serveToken(w, r, s.gate, func(ctx context.Context) (string, error) {
			return readTokenUsing(ctx, config.Entry, config.Executable, config.Environment, config.Directory)
		})
	})
	return &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: requestTimeout + 5*time.Second,
		MaxHeaderBytes: 4096, BaseContext: func(net.Listener) context.Context { return ctx }}
}

func (s *sharedService) unregister(project string) error {
	dir, err := runtimeDirectory(project)
	if err != nil {
		return err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if active, exists := s.projects[dir]; exists {
		active.close()
		delete(s.projects, dir)
		if len(s.projects) == 0 {
			s.idleSince = time.Now()
		}
	}
	return nil
}

func (s *sharedService) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for dir, project := range s.projects {
		project.close()
		delete(s.projects, dir)
	}
}

func (s *sharedService) watchIdle(ctx context.Context, cancel context.CancelFunc, dir string, stopped <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if s.stopWhenIdle(ctx, cancel, dir, stopped, now) {
				return
			}
		}
	}
}

func (s *sharedService) stopWhenIdle(ctx context.Context, cancel context.CancelFunc, dir string, stopped <-chan struct{}, now time.Time) bool {
	// Serialize retirement with prepare so a new registration cannot be lost.
	unlock, err := lockRuntime(ctx, filepath.Join(dir, "start.lock"), false)
	if err != nil {
		return false
	}
	defer unlock()
	s.mu.Lock()
	idle := len(s.projects) == 0 && now.Sub(s.idleSince) >= sharedIdleTimeout
	if idle {
		cancel()
	}
	s.mu.Unlock()
	if idle {
		<-stopped
	}
	return idle
}
