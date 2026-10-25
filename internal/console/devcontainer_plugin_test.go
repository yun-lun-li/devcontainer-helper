package console

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/common"
)

func TestProjectExecUsesStableContainerLabelsAndLiveMergedConfig(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"example:latest","postCreateCommand":"echo preserved"}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "args.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\nconfig=\nwhile [ $# -gt 0 ]; do if [ \"$1\" = --config ]; then config=$2; fi; shift; done\n[ -f \"$config\" ] || exit 12\n"
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := invokeProjectDevcontainer(context.Background(), newConsoleTestManager(t), Project{Name: "project", Path: project}, "exec", "--workspace-folder", project, "true"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	for _, want := range []string{"devcontainer.local_folder=" + project, "devcontainer.config_file=" + filepath.Join(configDir, "devcontainer.json"), "--config"} {
		if !strings.Contains(got, want) {
			t.Errorf("exec args missing %q: %s", want, got)
		}
	}
}

func TestConnectRefusesExistingContainerWithPendingPluginBuild(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{
		project: {Enabled: map[string]json.RawMessage{"github-app": authorizedPluginConfig(t, project)}, States: map[string]common.PluginState{"github-app": {BuildPending: true}}},
	}}
	if err := common.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	err := connectProject(newConsoleTestManager(t), Project{Name: "project", Path: project, State: "RUNNING"})
	if err == nil || !strings.Contains(err.Error(), "Build") {
		t.Fatalf("connect error = %v, want pending Build message", err)
	}
}

func TestConnectAlsoBlocksAfterPluginWasDisabledUntilBuild(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {States: map[string]common.PluginState{"github-app": {BuildPending: true}}}}}
	if err := common.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	err := connectProject(newConsoleTestManager(t), Project{Name: "project", Path: project, State: "RUNNING", ID: "container-id"})
	if err == nil || !strings.Contains(err.Error(), "Build") {
		t.Fatalf("connect error=%v, want Build requirement", err)
	}
}

func TestStartRefusesExistingContainerWithPendingPluginBuild(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	_ = os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700)
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	config := common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]json.RawMessage{"github-app": authorizedPluginConfig(t, project)}, States: map[string]common.PluginState{"github-app": {BuildPending: true}}}}}
	if err := common.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	err := startProject(newConsoleTestManager(t), Project{Name: "project", Path: project, State: "STOPPED"})
	if err == nil || !strings.Contains(err.Error(), "Build") {
		t.Fatalf("start error = %v, want pending Build message", err)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
