package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dch/internal/features"
)

func TestInitializeRunsResolvedHooksInOrderWithOptions(t *testing.T) {
	workspace := t.TempDir()
	first := feature("first", `printf '%s\n' "$DCH_FEATURE_OPTION_MESSAGE" "$DCH_FEATURE_OPTION_ENABLED" "$PWD" > result`)
	first["options"] = map[string]any{"message": map[string]any{"default": "default"}, "enabled": map[string]any{"default": true}}
	first["value"] = map[string]any{"message": "literal $(touch injected)"}
	second := feature("second", []string{"sh", "-c", "printf 'second\\n' >> result"})
	installResolver(t, []map[string]any{first, second})
	var output bytes.Buffer
	if err := features.Initialize(context.Background(), workspace, "/tmp/config.json", nil, &output, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "result"))
	if err != nil {
		t.Fatal(err)
	}
	want := "literal $(touch injected)\ntrue\n" + workspace + "\nsecond\n"
	if string(data) != want {
		t.Fatalf("result = %q, want %q", data, want)
	}
	if _, err := os.Stat(filepath.Join(workspace, "injected")); !os.IsNotExist(err) {
		t.Fatal("option was interpreted as shell code")
	}
}

func TestInitializeStopsAfterHookFailure(t *testing.T) {
	workspace := t.TempDir()
	installResolver(t, []map[string]any{feature("failure", "exit 9"), feature("skipped", "touch unexpected")})
	var output bytes.Buffer
	err := features.Initialize(context.Background(), workspace, "/tmp/config.json", nil, &output, &output)
	if err == nil || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "unexpected")); !os.IsNotExist(err) {
		t.Fatal("ran hook after failure")
	}
}

func TestInitializeCancellationStopsShellChildren(t *testing.T) {
	workspace := t.TempDir()
	installResolver(t, []map[string]any{feature("waiting", "sleep 30 & wait")})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	started := time.Now()
	if err := features.Initialize(ctx, workspace, "/tmp/config.json", nil, &output, &output); err == nil {
		t.Fatal("canceled hook succeeded")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("cancellation waited for shell children")
	}
}

func TestInitializeRunsObjectCommandsConcurrently(t *testing.T) {
	workspace := t.TempDir()
	hooks := map[string]any{
		"first":  "touch first; while [ ! -f second ]; do sleep 0.01; done",
		"second": []string{"sh", "-c", "touch second; while [ ! -f first ]; do sleep 0.01; done"},
	}
	installResolver(t, []map[string]any{feature("parallel", hooks)})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := features.Initialize(ctx, workspace, "/tmp/config.json", nil, &output, &output); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeRejectsInvalidCommandsAndOptionValues(t *testing.T) {
	cases := []struct {
		name  string
		hook  any
		value map[string]any
	}{
		{name: "empty command", hook: []any{}},
		{name: "nonstring argument", hook: []any{"sh", 7}},
		{name: "nested parallel", hook: map[string]any{"nested": map[string]any{"command": "true"}}},
		{name: "numeric option", hook: "true", value: map[string]any{"count": 7}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			item := feature("invalid", test.hook)
			if test.value != nil {
				item["value"] = test.value
			}
			installResolver(t, []map[string]any{item})
			var output bytes.Buffer
			if err := features.Initialize(context.Background(), t.TempDir(), "/tmp/config.json", nil, &output, &output); err == nil {
				t.Fatal("invalid hook accepted")
			}
		})
	}
}

func TestInitializeIgnoresFeaturesWithoutHostHooks(t *testing.T) {
	item := feature("ordinary", nil)
	delete(item, "customizations")
	installResolver(t, []map[string]any{item})
	var output bytes.Buffer
	if err := features.Initialize(context.Background(), t.TempDir(), "/tmp/config.json", nil, &output, &output); err != nil {
		t.Fatal(err)
	}
}

func feature(id string, hook any) map[string]any {
	return map[string]any{"id": id, "cachePath": "/tmp/feature", "value": map[string]any{},
		"customizations": map[string]any{"dch": map[string]any{"initializeCommand": hook}}}
}

func installResolver(t *testing.T, items []map[string]any) {
	t.Helper()
	var sets []any
	for _, item := range items {
		sets = append(sets, map[string]any{"features": []any{item}})
	}
	raw, err := json.Marshal(map[string]any{"featuresConfiguration": map[string]any{"featureSets": sets}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	metadata := filepath.Join(dir, "resolved.json")
	if err := os.WriteFile(metadata, raw, 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$1\" = read-configuration ] || exit 4\ncat \"$DCH_TEST_FEATURE_METADATA\"\n"
	if err := os.WriteFile(filepath.Join(dir, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_TEST_FEATURE_METADATA", metadata)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestInitializeSupportsVersionShorthandAndFeatureScriptDirectory(t *testing.T) {
	workspace := t.TempDir()
	source := t.TempDir()
	script := "printf '%s' \"$DCH_FEATURE_OPTION_VERSION\" > version\n"
	if err := os.WriteFile(filepath.Join(source, "initialize.sh"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	item := feature("versioned", "sh \"$DCH_FEATURE_DIR/initialize.sh\"")
	item["cachePath"] = source
	item["value"] = "1.2.3"
	installResolver(t, []map[string]any{item})
	var output bytes.Buffer
	if err := features.Initialize(context.Background(), workspace, "/tmp/config.json", nil, &output, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "version"))
	if err != nil || string(data) != "1.2.3" {
		t.Fatalf("script version = %q, error = %v", data, err)
	}
}
