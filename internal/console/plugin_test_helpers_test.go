package console

import (
	"encoding/json"
	"testing"

	"dch/internal/plugins"
	"dch/internal/plugins/githubapp"
)

func newConsoleTestManager(t *testing.T) *plugins.Manager {
	t.Helper()
	manager := plugins.NewManager()
	if err := manager.Register(githubapp.New()); err != nil {
		t.Fatal(err)
	}
	return manager
}

func rawPluginConfig(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func authorizedPluginConfig(t *testing.T, project string) json.RawMessage {
	t.Helper()
	return rawPluginConfig(t, githubapp.Config{ClientID: "Iv1_example", ProjectPath: project, UserID: 42, RepositoryID: 9, Repository: "octo/repo"})
}
