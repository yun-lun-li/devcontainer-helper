package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type Config struct {
	Version        int                       `json:"version"`
	Projects       []string                  `json:"projects"`
	ProjectPlugins map[string]ProjectPlugins `json:"project_plugins,omitempty"`
}

// ProjectPlugins stores host-side configuration and ownership state for one project.
type ProjectPlugins struct {
	Enabled map[string]json.RawMessage `json:"enabled,omitempty"`
	States  map[string]PluginState     `json:"states,omitempty"`
}

// PluginState stores manager lifecycle state and plugin-owned opaque data.
type PluginState struct {
	BuildPending bool            `json:"build_pending,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
}

func (s *PluginState) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["build_pending"]; ok {
		if err := json.Unmarshal(raw, &s.BuildPending); err != nil {
			return err
		}
		delete(fields, "build_pending")
	}
	if raw, ok := fields["data"]; ok {
		if err := json.Unmarshal(raw, &s.Data); err != nil {
			return err
		}
	} else {
		delete(fields, "build_pending")
		legacy, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			s.Data = legacy
		}
	}
	return nil
}

// ConfigPath returns the absolute path used to load and save the host configuration.
func ConfigPath() (string, error) { return configPath() }

func configPath() (string, error) {
	if p := os.Getenv("DCH_CONFIG"); p != "" {
		return filepath.Abs(p)
	} // useful in tests / isolated deployments
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dch", "config"), nil
}

func LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{Version: 1, Projects: []string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid config %s: %w", path, err)
	}
	if cfg.Version != 1 {
		return cfg, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if cfg.Projects == nil {
		cfg.Projects = []string{}
	}
	return cfg, nil
}

// ReadConfig reads a consistent snapshot while a configuration write is in progress.
func ReadConfig() (Config, error) {
	p, err := configPath()
	if err != nil {
		return Config{}, err
	}
	unlock, err := lockConfigFile(p)
	if err != nil {
		return Config{}, err
	}
	defer unlock()
	return LoadConfig()
}

func SaveConfig(cfg Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	unlock, err := lockConfigFile(p)
	if err != nil {
		return err
	}
	defer unlock()
	return saveConfigAt(p, cfg)
}

func saveConfigAt(p string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(p), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
}

// UpdateConfig serializes read-modify-write operations across processes.
func UpdateConfig(update func(*Config) error) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	unlock, err := lockConfigFile(p)
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if err := update(&cfg); err != nil {
		return err
	}
	return saveConfigAt(p, cfg)
}

func lockConfigFile(configPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return nil, err
	}
	lockPath := configPath + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	info, err := lock.Stat()
	if err != nil {
		lock.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || stat.Uid != uint32(os.Geteuid()) {
		lock.Close()
		return nil, fmt.Errorf("unsafe configuration lock file %s", lockPath)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}, nil
}

func NormalizePath(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "."
	}
	if s == "~" || strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		s = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/"))
	}
	abs, err := filepath.Abs(s)
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(abs); err == nil {
		return p, nil
	}
	return abs, nil
}
