package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"dch/internal/common"
)

func TestListShowsContainerIDWithoutPrompt(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".devcontainer", "devcontainer.json"), []byte(`{"name":"example"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}}); err != nil {
		t.Fatal(err)
	}

	container := common.DockerContainer{ID: "abc123", Name: "/example"}
	container.Config.Labels = map[string]string{
		"devcontainer.local_folder": project,
		"devcontainer.config_file":  filepath.Join(project, ".devcontainer", "devcontainer.json"),
	}
	container.State.Running = true
	container.State.Status = "running"
	data, err := json.Marshal([]common.DockerContainer{container})
	if err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	docker := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"ps\" ]; then echo abc123; else cat <<'JSON'\n%s\nJSON\nfi\n", data)
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(docker), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	oldStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()
	listErr := list()
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if listErr != nil {
		t.Fatalf("list returned %v; output: %s", listErr, out)
	}
	for _, want := range []string{"DEVCONTAINER", "STATUS", "PROJECT PATH", "CONTAINER ID", "example", "RUNNING", project, "abc123"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
	if strings.Contains(string(out), "Select project") {
		t.Errorf("list prompted for selection: %s", out)
	}
	if strings.Contains(string(out), "\x1b[") {
		t.Errorf("redirected output contains color codes: %q", out)
	}
}

func TestListColorsKeepColumnsAligned(t *testing.T) {
	projects := []common.Project{
		{Name: "short", State: "RUNNING", Path: "/work/short", ID: "abc123"},
		{Name: "long project", State: "STOPPED", Path: "/work/long", ID: "def456"},
	}
	var plain, colored bytes.Buffer
	renderProjects(&plain, projects, false)
	renderProjects(&colored, projects, true)
	if !strings.Contains(colored.String(), "\x1b[94mRUNNING\x1b[0m") || !strings.Contains(colored.String(), "\x1b[33mSTOPPED\x1b[0m") {
		t.Fatalf("status colors missing: %q", colored.String())
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if got := ansi.ReplaceAllString(colored.String(), ""); got != plain.String() {
		t.Fatalf("color changed table layout:\ncolored without ANSI: %q\nplain: %q", got, plain.String())
	}
}
