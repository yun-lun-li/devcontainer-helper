package console

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/common"
	"dch/internal/plugins"
)

type preparePendingPlugin struct{ containerCalls int }

func (*preparePendingPlugin) Definition() plugins.Definition {
	return plugins.Definition{ID: "prepare-pending", Name: "Prepare pending"}
}
func (*preparePendingPlugin) ValidateConfig(json.RawMessage) error { return nil }
func (*preparePendingPlugin) HandleHost(_ context.Context, _ plugins.HostContext, event plugins.Event) (plugins.Result, error) {
	if event == plugins.EventStart {
		return plugins.Result{NeedsBuild: true}, nil
	}
	return plugins.Result{}, nil
}
func (plugin *preparePendingPlugin) HandleContainer(_ context.Context, _ plugins.ContainerContext, _ plugins.Event) (plugins.Result, error) {
	plugin.containerCalls++
	return plugins.Result{}, nil
}
func (*preparePendingPlugin) ValidateContainerConfig(context.Context, plugins.ContainerContext, map[string]json.RawMessage) error {
	return nil
}
func (*preparePendingPlugin) Status(context.Context, plugins.HostContext) (plugins.PluginStatus, error) {
	return plugins.PluginStatus{}, nil
}

func TestStartBlocksWhenHostPreparationMakesExistingContainerPending(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".devcontainer", "devcontainer.json"), []byte(`{"image":"example:latest"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	plugin := &preparePendingPlugin{}
	manager := plugins.NewManager()
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{
		project: {Enabled: map[string]json.RawMessage{"prepare-pending": json.RawMessage(`{}`)}},
	}}
	if err := common.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "devcontainer-ran")
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte("#!/bin/sh\ntouch "+shellQuote(marker)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := startProject(manager, Project{Name: "project", Path: project, State: "STOPPED", ID: "existing-container"})
	if err == nil || !strings.Contains(err.Error(), "Build") {
		t.Fatalf("start error = %v, want pending Build error", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("devcontainer ran with pending settings, stat error = %v", err)
	}
	if plugin.containerCalls != 0 {
		t.Fatalf("container hook ran %d times, want it blocked", plugin.containerCalls)
	}
}
