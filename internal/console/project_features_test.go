package console

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/features"
)

func TestProjectFeaturesMenuEnablesAndDisables(t *testing.T) {
	project := newFeatureMenuProject(t)
	output, err := runFeatureMenu(t, project, "1\nproject/github\n2\n")
	if err != nil {
		t.Fatal(err)
	}
	assertMenuFeatureEnabled(t, project, true)
	if !strings.Contains(output, "Disable GitHub CLI with host pass  [project/github]") {
		t.Fatalf("missing enabled status: %s", output)
	}
	t.Log(output)
	output, err = runFeatureMenu(t, project, "1\n2\n")
	if err != nil {
		t.Fatal(err)
	}
	assertMenuFeatureEnabled(t, project, false)
	if !strings.Contains(output, "Use Build to apply") {
		t.Fatalf("missing rebuild notice: %s", output)
	}
}

func TestProjectFeaturesMenuCancelDoesNotWriteSettings(t *testing.T) {
	project := newFeatureMenuProject(t)
	_, err := runFeatureMenu(t, project, "1\nq\n")
	if !errors.Is(err, errCancelled) {
		t.Fatalf("got %v", err)
	}
	_, err = os.Stat(filepath.Join(project.Path, ".devcontainer", "devcontainer.local.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected local config: %v", err)
	}
}

func newFeatureMenuProject(t *testing.T) Project {
	t.Helper()
	project := Project{Name: "demo", Path: t.TempDir(), Valid: true}
	dir := filepath.Join(project.Path, ".devcontainer")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	return project
}

func assertMenuFeatureEnabled(t *testing.T, project Project, want bool) {
	t.Helper()
	enabled, _, err := features.GitHubCLISettings(project.Path)
	if err != nil {
		t.Fatal(err)
	}
	if enabled != want {
		t.Fatalf("enabled = %v, want %v", enabled, want)
	}
}

func runFeatureMenu(t *testing.T, project Project, answers string) (string, error) {
	t.Helper()
	previousInput := input
	input = bufio.NewReader(strings.NewReader(answers))
	defer func() { input = previousInput }()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	previousStdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previousStdout }()
	menuErr := projectFeatures(project)
	writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output), menuErr
}
