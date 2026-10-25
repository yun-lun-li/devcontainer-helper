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
