package console

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProjectConfigPreviewAndDevcontainerUseOptionAwareMerge(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := `{"runArgs":["--cpus","2","--memory=4g","-e","BASE=base","--env=OVERRIDE=base","--label","same=base","--label=base-only=yes","--cap-add","NET_ADMIN","--name","demo"],"build":{"options":["--build-arg","BASE=base","--build-arg=OVERRIDE=base","--platform","linux/amd64","--no-cache"]}}`
	local := `{"runArgs":["--cpus=4","--memory","8g","--env","BASE=local","-e","OVERRIDE=local","-l","same=local","--label","local-only=yes","--cap-add=SYS_PTRACE"],"build":{"options":["--build-arg=BASE=local","--build-arg","OVERRIDE=local","--platform=linux/arm64","--progress","plain"]}}`
	basePath := filepath.Join(configDir, "devcontainer.json")
	localPath := filepath.Join(configDir, "devcontainer.local.json")
	writeMergeFixture(t, basePath, base)
	writeMergeFixture(t, localPath, local)

	preview := captureProjectPreview(t, Project{Name: "demo", Path: projectDir})
	previewConfig := decodeConfig(t, []byte(strings.SplitN(preview, "\n\nKeep .devcontainer", 2)[0]))

	fake := installFakeDevcontainer(t)
	t.Setenv("PATH", filepath.Dir(fake)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := invokeProjectDevcontainer(Project{Name: "demo", Path: projectDir}, "up"); err != nil {
		t.Fatal(err)
	}
	cliConfig := decodeConfig(t, mustReadFile(t, os.Getenv("DCH_CAPTURE_CONFIG")))

	wantRunArgs := []any{"--label=base-only=yes", "--cap-add", "NET_ADMIN", "--name", "demo", "--cpus=4", "--memory", "8g", "--env", "BASE=local", "-e", "OVERRIDE=local", "-l", "same=local", "--label", "local-only=yes", "--cap-add=SYS_PTRACE"}
	wantBuildOptions := []any{"--platform", "linux/amd64", "--no-cache", "--build-arg=BASE=local", "--build-arg", "OVERRIDE=local", "--platform=linux/arm64", "--progress", "plain"}
	for channel, config := range map[string]map[string]any{"preview": previewConfig, "CLI": cliConfig} {
		if !reflect.DeepEqual(config["runArgs"], wantRunArgs) {
			t.Errorf("%s runArgs = %#v, want %#v", channel, config["runArgs"], wantRunArgs)
		}
		build, ok := config["build"].(map[string]any)
		if !ok || !reflect.DeepEqual(build["options"], wantBuildOptions) {
			t.Errorf("%s build.options = %#v, want %#v", channel, build["options"], wantBuildOptions)
		}
	}
	if !reflect.DeepEqual(previewConfig["runArgs"], cliConfig["runArgs"]) {
		t.Errorf("preview runArgs differ from CLI: %#v vs %#v", previewConfig["runArgs"], cliConfig["runArgs"])
	}
	if !reflect.DeepEqual(previewConfig["build"], cliConfig["build"]) {
		t.Errorf("preview build differs from CLI: %#v vs %#v", previewConfig["build"], cliConfig["build"])
	}
	if got := string(mustReadFile(t, basePath)); got != base {
		t.Errorf("base config changed: %q", got)
	}
	if got := string(mustReadFile(t, localPath)); got != local {
		t.Errorf("local config changed: %q", got)
	}
}

func writeMergeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func captureProjectPreview(t *testing.T, project Project) string {
	t.Helper()
	previous := screenActive
	screenActive = false
	defer func() { screenActive = previous }()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = writer
	showErr := showProjectConfig(project)
	os.Stdout = previousStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if showErr != nil {
		t.Fatal(showErr)
	}
	return string(content)
}

func decodeConfig(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	return config
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
