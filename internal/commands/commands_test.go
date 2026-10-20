package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJSONCNameAndTrailingComma(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	sample := `{
      // My project
      "name": "api // not a comment, /* not a comment */",
      "build": {"args": ["x", "y",],}, /* trailing */
    }`
	if err := os.WriteFile(filepath.Join(root, ".devcontainer", "devcontainer.json"), []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := projectName(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := "api // not a comment, /* not a comment */"; name != want {
		t.Fatalf("got %q, want %q", name, want)
	}
}

func TestJSONCStringEscapes(t *testing.T) {
	sample := []byte(`{"name":"hello \" // there","items":[1,2,]}`)
	data, err := jsoncToJSON(sample)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Name  string `json:"name"`
		Items []int  `json:"items"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != `hello " // there` || !reflect.DeepEqual(v.Items, []int{1, 2}) {
		t.Fatalf("unexpected parse: %+v", v)
	}
}

func TestInvalidJSONC(t *testing.T) {
	if _, err := jsoncToJSON([]byte(`{"x": 1 /* missing end`)); err == nil {
		t.Fatal("expected unterminated comment error")
	}
	p := t.TempDir()
	if err := os.Mkdir(filepath.Join(p, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, ".devcontainer", "devcontainer.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := projectName(p); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestSaveLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".dch", "config")
	t.Setenv("DCH_CONFIG", path)
	want := Config{Version: 1, Projects: []string{"/tmp/work-a", "/tmp/work-b"}}
	if err := saveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("config permission: %o", fi.Mode().Perm())
	}
}

func TestMatchContainerAndStatus(t *testing.T) {
	path := "/home/user/repo"
	cs := []DockerContainer{{ID: "aa", Created: "2025-01-01"}, {ID: "bb", Created: "2025-02-01"}}
	for i := range cs {
		cs[i].Config.Labels = map[string]string{
			"devcontainer.local_folder": path,
			"devcontainer.config_file":  filepath.Join(path, ".devcontainer", "devcontainer.json"),
		}
	}
	cs[0].State.Status = "exited"
	cs[1].State.Running = true
	cs[1].State.Status = "running"
	c, ok, n := matchContainer(path, cs)
	if !ok || n != 2 || c.ID != "bb" {
		t.Fatalf("got %+v %v %d", c, ok, n)
	}
	cs[1].Config.Labels["devcontainer.config_file"] = "/different/config.json"
	_, _, n = matchContainer(path, cs)
	if n != 1 {
		t.Fatalf("config label filter failed, n=%d", n)
	}
}

func TestProjectNameFallbackAndMissing(t *testing.T) {
	parent := t.TempDir()
	p := filepath.Join(parent, "repo")
	if err := os.MkdirAll(filepath.Join(p, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(p, ".devcontainer", "devcontainer.json")
	if _, err := projectName(p); err == nil || !strings.Contains(err.Error(), "devcontainer.json") {
		t.Fatalf("got %v", err)
	}
	if err := os.WriteFile(cfg, []byte(`{"image":"ubuntu:22.04"}`), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := projectName(p)
	if err != nil || name != "repo" {
		t.Fatalf("got %q, %v", name, err)
	}
}
