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
	installFeatureSetupTools(t, project)
	output, err := runFeatureMenu(t, project, "1\na\nproject/github\nq\n")
	if err != nil {
		t.Fatal(err)
	}
	assertMenuFeatureEnabled(t, project, true)
	if !strings.Contains(output, "[x] GitHub CLI with host pass  (project/github)") {
		t.Fatalf("missing enabled status: %s", output)
	}
	t.Log(output)
	output, err = runFeatureMenu(t, project, "1\na\nq\n")
	if err != nil {
		t.Fatal(err)
	}
	assertMenuFeatureEnabled(t, project, false)
	if !strings.Contains(output, "Use Build to apply") {
		t.Fatalf("missing rebuild notice: %s", output)
	}
}

func installFeatureSetupTools(t *testing.T, project Project) {
	t.Helper()
	bin := filepath.Join(project.Path, "bin")
	store := filepath.Join(project.Path, "store")
	for _, dir := range []string{bin, filepath.Join(store, "project")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"pass", "gpg", "gpg-agent", "pinentry"} {
		script := "#!/bin/sh\n[ -z \"$DCH_SETUP_FAIL\" ] || exit 1\nprintf ghp_TEST_ONLY\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(store, ".gpg-id"), filepath.Join(store, "project", "github.gpg")} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PASSWORD_STORE_DIR", store)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(project.Path, "runtime"))
}

func TestProjectFeaturesSetupFailureDoesNotEnableFeature(t *testing.T) {
	project := newFeatureMenuProject(t)
	installFeatureSetupTools(t, project)
	t.Setenv("DCH_SETUP_FAIL", "1")
	_, err := runFeatureMenu(t, project, "1\na\nproject/github\n")
	if err == nil {
		t.Fatal("enabled Feature after setup failed")
	}
	assertMenuFeatureEnabled(t, project, false)
	_, err = os.Stat(filepath.Join(project.Path, ".devcontainer", "devcontainer.local.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected local config: %v", err)
	}
}

func TestProjectFeaturesMenuCancelDoesNotWriteSettings(t *testing.T) {
	project := newFeatureMenuProject(t)
	_, err := runFeatureMenu(t, project, "1\na\nq\n")
	if !errors.Is(err, errCancelled) {
		t.Fatalf("got %v", err)
	}
	_, err = os.Stat(filepath.Join(project.Path, ".devcontainer", "devcontainer.local.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected local config: %v", err)
	}
}

func TestProjectFeaturesDiscardDoesNotRunSetup(t *testing.T) {
	project := newFeatureMenuProject(t)
	_, err := runFeatureMenu(t, project, "1\nq\n")
	if err != nil {
		t.Fatal(err)
	}
	assertMenuFeatureEnabled(t, project, false)
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
