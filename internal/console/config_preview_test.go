package console

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowProjectConfigMentionsLocalFragments(t *testing.T) {
	previousStdout := os.Stdout
	previousScreenActive := screenActive
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	screenActive = false
	t.Cleanup(func() {
		os.Stdout = previousStdout
		screenActive = previousScreenActive
	})

	projectPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectPath, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".devcontainer", "devcontainer.json"), []byte(`{"image":"ubuntu"}`), 0600); err != nil {
		t.Fatal(err)
	}
	err = showProjectConfig(Project{Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previousStdout
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), ".devcontainer/devcontainer.local.d/") {
		t.Fatalf("preview does not mention local.d: %s", output)
	}
	if strings.Contains(string(output), "devcontainer.local.json") {
		t.Fatalf("preview mentions legacy local file: %s", output)
	}
}
