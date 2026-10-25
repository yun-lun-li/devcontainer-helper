package githubapp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigDefaultsPassEntryAndAllowsInstallationWideScope(t *testing.T) {
	cfg := Config{AppID: 12, InstallationID: 34, Permissions: map[string]string{"contents": "read"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := cfg.normalized().PrivateKeyPassEntry; got != "dch/github-app/12/private-key" {
		t.Fatalf("default pass entry = %q", got)
	}
}

func TestConfigValidateAcceptsSingleRepositoryAndSupportedPermissions(t *testing.T) {
	cfg := Config{AppID: 12, InstallationID: 34, PrivateKeyPassEntry: "dch/project/key", Repository: "octo/repo", Permissions: map[string]string{"contents": "read", "pull_requests": "write"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsGitHubRepositoryPunctuation(t *testing.T) {
	cfg := Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "valid/key", Repository: "octo/devcontainer_helper.test", Permissions: map[string]string{"contents": "read"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsUnsafeOrBroaderConfiguration(t *testing.T) {
	cases := []Config{
		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "-bad", Repository: "octo/repo", Permissions: map[string]string{"contents": "read"}},
		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "valid/key", Repository: "octo/repo/other", Permissions: map[string]string{"contents": "read"}},
		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "valid/key", Repository: "octo/repo", Permissions: map[string]string{"issues": "write"}},
		{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "valid/key", Repository: "octo/repo", Permissions: map[string]string{"contents": "admin"}},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() accepted %#v", cfg)
		}
	}
}

func TestValidateConfigReportsSafeIDFieldErrors(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"App ID URL", `{"app_id":"https://github.com/apps/example","installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a decimal numeric App ID (not Client ID)"},
		{"App ID Client ID", `{"app_id":"Iv1.example","installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a decimal numeric App ID (not Client ID)"},
		{"App ID nondecimal", `{"app_id":"12x","installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a decimal numeric App ID (not Client ID)"},
		{"App ID overflow", `{"app_id":"9223372036854775808","installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a decimal numeric App ID (not Client ID)"},
		{"App ID missing", `{"installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a positive numeric App ID"},
		{"App ID zero", `{"app_id":"0","installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a positive numeric App ID"},
		{"App ID negative", `{"app_id":"-1","installation_id":"34","permissions":{"contents":"read"}}`, "app_id must be a positive numeric App ID"},
		{"Installation ID URL", `{"app_id":"12","installation_id":"https://github.com/settings/installations/34","permissions":{"contents":"read"}}`, "installation_id must be the numeric installation ID from the installation settings URL, not the App ID or Client ID"},
		{"Installation ID Client ID", `{"app_id":"12","installation_id":"Iv1.example","permissions":{"contents":"read"}}`, "installation_id must be the numeric installation ID from the installation settings URL, not the App ID or Client ID"},
		{"Installation ID nondecimal", `{"app_id":"12","installation_id":"34x","permissions":{"contents":"read"}}`, "installation_id must be the numeric installation ID from the installation settings URL, not the App ID or Client ID"},
		{"Installation ID overflow", `{"app_id":"12","installation_id":"9223372036854775808","permissions":{"contents":"read"}}`, "installation_id must be the numeric installation ID from the installation settings URL, not the App ID or Client ID"},
		{"Installation ID missing", `{"app_id":"12","permissions":{"contents":"read"}}`, "installation_id must be a positive numeric installation ID"},
		{"Installation ID zero", `{"app_id":"12","installation_id":"0","permissions":{"contents":"read"}}`, "installation_id must be a positive numeric installation ID"},
		{"Installation ID negative", `{"app_id":"12","installation_id":"-1","permissions":{"contents":"read"}}`, "installation_id must be a positive numeric installation ID"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateConfig(json.RawMessage(tt.raw))
			if err == nil || err.Error() != tt.want {
				t.Fatalf("ValidateConfig() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "https://") || strings.Contains(err.Error(), "Iv1.") || strings.Contains(err.Error(), "9223372036854775808") {
				t.Fatalf("validation error exposed an input value: %v", err)
			}
		})
	}
}

func TestValidateConfigReportsAllowlistedTypeErrors(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"repository", `{"app_id":"12","installation_id":"34","repository":12,"permissions":{"contents":"read"}}`, "repository must be a string"},
		{"private key pass entry", `{"app_id":"12","installation_id":"34","private_key_pass_entry":12,"permissions":{"contents":"read"}}`, "private_key_pass_entry must be a string"},
		{"permissions object", `{"app_id":"12","installation_id":"34","permissions":"contents"}`, "permissions must be an object"},
		{"permission value", `{"app_id":"12","installation_id":"34","permissions":{"contents":12}}`, "permissions values must be strings"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateConfig(json.RawMessage(tt.raw))
			if err == nil || err.Error() != tt.want {
				t.Fatalf("ValidateConfig() error = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), `12`) || strings.Contains(err.Error(), "contents") {
				t.Fatalf("validation error exposed an input value or key: %v", err)
			}
		})
	}
}

func TestValidateConfigHidesMalformedJSONDetails(t *testing.T) {
	err := New().ValidateConfig(json.RawMessage(`{"app_id":"12", "private_key_pass_entry":`))
	if err == nil || err.Error() != "invalid GitHub App configuration" {
		t.Fatalf("ValidateConfig() error = %v, want sanitized malformed-config error", err)
	}
}
