package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dch/internal/githubapp"
)

type Config struct {
	Version        int                       `json:"version"`
	Projects       []string                  `json:"projects"`
	ProjectPlugins map[string]ProjectPlugins `json:"project_plugins,omitempty"`
}

// ProjectPlugins stores host-side configuration and ownership state for one project.
type ProjectPlugins struct {
	Enabled map[string]githubapp.Config `json:"enabled,omitempty"`
	States  map[string]PluginState      `json:"states,omitempty"`
}

// PluginState tracks generated-file ownership and whether containers need a Build.
type PluginState struct {
	FragmentDigest   string `json:"fragment_digest,omitempty"`
	BuildFingerprint string `json:"build_fingerprint,omitempty"`
	BuildPending     bool   `json:"build_pending,omitempty"`
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

func SaveConfig(cfg Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
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
