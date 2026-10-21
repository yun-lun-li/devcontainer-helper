package commands

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"dch/internal/common"
)

func TestRegisterRelativePathAndRejectExtraArguments(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".devcontainer", "devcontainer.json"), []byte(`{"name":"example"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, project)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Run([]string{"register", "--path", relative}, nil); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0] != project || !filepath.IsAbs(cfg.Projects[0]) {
		t.Fatalf("registration did not store one absolute path: %v", cfg.Projects)
	}
	for _, args := range [][]string{
		{"register"}, {"register", "--path"}, {"register", "--path", project, "extra"},
		{"register", "--wrong", project}, {"register", "--path", " "},
		{"config"}, {"build"}, {"up"}, {"connect"},
	} {
		if err := Run(args, nil); err == nil {
			t.Errorf("Run(%v) should reject this command", args)
		}
	}
}

func TestRemoveProjectOnlyChangesRegistration(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(project, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}}); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	previousStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = previousStdout }()
	if err := common.RemoveProjectRegistration(project); err != nil {
		t.Fatal(err)
	}
	w.Close()
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 0 {
		t.Fatalf("unregister prompted or printed another screen: %s", output)
	}
	cfg, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 0 {
		t.Fatalf("registration was not removed: %v", cfg.Projects)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("project file was touched: %v", err)
	}
}
