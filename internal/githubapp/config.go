package githubapp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	AppID               int64             `json:"app_id"`
	InstallationID      int64             `json:"installation_id"`
	PrivateKeyPassEntry string            `json:"private_key_pass_entry"`
	Repository          string            `json:"repository"`
	Permissions         map[string]string `json:"permissions"`
}

func (c Config) appIDString() string { return strconv.FormatInt(c.AppID, 10) }

var passEntryPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func (c Config) Validate() error {
	if c.AppID <= 0 || c.InstallationID <= 0 {
		return fmt.Errorf("app and installation IDs must be positive")
	}
	if c.PrivateKeyPassEntry == "" || strings.HasPrefix(c.PrivateKeyPassEntry, "-") || !passEntryPattern.MatchString(c.PrivateKeyPassEntry) || strings.Contains(c.PrivateKeyPassEntry, "..") {
		return fmt.Errorf("invalid pass entry")
	}
	parts := strings.Split(c.Repository, "/")
	if len(parts) != 2 || !validOwner(parts[0]) || !validRepository(parts[1]) {
		return fmt.Errorf("repository must be owner/repository")
	}
	if len(c.Permissions) == 0 {
		return fmt.Errorf("at least one permission is required")
	}
	for name, level := range c.Permissions {
		if name != "contents" && name != "pull_requests" {
			return fmt.Errorf("unsupported permission")
		}
		if level != "read" && level != "write" {
			return fmt.Errorf("invalid permission level")
		}
	}
	return nil
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
	if name == "" || len(name) > 100 || name == "." || name == ".." || strings.HasSuffix(name, ".git") || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
