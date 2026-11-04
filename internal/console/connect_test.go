package console

import (
	"bufio"
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

func TestConnectProjectRoutesRootThroughDockerExec(t *testing.T) {
	fakeDocker := installFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(fakeDocker)+string(os.PathListSeparator)+os.Getenv("PATH"))
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"workspaceFolder":"/workspaces/demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	setConnectInput("2\n")
	defer resetConnectInput()

	if err := connectProject(Project{Name: "demo", Path: projectDir, ID: "container-id", State: "RUNNING"}); err != nil {
		t.Fatal(err)
	}
	if args := readInvocation(t); !strings.Contains(args, "exec\n-u\nroot\n-w\n/workspaces/demo\n-it\ncontainer-id\n") || !strings.Contains(args, "/bin/sh") {
		t.Fatalf("root route arguments = %q", args)
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

func installFakeDocker(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	tool := filepath.Join(binDir, "docker")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DCH_CAPTURE_ARGS\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	captureDir := t.TempDir()
	t.Setenv("DCH_CAPTURE_ARGS", filepath.Join(captureDir, "args"))
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
