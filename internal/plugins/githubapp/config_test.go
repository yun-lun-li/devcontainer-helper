package githubapp

import "testing"

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
