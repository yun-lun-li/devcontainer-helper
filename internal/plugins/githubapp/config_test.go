package githubapp

import (
	"encoding/json"
	"testing"
)

func TestUserAuthorizationConfigValidation(t *testing.T) {
	valid := Config{ClientID: "Iv1_example", ProjectPath: "/tmp/project", UserID: 2, Login: "tester", RepositoryID: 3, Repository: "octo/repo"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := []Config{{}, {ClientID: "Iv1_example", ProjectPath: "/tmp/p", UserID: 0, RepositoryID: 3, Repository: "octo/repo"}, {ClientID: "Iv1_example", ProjectPath: "/tmp/p", UserID: 2, Login: "tester", RepositoryID: 0, Repository: "octo/repo"}}
	for _, cfg := range invalid {
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate accepted %#v", cfg)
		}
	}
}

func TestConfigValidationAcceptsFineGrainedPATWithoutClientID(t *testing.T) {
	plugin := New()
	raw := json.RawMessage(`{"auth_method":"pat","project_path":"/tmp/project","user_id":2,"login":"tester","repository_id":3,"repository":"octo/repo"}`)
	if err := plugin.ValidateConfig(raw); err != nil {
		t.Fatalf("PAT config without ClientID: %v", err)
	}
	withClientID := json.RawMessage(`{"auth_method":"pat","client_id":"Iv1_example","project_path":"/tmp/project","user_id":2,"login":"tester","repository_id":3,"repository":"octo/repo"}`)
	if err := plugin.ValidateConfig(withClientID); err == nil {
		t.Fatal("PAT config accepted an App ClientID")
	}
}

func TestDecodeConfigRejectsLegacyInstallationMode(t *testing.T) {
	raw := json.RawMessage(`{"app_id":12,"installation_id":34,"private_key_pass_entry":"old/key","permissions":{"contents":"read"}}`)
	if !isLegacyConfig(raw) {
		t.Fatal("legacy mode was not detected")
	}
	if _, err := decodeConfig(raw); err == nil {
		t.Fatal("legacy mode was accepted")
	}
}
