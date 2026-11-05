package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newMenuProject(t *testing.T, base string) string {
	t.Helper()
	project := t.TempDir()
	dir := filepath.Join(project, ".devcontainer")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devcontainer.json"), []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	return project
}

func TestGitHubCLIMenuSettingsArePerProject(t *testing.T) {
	first := newMenuProject(t, `{}`)
	second := newMenuProject(t, `{}`)
	if err := EnableGitHubCLI(first, "project/first"); err != nil {
		t.Fatal(err)
	}
	assertGitHubCLISettings(t, first, true, "project/first")
	assertGitHubCLISettings(t, second, false, "dch/github/pat")
	for _, name := range []string{"devcontainer-feature.json", "install.sh", "gh-wrapper.sh", "credential-helper.sh"} {
		path := filepath.Join(first, ".devcontainer", ".dch", "features", "github-cli", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := DisableGitHubCLI(first); err != nil {
		t.Fatal(err)
	}
	assertGitHubCLISettings(t, first, false, "dch/github/pat")
}

func assertGitHubCLISettings(t *testing.T, project string, wantEnabled bool, wantEntry string) {
	t.Helper()
	enabled, entry, err := GitHubCLISettings(project)
	if err != nil {
		t.Fatal(err)
	}
	if enabled != wantEnabled || entry != wantEntry {
		t.Fatalf("got %v %q", enabled, entry)
	}
}

func TestGitHubCLIMenuProtectsBaseConfiguration(t *testing.T) {
	base := `{"features":{"` + GitHubCLIKey + `":{"passEntry":"shared"}}}`
	project := newMenuProject(t, base)
	if err := DisableGitHubCLI(project); err == nil {
		t.Fatal("removed base Feature")
	}
	if err := EnableGitHubCLI(project, "new"); err == nil {
		t.Fatal("overrode base Feature")
	}
	data, err := os.ReadFile(filepath.Join(project, ".devcontainer", "devcontainer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != base {
		t.Fatalf("base changed: %s", data)
	}
}

func TestGitHubCLIMenuRejectsBrokenLocalConfiguration(t *testing.T) {
	project := newMenuProject(t, `{}`)
	path := filepath.Join(project, ".devcontainer", "devcontainer.local.json")
	if err := os.WriteFile(path, []byte(`{"features":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EnableGitHubCLI(project, "entry"); err == nil {
		t.Fatal("accepted invalid local config")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), ":") {
		t.Fatalf("changed invalid config: %s", data)
	}
}

func TestGitHubCLIMenuRejectsDuplicateManualFeature(t *testing.T) {
	project := newMenuProject(t, `{"features":{"./features/github-cli":{}}}`)
	if err := EnableGitHubCLI(project, "project/token"); err == nil {
		t.Fatal("enabled a duplicate GitHub CLI host pass Feature")
	}
	_, err := os.Stat(filepath.Join(project, ".devcontainer", "devcontainer.local.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected local config: %v", err)
	}
}
