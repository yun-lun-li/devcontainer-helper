package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAvailableProjectActions(t *testing.T) {
	cases := []struct {
		state  string
		valid  bool
		docker bool
		want   []projectAction
	}{
		{"RUNNING", true, true, []projectAction{actionConnect, actionStop, actionBuild, actionBack}},
		{"RUNNING (2)", true, true, []projectAction{actionConnect, actionStop, actionBuild, actionBack}},
		{"STOPPED", true, true, []projectAction{actionStart, actionBuild, actionBack}},
		{"NOT CREATED", true, true, []projectAction{actionStart, actionBuild, actionBack}},
		{"UNKNOWN", true, false, []projectAction{actionBack}},
		{"CONFIG MISSING/INVALID", false, true, []projectAction{actionBack}},
	}
	for _, tc := range cases {
		got := availableProjectActions(Project{State: tc.state, Valid: tc.valid}, tc.docker)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("state %q: got %v, want %v", tc.state, got, tc.want)
		}
	}
}

func TestStopOffersOptionalDeleteForMatchingContainers(t *testing.T) {
	for _, tc := range []struct {
		name, answer, want string
	}{
		{"keep", "n\n", "stop running\n"},
		{"delete", "y\n", "stop running\nrm running\nrm stopped\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testStopProject(t, tc.answer, tc.want)
		})
	}
}

func testStopProject(t *testing.T, answer, want string) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	configFile := filepath.Join(project, ".devcontainer", "devcontainer.json")
	containers := []DockerContainer{
		{ID: "running"}, {ID: "stopped"}, {ID: "other"},
	}
	for i := range containers {
		containers[i].Config.Labels = map[string]string{
			"devcontainer.local_folder": project,
			"devcontainer.config_file":  configFile,
		}
	}
	containers[0].State.Running = true
	containers[2].Config.Labels["devcontainer.config_file"] = "/other/config.json"
	data, err := json.Marshal(containers)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "docker.log")
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n  ps) echo 'running stopped other' ;;\n  inspect) cat <<'JSON'\n%s\nJSON\n    ;;\n  *) echo \"$1 $2\" >> %q ;;\nesac\n", data, logPath)
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	previous := input
	input = bufio.NewReader(strings.NewReader(answer))
	t.Cleanup(func() { input = previous })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	previousStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = previousStdout }()
	if err := stopProject(Project{Name: "project", Path: project}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Delete all 2 matching container(s)") {
		t.Fatalf("missing post-stop delete prompt: %s", output)
	}
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected Docker actions: %s", got)
	}
}
