package githubapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

type configFieldError string

func (err configFieldError) Error() string { return string(err) }

func (c *Config) UnmarshalJSON(data []byte) error {
	type configAlias Config
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	appID, err := decodeID(fields["app_id"])
	if err != nil {
		return configFieldError("app_id must be a decimal numeric App ID (not Client ID)")
	}
	installationID, err := decodeID(fields["installation_id"])
	if err != nil {
		return configFieldError("installation_id must be the numeric installation ID from the installation settings URL, not the App ID or Client ID")
	}
	delete(fields, "app_id")
	delete(fields, "installation_id")
	remaining, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	var decoded configAlias
	if err := json.Unmarshal(remaining, &decoded); err != nil {
		return safeConfigTypeError(err)
	}
	decoded.AppID, decoded.InstallationID = appID, installationID
	*c = Config(decoded)
	return nil
}

func safeConfigTypeError(err error) error {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return err
	}
	switch strings.ToLower(typeErr.Field) {
	case "repository":
		return configFieldError("repository must be a string")
	case "privatekeypassentry", "private_key_pass_entry":
		return configFieldError("private_key_pass_entry must be a string")
	case "permissions":
		if typeErr.Type == reflect.TypeOf(map[string]string(nil)) {
			return configFieldError("permissions must be an object")
		}
		return configFieldError("permissions values must be strings")
	}
	if strings.HasPrefix(strings.ToLower(typeErr.Field), "permissions.") {
		return configFieldError("permissions values must be strings")
	}
	return err
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

func (c Config) normalized() Config {
	if c.PrivateKeyPassEntry == "" && c.AppID > 0 {
		c.PrivateKeyPassEntry = "dch/github-app/" + c.appIDString() + "/private-key"
	}
	return c
}

var passEntryPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func (c Config) Validate() error {
	c = c.normalized()
	if err := validateIDs(c); err != nil {
		return err
	}
	if err := validatePassEntry(c.PrivateKeyPassEntry); err != nil {
		return err
	}
	if err := validateOptionalRepository(c.Repository); err != nil {
		return err
	}
	return validatePermissions(c.Permissions)
}

func validateIDs(c Config) error {
	if c.AppID <= 0 {
		return configFieldError("app_id must be a positive numeric App ID")
	}
	if c.InstallationID <= 0 {
		return configFieldError("installation_id must be a positive numeric installation ID")
	}
	return nil
}

func validatePassEntry(entry string) error {
	if entry == "" || strings.HasPrefix(entry, "-") || !passEntryPattern.MatchString(entry) || strings.Contains(entry, "..") {
		return fmt.Errorf("invalid pass entry")
	}
	return nil
}

func validateOptionalRepository(repository string) error {
	if repository == "" {
		return nil
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || !validOwner(parts[0]) || !validRepository(parts[1]) {
		return fmt.Errorf("repository must be owner/repository")
	}
	return nil
}

func validatePermissions(permissions map[string]string) error {
	if len(permissions) == 0 {
		return fmt.Errorf("at least one permission is required")
	}
	for name, level := range permissions {
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
