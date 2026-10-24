package console

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
		{"RUNNING", true, true, []projectAction{actionConnect, actionStop, actionBuild, actionConfig, actionRemove, actionBack}},
		{"RUNNING (2)", true, true, []projectAction{actionConnect, actionStop, actionBuild, actionConfig, actionRemove, actionBack}},
		{"STOPPED", true, true, []projectAction{actionStart, actionBuild, actionConfig, actionRemove, actionBack}},
		{"NOT CREATED", true, true, []projectAction{actionStart, actionBuild, actionConfig, actionRemove, actionBack}},
		{"UNKNOWN", true, false, []projectAction{actionConfig, actionRemove, actionBack}},
		{"CONFIG MISSING/INVALID", false, true, []projectAction{actionConfig, actionRemove, actionBack}},
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
		{"keep", "1\n", "stop running\n"},
		{"delete", "2\n", "stop running\nrm running\nrm stopped\n"},
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
	if !strings.Contains(string(output), "Keep containers") || !strings.Contains(string(output), "Delete all 2 matching containers") {
		t.Fatalf("missing post-stop menu options: %s", output)
	}
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected Docker actions: %s", got)
	}
}

func TestStopHandlesContainersDisappearing(t *testing.T) {
	for _, tc := range []struct {
		mode, wantLog string
		wantError     bool
	}{
		{"auto", "stop vanishing\n", false},
		{"race", "stop vanishing\nrm vanishing\n", false},
		{"failure", "stop vanishing\nrm vanishing\n", true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "project")
			container := DockerContainer{ID: "vanishing"}
			container.State.Running = true
			container.Config.Labels = map[string]string{
				"devcontainer.local_folder": project,
				"devcontainer.config_file":  filepath.Join(project, ".devcontainer", "devcontainer.json"),
			}
			data, err := json.Marshal([]DockerContainer{container})
			if err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			gone := filepath.Join(root, "gone")
			logPath := filepath.Join(root, "docker.log")
			script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  ps) if [ ! -f %q ]; then echo vanishing; fi ;;
  inspect) cat <<'JSON'
%s
JSON
    ;;
  stop) echo "stop $2" >> %q; if [ "$DCH_TEST_MODE" = auto ]; then touch %q; fi ;;
  rm) echo "rm $2" >> %q
      if [ "$DCH_TEST_MODE" = failure ]; then
        echo 'permission denied' >&2
      else
        touch %q
        echo 'Error response from daemon: No such container: vanishing' >&2
      fi
      exit 1 ;;
esac
`, gone, data, logPath, gone, logPath, gone)
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DCH_TEST_MODE", tc.mode)
			previous := input
			input = bufio.NewReader(strings.NewReader("2\n"))
			defer func() { input = previous }()
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			previousStdout := os.Stdout
			os.Stdout = w
			defer func() { os.Stdout = previousStdout }()
			stopErr := stopProject(Project{Name: "project", Path: project})
			if (stopErr != nil) != tc.wantError {
				t.Fatalf("stopProject error = %v, want error %v", stopErr, tc.wantError)
			}
			w.Close()
			output, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			log, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(log) != tc.wantLog {
				t.Fatalf("unexpected Docker actions: %s", log)
			}
			if strings.Contains(string(output), "No such container") {
				t.Fatalf("reported an already removed container as an error: %s", output)
			}
			if tc.mode == "auto" && !strings.Contains(string(output), "No matching containers remain") {
				t.Fatalf("missing auto-removal notice: %s", output)
			}
			if tc.mode == "race" && !strings.Contains(string(output), "was already removed") {
				t.Fatalf("missing removal-race notice: %s", output)
			}
			if tc.mode == "failure" && !strings.Contains(stopErr.Error(), "permission denied") {
				t.Fatalf("lost the real Docker error: %v", stopErr)
			}
		})
	}
}
