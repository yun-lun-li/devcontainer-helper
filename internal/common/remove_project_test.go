package common

import (
	"os"
	"path/filepath"
	"testing"

	"dch/internal/githubapp"
)

func TestRemoveProjectRegistrationDeletesPerProjectPluginMetadata(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	project := filepath.Join(root, "project")
	config := Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]ProjectPlugins{project: {Enabled: map[string]githubapp.Config{}, States: map[string]PluginState{"github-app": {BuildPending: true}}}}}
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

func TestRemoveProjectRegistrationPreservesUnknownPluginMetadata(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	project := filepath.Join(root, "project")
	for _, tc := range []struct {
		name    string
		plugins ProjectPlugins
	}{
		{name: "enabled plugin", plugins: ProjectPlugins{Enabled: map[string]githubapp.Config{"future-plugin": {AppID: 7}}}},
		{name: "plugin state", plugins: ProjectPlugins{States: map[string]PluginState{"future-plugin": {BuildPending: true}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertUnknownPluginMetadataPreserved(t, project, tc.plugins)
		})
	}
}

func assertUnknownPluginMetadataPreserved(t *testing.T, project string, plugins ProjectPlugins) {
	t.Helper()
	config := Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]ProjectPlugins{project: plugins}}
	if err := SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	if err := RemoveProjectRegistration(project); err == nil {
		t.Fatal("expected removal to reject unknown plugin metadata")
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0] != project {
		t.Fatalf("unknown plugin project was removed: %v", loaded.Projects)
	}
	if _, exists := loaded.ProjectPlugins[project]; !exists {
		t.Fatal("unknown plugin metadata was removed")
	}
}
