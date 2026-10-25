package commands

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"dch/internal/common"
	"dch/internal/plugins"
)

type serveTestPlugin struct{ served *string }

func (plugin *serveTestPlugin) Definition() plugins.Definition {
	return plugins.Definition{ID: "sample", Name: "Sample", ServiceAliases: []string{"_sample-serve"}}
}
func (*serveTestPlugin) ValidateConfig(json.RawMessage) error { return nil }
func (*serveTestPlugin) HandleHost(_ context.Context, _ plugins.HostContext, _ plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (*serveTestPlugin) HandleContainer(_ context.Context, _ plugins.ContainerContext, _ plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (*serveTestPlugin) ValidateContainerConfig(context.Context, plugins.ContainerContext, map[string]json.RawMessage) error {
	return nil
}
func (*serveTestPlugin) Status(context.Context, plugins.HostContext) (plugins.PluginStatus, error) {
	return plugins.PluginStatus{}, nil
}
func (plugin *serveTestPlugin) ServeHost(_ context.Context, host plugins.HostContext) error {
	*plugin.served = host.ProjectPath
	return nil
}

func TestPluginServeCommandUsesGenericIDAndRegisteredAlias(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}}); err != nil {
		t.Fatal(err)
	}
	served := ""
	manager := plugins.NewManager()
	if err := manager.Register(&serveTestPlugin{served: &served}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), project, "sample", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"_plugin-serve", "--plugin", "sample", "--project", project},
		{"_sample-serve", "--project", project},
	} {
		if err := Run(args, manager, nil); err != nil {
			t.Fatalf("Run(%v): %v", args, err)
		}
		if served != project {
			t.Fatalf("host service got project %q, want %q", served, project)
		}
	}
	if err := Run([]string{"_plugin-serve", "--plugin", "sample"}, manager, nil); err == nil {
		t.Fatal("generic service command accepted missing project path")
	}
	if err := Run([]string{"_sample-serve", "--project"}, manager, nil); err == nil {
		t.Fatal("service alias accepted missing project path")
	}
}
