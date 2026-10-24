package common

import (
	"os"
	"path/filepath"
	"testing"

	"dch/internal/githubapp"
)

func TestConfigRoundTripsProjectPluginsAndLoadsLegacyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	t.Setenv("DCH_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"version":1,"projects":["/old"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil || len(cfg.Projects) != 1 || len(cfg.ProjectPlugins) != 0 {
		t.Fatalf("legacy config = %#v, %v", cfg, err)
	}
	cfg.ProjectPlugins = map[string]ProjectPlugins{}
	cfg.ProjectPlugins["/project"] = ProjectPlugins{
		Enabled: map[string]githubapp.Config{"github-app": {AppID: 7, InstallationID: 8, PrivateKeyPassEntry: "key", Repository: "owner/repo", Permissions: map[string]string{"contents": "read"}}},
		States:  map[string]PluginState{"github-app": {FragmentDigest: "digest", UnitDigest: "unit-digest", UnitPath: "/home/user/.config/systemd/user/dch.service", BuildPending: true}},
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.ProjectPlugins["/project"]
	if got.Enabled["github-app"].AppID != 7 || got.States["github-app"].FragmentDigest != "digest" || got.States["github-app"].UnitDigest != "unit-digest" || got.States["github-app"].UnitPath != "/home/user/.config/systemd/user/dch.service" || !got.States["github-app"].BuildPending {
		t.Fatalf("plugin state did not round trip: %#v", got)
	}
}
