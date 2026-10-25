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

type executorValidationPlugin struct{ validated bool }

func (*executorValidationPlugin) Definition() plugins.Definition {
	return plugins.Definition{ID: "executor-test", Name: "Executor test"}
}
func (*executorValidationPlugin) ValidateConfig(json.RawMessage) error { return nil }
func (*executorValidationPlugin) HandleHost(context.Context, plugins.HostContext, plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (*executorValidationPlugin) HandleContainer(context.Context, plugins.ContainerContext, plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (plugin *executorValidationPlugin) ValidateContainerConfig(_ context.Context, _ plugins.ContainerContext, merged map[string]json.RawMessage) error {
	plugin.validated = string(merged["image"]) == `"example:latest"`
	return nil
}
func (*executorValidationPlugin) Status(context.Context, plugins.HostContext) (plugins.PluginStatus, error) {
	return plugins.PluginStatus{}, nil
}

func TestProjectExecutorReturnsStdoutAndValidatesMergedConfig(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	configDir := filepath.Join(projectPath, ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"example:latest"}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(root, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsPath) + "\nif [ \"$DCH_EMIT_BIG\" = 1 ]; then head -c 1048577 /dev/zero; else printf 'executor stdout\\n'; fi\nprintf 'diagnostic stderr\\n' >&2\nconfig=\nwhile [ $# -gt 0 ]; do if [ \"$1\" = --config ]; then config=$2; fi; shift; done\n[ -f \"$config\" ] || exit 12\ngrep -q 'example:latest' \"$config\" || exit 13\n"
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	plugin := &executorValidationPlugin{}
	manager := plugins.NewManager()
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{projectPath}, ProjectPlugins: map[string]common.ProjectPlugins{
		projectPath: {Enabled: map[string]json.RawMessage{"executor-test": json.RawMessage(`{}`)}},
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := (projectExecutor{manager: manager, project: Project{Name: "project", Path: projectPath}}).Exec(context.Background(), "true")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "executor stdout\n" {
		t.Fatalf("executor output = %q, want stdout only", out)
	}
	if !plugin.validated {
		t.Fatal("merged project config was not validated")
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--workspace-folder\n"+projectPath+"\n") || !strings.Contains(string(args), "--config\n") {
		t.Fatalf("devcontainer args do not include workspace/config: %s", args)
	}
	configArg := strings.Split(string(args), "--config\n")[1]
	configPath := strings.SplitN(configArg, "\n", 2)[0]
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("temporary merged config still exists after execution: %s (stat error: %v)", configPath, err)
	}
	t.Setenv("DCH_EMIT_BIG", "1")
	if _, err := (projectExecutor{manager: manager, project: Project{Name: "project", Path: projectPath}}).Exec(context.Background(), "true"); err == nil || !strings.Contains(err.Error(), "stdout output exceeded 1048576 bytes") {
		t.Fatalf("oversized stdout error = %v, want output limit error", err)
	}
}
