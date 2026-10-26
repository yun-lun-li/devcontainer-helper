package githubapp

import (
	"context"
	"encoding/json"
	"strings"

	"dch/internal/plugins"
	"testing"
)

func TestValidateConfigAcceptsOnlyAuthorizedUserBinding(t *testing.T) {
	plugin := New()
	if err := plugin.ValidateConfig(json.RawMessage(`{"client_id":"Iv1.abc123","user_id":42,"login":"test-user","project_path":"/tmp/project","repository_id":9,"repository":"octo/repo"}`)); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestLegacyInstallationConfigIsRecognizedButRejectedForUse(t *testing.T) {
	plugin := New()
	raw := json.RawMessage(`{"app_id":12,"installation_id":34,"private_key_pass_entry":"legacy/key","repository":"octo/repo","permissions":{"contents":"read"}}`)
	if !isLegacyConfig(raw) {
		t.Fatal("legacy config was not recognized")
	}
	if err := plugin.ValidateConfig(raw); err == nil {
		t.Fatal("legacy installation config must require user authorization")
	}
}

func TestSharedSettingsRequiresClientID(t *testing.T) {
	if err := New().ValidateSharedSettings(json.RawMessage(`{"client_id":"Iv1.abc123"}`)); err != nil {
		t.Fatalf("ValidateSharedSettings() error = %v", err)
	}
	if err := New().ValidateSharedSettings(json.RawMessage(`{"client_id":" "}`)); err == nil {
		t.Fatal("blank client ID accepted")
	}
}

func TestSharedSettingsCanOmitAppClientIDForPATProjects(t *testing.T) {
	if err := New().ValidateSharedSettings(json.RawMessage(`{}`)); err != nil {
		t.Fatalf("empty shared settings: %v", err)
	}
}

func TestStatusKeepsProjectAuthorizationWhenSharedClientIDChanges(t *testing.T) {
	cfg := Config{ClientID: "Iv1.old", ProjectPath: t.TempDir(), UserID: 42, Login: "octo", RepositoryID: 9, Repository: "octo/repo"}
	raw, _ := json.Marshal(cfg)
	shared, _ := json.Marshal(SharedConfig{ClientID: "Iv1.new"})
	status, err := New().Status(context.Background(), plugins.HostContext{ProjectPath: cfg.ProjectPath, Config: raw, SharedConfig: shared})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(status.Message, "reauthorize") {
		t.Fatalf("status message = %q", status.Message)
	}
}

func TestStatusBlocksPATWhenItsCredentialRecoveryIsPending(t *testing.T) {
	project := t.TempDir()
	cfg := Config{AuthMethod: authMethodPAT, ProjectPath: project, UserID: 42, Login: "octo", RepositoryID: 9, Repository: "octo/repo"}
	raw, _ := json.Marshal(cfg)
	state := json.RawMessage(`{"credential_cleanup_identities":[{"method":"pat","key":"pat"}]}`)
	status, err := New().Status(context.Background(), plugins.HostContext{ProjectPath: project, Config: raw, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Message, "recovery is pending") {
		t.Fatalf("status message = %q", status.Message)
	}
}
