package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectUpInitializesFeaturesBeforeCLIAndPreservesUserHook(t *testing.T) {
	project := installFeatureProject(t, "printf 'feature\\n' >> \"$DCH_TEST_SEQUENCE\"")
	if err := invokeProjectDevcontainer(project, "up", "--workspace-folder", project.Path); err != nil {
		t.Fatal(err)
	}
	sequence, err := os.ReadFile(os.Getenv("DCH_TEST_SEQUENCE"))
	if err != nil {
		t.Fatal(err)
	}
	if string(sequence) != "feature\nup\n" {
		t.Fatalf("sequence = %q", sequence)
	}
	config, err := os.ReadFile(os.Getenv("DCH_CAPTURE_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(config, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["initializeCommand"] != "echo user hook" {
		t.Fatalf("user hook changed: %v", settings["initializeCommand"])
	}
}

func TestProjectUpAbortsWhenFeatureInitializationFails(t *testing.T) {
	project := installFeatureProject(t, "exit 3")
	err := invokeProjectDevcontainer(project, "up", "--workspace-folder", project.Path)
	if err == nil || !strings.Contains(err.Error(), "host initialization") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(os.Getenv("DCH_TEST_SEQUENCE")); !os.IsNotExist(err) {
		t.Fatal("up executed after hook failed")
	}
}

func installFeatureProject(t *testing.T, hook string) Project {
	t.Helper()
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := `{"image":"debian:bookworm","features":{"./features/example":{}},"initializeCommand":"echo user hook"}`
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	feature := map[string]any{"id": "example", "value": map[string]any{}, "cachePath": configDir,
		"customizations": map[string]any{"dch": map[string]any{"initializeCommand": hook}}}
	metadata := map[string]any{"featuresConfiguration": map[string]any{"featureSets": []any{map[string]any{"features": []any{feature}}}}}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "metadata.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = read-configuration ]; then cat "$DCH_TEST_METADATA"; exit; fi
printf '%s\n' "$1" >> "$DCH_TEST_SEQUENCE"
while [ $# -gt 0 ]; do
 if [ "$1" = --config ]; then cp "$2" "$DCH_CAPTURE_CONFIG"; fi
 shift
done
`
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DCH_TEST_METADATA", filepath.Join(project, "metadata.json"))
	t.Setenv("DCH_TEST_SEQUENCE", filepath.Join(project, "sequence"))
	t.Setenv("DCH_CAPTURE_CONFIG", filepath.Join(project, "captured-config.json"))
	return Project{Path: project, Name: "example"}
}
