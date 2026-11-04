package common

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMergedDevcontainerConfigMergesRunArgsByOption(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(configDir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig("devcontainer.json", `{"runArgs":["--name","base","--env","A=1","--env","B=2","--rm"]}`)
	writeConfig("devcontainer.local.json", `{"runArgs":["-e","A=local","--rm=false"]}`)

	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--name","base","--env","B=2","-e","A=local","--rm=false"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergeConfigMountsOverridesBaseByTarget(t *testing.T) {
	base := []any{
		"source=base,target=/shared,type=volume",
		map[string]any{"source": "base-data", "target": "/data", "type": "volume"},
		"source=keep,target=/keep,type=volume",
		"source=unknown,type=volume",
	}
	local := []any{
		map[string]any{"source": "local-shared", "target": "/shared", "type": "volume"},
		"source=local-data,dst=/data,type=volume",
		"source=new,target=/new,type=volume",
		"source=local-unknown,type=volume",
	}
	got := mergeConfigValues(map[string]any{"mounts": base}, map[string]any{"mounts": local})
	want := map[string]any{"mounts": []any{base[2], base[3], local[0], local[1], local[2], local[3]}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged config = %#v, want %#v", got, want)
	}
}

func TestMergeConfigEmptyLocalMountsKeepsBase(t *testing.T) {
	base := map[string]any{"mounts": []any{"source=data,target=/data,type=volume"}}
	got := mergeConfigValues(base, map[string]any{"mounts": []any{}})
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("merged config = %#v, want %#v", got, base)
	}
}

func TestMountTarget(t *testing.T) {
	tests := []struct {
		name  string
		mount any
		want  string
	}{
		{"target", "source=data,target=/data,type=volume", "/data"},
		{"dst", "dst=/data,source=data", "/data"},
		{"destination", "destination=/data,source=data", "/data"},
		{"quoted CSV", `"source=host,dir","target=/data,dir",type=bind`, "/data,dir"},
		{"object", map[string]any{"target": "/data"}, "/data"},
		{"missing target", "source=data,type=volume", ""},
		{"invalid object target", map[string]any{"target": 123}, ""},
		{"invalid CSV", `source="broken,target=/data`, ""},
		{"multiple records", "target=/data\ntarget=/other", ""},
		{"invalid type", 123, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mountTarget(tt.mount); got != tt.want {
				t.Fatalf("mountTarget(%#v) = %q, want %q", tt.mount, got, tt.want)
			}
		})
	}
}

func compactJSON(t *testing.T, data []byte) string {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatal(err)
	}
	return compact.String()
}

func TestMergedDevcontainerConfigOverridesLifecycleCommandAsWholeValue(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"postCreateCommand":{"npm":"install","go":"mod download"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"postCreateCommand":{"npm":"ci"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, got) != `{"postCreateCommand":{"npm":"ci"}}` {
		t.Fatalf("merged config = %s", compactJSON(t, got))
	}
}

func TestMergedDevcontainerConfigAppendsComposeAndOverridesUnknownArrays(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"dockerComposeFile":["base.yml"],"custom":[1,2]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"dockerComposeFile":["local.yml"],"custom":[2,3]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"custom":[2,3],"dockerComposeFile":["base.yml","local.yml"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigMergesBuildOptionsByKeyAndKeepsAppendOptions(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"build":{"options":["--build-arg","A=base","--build-arg=B=base","--platform","linux/amd64"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"build":{"options":["--build-arg=A=local","--platform=linux/arm64","--progress","tty"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"build":{"options":["--build-arg=B=base","--platform","linux/amd64","--build-arg=A=local","--platform=linux/arm64","--progress","tty"]}}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigKeepsUnknownDockerSuffixAndBaseDuplicates(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--rm","--rm","--future-flag","--memory","4g"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--memory","8g"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--rm","--rm","--future-flag","--memory","4g","--memory","8g"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigParsesAttachedShortValuesAndAliases(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["-m4g","-eA=base","--platform","linux/amd64"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--memory=8g","--env=A=local","--platform","linux/arm64"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--memory=8g","--env=A=local","--platform","linux/arm64"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigKeepsOptionKeyNamespacesSeparate(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--env=A=base","--label=A=base"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--env=A=local"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--label=A=base","--env=A=local"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigPreservesRepeatedBaseTokensWithoutLocalFile(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--rm","--rm"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, got) != `{"runArgs":["--rm","--rm"]}` {
		t.Fatalf("merged config = %s", compactJSON(t, got))
	}
}

func TestMergedDevcontainerConfigDeduplicatesOnlyKnownSetArrays(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"customizations":{"vscode":{"extensions":["a","a"]}},"unknown":["x","x"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"customizations":{"vscode":{"extensions":["a","b"]}},"unknown":["y","y"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"customizations":{"vscode":{"extensions":["a","b"]}},"unknown":["y","y"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigPreservesOtherShortBooleanInBundle(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["-it"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--interactive=false"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["-t","--interactive=false"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigAccumulatesLinkLocalIPs(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--link-local-ip","169.254.1.1"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--link-local-ip","169.254.1.2"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--link-local-ip","169.254.1.1","--link-local-ip","169.254.1.2"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigOverridesSingleValueBuildAttestations(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"build":{"options":["--sbom","true","--provenance=true"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"build":{"options":["--sbom=false","--provenance","false"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"build":{"options":["--sbom=false","--provenance","false"]}}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigAccumulatesNetworkOptions(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--network","front"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--network=back"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--network","front","--network=back"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigAccumulatesCustomListOptions(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--blkio-weight-device","/dev/sda:100","--gpus","device=0"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--blkio-weight-device=/dev/sdb:200","--gpus=all"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--blkio-weight-device","/dev/sda:100","--gpus","device=0","--blkio-weight-device=/dev/sdb:200","--gpus=all"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}

func TestMergedDevcontainerConfigMergesRunAnnotationsByKey(t *testing.T) {
	project := t.TempDir()
	configDir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(`{"runArgs":["--annotation","org.example.same=base","--annotation","org.example.other=base"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "devcontainer.local.json"), []byte(`{"runArgs":["--annotation=org.example.same=local"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := MergedDevcontainerConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runArgs":["--annotation","org.example.other=base","--annotation=org.example.same=local"]}`
	if compactJSON(t, got) != want {
		t.Fatalf("merged config = %s, want %s", compactJSON(t, got), want)
	}
}
