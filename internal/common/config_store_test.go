package common

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigRoundTripsProjectPluginsAndLoadsLegacyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	t.Setenv("DCH_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"version":1,"projects":["/old"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil || len(cfg.Projects) != 1 || len(cfg.ProjectPlugins) != 0 {
		t.Fatalf("legacy config = %#v, %v", cfg, err)
	}
	cfg.ProjectPlugins = map[string]ProjectPlugins{}
	cfg.PluginSettings = map[string]json.RawMessage{"plugin-a": json.RawMessage(`{"opaque":[1,true]}`)}
	cfg.ProjectPlugins["/project"] = ProjectPlugins{
		Enabled: map[string]json.RawMessage{"plugin-a": json.RawMessage(`{"setting":"value"}`)},
		States:  map[string]PluginState{"plugin-a": {Data: json.RawMessage(`{"private":"state"}`), BuildPending: true}},
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.ProjectPlugins["/project"]
	var sharedGot, sharedWant any
	if err := json.Unmarshal(loaded.PluginSettings["plugin-a"], &sharedGot); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(json.RawMessage(`{"opaque":[1,true]}`), &sharedWant); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sharedGot, sharedWant) {
		t.Fatalf("plugin settings = %s", loaded.PluginSettings["plugin-a"])
	}
	if !sameJSON(got.Enabled["plugin-a"], `{"setting":"value"}`) || !sameJSON(got.States["plugin-a"].Data, `{"private":"state"}`) || !got.States["plugin-a"].BuildPending {
		t.Fatalf("plugin state did not round trip: %#v", got)
	}
}

func sameJSON(got json.RawMessage, want string) bool {
	var compact bytes.Buffer
	if err := json.Compact(&compact, got); err != nil {
		return false
	}
	return compact.String() == want
}

func TestLegacyPluginStateMovesUnknownFieldsIntoOpaqueData(t *testing.T) {
	var state PluginState
	if err := json.Unmarshal([]byte(`{"fragment_digest":"fragment","unit_digest":"unit","unit_path":"service","build_pending":true}`), &state); err != nil {
		t.Fatal(err)
	}
	if !state.BuildPending || !sameJSON(state.Data, `{"fragment_digest":"fragment","unit_digest":"unit","unit_path":"service"}`) {
		t.Fatalf("legacy state was not preserved: %#v", state)
	}
	configPath := filepath.Join(t.TempDir(), "config")
	t.Setenv("DCH_CONFIG", configPath)
	if err := SaveConfig(Config{Version: 1, ProjectPlugins: map[string]ProjectPlugins{"/project": {States: map[string]PluginState{"plugin-a": state}}}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.ProjectPlugins["/project"].States["plugin-a"]
	if !got.BuildPending || !sameJSON(got.Data, `{"fragment_digest":"fragment","unit_digest":"unit","unit_path":"service"}`) {
		t.Fatalf("legacy state changed after config round trip: %#v", got)
	}
}

func TestPluginStateDataPreservesAnyJSONShapeAndReservedNames(t *testing.T) {
	for _, value := range []string{`["fragment",7]`, `"scalar"`, `{"build_pending":false,"data":"plugin-owned"}`} {
		t.Run(value, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config")
			t.Setenv("DCH_CONFIG", configPath)
			state := PluginState{BuildPending: true, Data: json.RawMessage(value)}
			if err := SaveConfig(Config{Version: 1, ProjectPlugins: map[string]ProjectPlugins{"/project": {States: map[string]PluginState{"plugin-a": state}}}}); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			got := loaded.ProjectPlugins["/project"].States["plugin-a"]
			if !got.BuildPending || !sameJSON(got.Data, value) {
				t.Fatalf("opaque state = %#v, want %s", got, value)
			}
		})
	}
}
