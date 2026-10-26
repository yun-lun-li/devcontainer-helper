package githubapp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	AuthMethod   string `json:"auth_method,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ProjectPath  string `json:"project_path"`
	UserID       int64  `json:"user_id"`
	Login        string `json:"login"`
	RepositoryID int64  `json:"repository_id"`
	Repository   string `json:"repository"`
}

const (
	authMethodApp = "app"
	authMethodPAT = "pat"
)

type SharedConfig struct {
	ClientID string `json:"client_id"`
}

var clientIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{4,128}$`)

func (p *Plugin) ValidateSharedSettings(raw json.RawMessage) error {
	var cfg SharedConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("invalid GitHub App shared settings")
	}
	if cfg.ClientID == "" {
		return nil
	}
	if strings.TrimSpace(cfg.ClientID) != cfg.ClientID || !clientIDPattern.MatchString(cfg.ClientID) {
		return fmt.Errorf("GitHub App Client ID is required")
	}
	return nil
}

func isLegacyConfig(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	_, app := fields["app_id"]
	_, installation := fields["installation_id"]
	_, privateKey := fields["private_key_pass_entry"]
	return app || installation || privateKey
}

func decodeConfig(raw json.RawMessage) (Config, error) {
	if isLegacyConfig(raw) {
		return Config{}, fmt.Errorf("GitHub App requires user authorization; legacy installation credentials are not used")
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("invalid GitHub App configuration")
	}
	return cfg, nil
}

func (c Config) Validate() error {
	method := c.authMethod()
	validClientID := method == authMethodApp && clientIDPattern.MatchString(c.ClientID) || method == authMethodPAT && c.ClientID == ""
	if !validClientID || !filepath.IsAbs(c.ProjectPath) || filepath.Clean(c.ProjectPath) != c.ProjectPath || c.UserID <= 0 || c.RepositoryID <= 0 || c.Repository == "" {
		return fmt.Errorf("GitHub App user authorization is incomplete")
	}
	if method != authMethodApp && method != authMethodPAT {
		return fmt.Errorf("GitHub authentication method is invalid")
	}
	if !validRepositoryName(c.Repository) {
		return fmt.Errorf("GitHub App repository binding is invalid")
	}
	return nil
}

func (c Config) authMethod() string {
	if c.AuthMethod == "" {
		return authMethodApp
	}
	return c.AuthMethod
}

func validRepositoryName(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 2 && validOwner(parts[0]) && validRepository(parts[1])
}

func validOwner(name string) bool {
	if name == "" || len(name) > 39 || name[0] == '-' || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func validRepository(name string) bool {
	if name == "" || len(name) > 100 || name == "." || name == ".." || strings.HasSuffix(name, ".git") || strings.HasSuffix(name, ".") {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
