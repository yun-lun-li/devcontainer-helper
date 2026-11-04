package console

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectProjectRoutesUserShell(t *testing.T) {
	projectDir := t.TempDir()
	fakeDevcontainer := installFakeDevcontainer(t)
	t.Setenv("PATH", filepath.Dir(fakeDevcontainer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	setConnectInput("1\n")
	defer resetConnectInput()

	if err := connectProject(Project{Name: "demo", Path: projectDir, State: "RUNNING"}); err != nil {
		t.Fatal(err)
	}
	args := readInvocation(t)
	if !strings.Contains(args, "--workspace-folder\n"+projectDir) || strings.Contains(args, "--config") {
		t.Fatalf("user route arguments = %q", args)
	}
}

func TestConnectProjectRoutesRootWithMergedTemporaryConfig(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := `{"name":"base","remoteUser":"developer","features":{"example":"1"}}`
	local := `{"name":"local","remoteUser":"local-user","remoteEnv":{"KEEP":"yes"}}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeDevcontainer := installFakeDevcontainer(t)
	t.Setenv("PATH", filepath.Dir(fakeDevcontainer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	setConnectInput("2\n")
	defer resetConnectInput()

	if err := connectProject(Project{Name: "demo", Path: projectDir, State: "RUNNING"}); err != nil {
		t.Fatal(err)
	}
	args := readInvocation(t)
	if !strings.Contains(args, "--config\n") || !strings.Contains(args, "/bin/sh") {
		t.Fatalf("root route arguments = %q", args)
	}
	configFlag := strings.Index(args, "--config\n")
	if configFlag > strings.Index(args, "/bin/sh\n") {
		t.Fatalf("config option must precede the shell command: %q", args)
	}
	configPath := strings.SplitN(args[configFlag+len("--config\n"):], "\n", 2)[0]
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("temporary config should be removed after command: %v", err)
	}
	data, err := os.ReadFile(os.Getenv("DCH_CAPTURE_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	features, _ := config["features"].(map[string]any)
	remoteEnv, _ := config["remoteEnv"].(map[string]any)
	if config["remoteUser"] != "root" || config["name"] != "local" || features["example"] != "1" || remoteEnv["KEEP"] != "yes" {
		t.Fatalf("temporary config did not preserve merged settings and override remoteUser: %s", data)
	}
	projectConfig, err := os.ReadFile(filepath.Join(configDir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	if string(projectConfig) != base {
		t.Fatalf("project config changed: %s", projectConfig)
	}
}

func TestConnectProjectCancelDoesNotInvokeDevcontainer(t *testing.T) {
	fakeDevcontainer := installFakeDevcontainer(t)
	t.Setenv("PATH", filepath.Dir(fakeDevcontainer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	setConnectInput("3\n")
	defer resetConnectInput()

	err := connectProject(Project{Name: "demo", Path: t.TempDir(), State: "RUNNING"})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if _, err := os.Stat(os.Getenv("DCH_CAPTURE_ARGS")); !os.IsNotExist(err) {
		t.Fatalf("devcontainer invoked on cancel: %v", err)
	}
}

func TestConnectProjectRequiresRunningBeforePrompt(t *testing.T) {
	fakeDevcontainer := installFakeDevcontainer(t)
	t.Setenv("PATH", filepath.Dir(fakeDevcontainer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	setConnectInput("2\n")
	defer resetConnectInput()

	err := connectProject(Project{Name: "demo", Path: t.TempDir(), State: "STOPPED"})
	if err == nil || !strings.Contains(err.Error(), "not RUNNING") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(os.Getenv("DCH_CAPTURE_ARGS")); !os.IsNotExist(err) {
		t.Fatalf("devcontainer invoked for stopped project: %v", err)
	}
}

func setConnectInput(value string) { input = bufio.NewReader(strings.NewReader(value)) }
func resetConnectInput()           { input = bufio.NewReader(os.Stdin) }

func installFakeDevcontainer(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	tool := filepath.Join(binDir, "devcontainer")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DCH_CAPTURE_ARGS\"\nwhile [ $# -gt 0 ]; do\n  if [ \"$1\" = --config ]; then cp \"$2\" \"$DCH_CAPTURE_CONFIG\"; fi\n  shift\ndone\n"
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	captureDir := t.TempDir()
	t.Setenv("DCH_CAPTURE_ARGS", filepath.Join(captureDir, "args"))
	t.Setenv("DCH_CAPTURE_CONFIG", filepath.Join(captureDir, "config"))
	return tool
}

func readInvocation(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("DCH_CAPTURE_ARGS"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
