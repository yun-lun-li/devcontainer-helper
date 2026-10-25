package githubapp

import (
	"encoding/json"
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

func (c *Config) UnmarshalJSON(data []byte) error {
	type configAlias Config
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	appID, err := decodeID(fields["app_id"])
	if err != nil {
		return err
	}
	installationID, err := decodeID(fields["installation_id"])
	if err != nil {
		return err
	}
	delete(fields, "app_id")
	delete(fields, "installation_id")
	remaining, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	var decoded configAlias
	if err := json.Unmarshal(remaining, &decoded); err != nil {
		return err
	}
	decoded.AppID, decoded.InstallationID = appID, installationID
	*c = Config(decoded)
	return nil
}

func decodeID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var number int64
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, err
		}
		return strconv.ParseInt(value, 10, 64)
	}
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, err
	}
	return number, nil
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
