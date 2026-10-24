package console

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/common"
)

func TestBuildRequiresConfirmationBeforeReplacingExistingContainer(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	configDir := filepath.Join(project, ".devcontainer")
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
	marker := filepath.Join(root, "called")
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte("#!/bin/sh\ntouch "+shellQuote(marker)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	oldInput := input
	input = bufio.NewReader(strings.NewReader("2\n"))
	defer func() { input = oldInput }()
	err := buildProject(Project{Name: "project", Path: project, State: "RUNNING"})
	if !errors.Is(err, errCancelled) {
		t.Fatalf("build error=%v, want cancellation", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("devcontainer ran before confirmation, stat err=%v", err)
	}
}

func TestBuildAfterDisableAppliesConfigAndClearsPending(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"image":"example:latest","postCreateCommand":"echo keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "args.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\nconfig=\nprev=\nfor arg in \"$@\"; do if [ \"$prev\" = --config ]; then config=$arg; fi; prev=$arg; done\n[ -f \"$config\" ] || exit 12\n"
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	configPath := filepath.Join(root, "config")
	t.Setenv("DCH_CONFIG", configPath)
	projectPlugins := common.ProjectPlugins{States: map[string]common.PluginState{"github-app": {BuildPending: true}}}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: projectPlugins}}); err != nil {
		t.Fatal(err)
	}
	oldInput := input
	input = bufio.NewReader(strings.NewReader("1\n"))
	defer func() { input = oldInput }()
	if err := buildProject(Project{Name: "project", Path: project, State: "NOT CREATED"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProjectPlugins[project].States["github-app"].BuildPending {
		t.Fatal("successful Build did not clear disabled-plugin pending state")
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--remove-existing-container") {
		t.Fatalf("Build did not request rebuild: %s", args)
	}
}
