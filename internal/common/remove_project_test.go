package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveProjectRegistrationDeletesPerProjectPluginMetadata(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	project := filepath.Join(root, "project")
	config := Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]ProjectPlugins{project: {Enabled: map[string]json.RawMessage{}, States: map[string]PluginState{"plugin-a": {BuildPending: true}}}}}
	if err := SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	if err := RemoveProjectRegistration(project); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 0 {
		t.Fatalf("registered projects=%v", loaded.Projects)
	}
	if _, exists := loaded.ProjectPlugins[project]; exists {
		t.Fatal("removed project plugin metadata remains")
	}
	if _, err := os.Stat(filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
}
