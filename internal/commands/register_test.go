package commands

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if err := Run([]string{"register", "--path", relative}); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := loadConfig()
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
		if err := Run(args); err == nil {
			t.Errorf("Run(%v) should reject this command", args)
		}
	}
}

func TestRemoveProjectOnlyChangesRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		wantRemoved  bool
	}{
		{"cancel", "n\n", false},
		{"confirm", "y\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			if err := saveConfig(Config{Version: 1, Projects: []string{project}}); err != nil {
				t.Fatal(err)
			}
			previous := input
			input = bufio.NewReader(strings.NewReader(tc.answer))
			t.Cleanup(func() { input = previous })
			removed, err := removeProjectRegistration(Project{Name: "project", Path: project})
			if err != nil || removed != tc.wantRemoved {
				t.Fatalf("removed = %v, error = %v", removed, err)
			}
			cfg, err := loadConfig()
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if tc.wantRemoved {
				wantCount = 0
			}
			if len(cfg.Projects) != wantCount {
				t.Fatalf("unexpected registrations: %v", cfg.Projects)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("project file was touched: %v", err)
			}
		})
	}
}
