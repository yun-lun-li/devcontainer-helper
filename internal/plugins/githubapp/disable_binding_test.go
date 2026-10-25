package githubapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dch/internal/plugins"
)

func TestDisableRejectsDifferentProjectBindingBeforeAnySideEffects(t *testing.T) {
	env := setupHostHookTest(t, true)
	projectA, projectB := t.TempDir(), t.TempDir()
	fragment := []byte(`{"owned":true}`)
	fragmentPath := managedFragment(projectB)
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, fragment, 0600); err != nil {
		t.Fatal(err)
	}
	unit, err := prepareServiceUnit(projectB, ownershipState{})
	if err != nil {
		t.Fatal(err)
	}
	state := ownershipState{FragmentDigest: digest(fragment), UnitDigest: unit.digest, UnitPath: unit.path}
	stateRaw := encodeState(state)
	cfg := Config{ClientID: "Iv1.example", ProjectPath: projectA, UserID: 12, Login: "octo", RepositoryID: 34, Repository: "octo/repo"}
	configRaw, _ := json.Marshal(cfg)
	passFile := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(passFile, []byte("credential-A"), 0600); err != nil {
		t.Fatal(err)
	}
	passPath := filepath.Join(t.TempDir(), "pass")
	passScript := "#!/bin/sh\ncase \"$1\" in\nshow) cat \"$DCH_FAKE_PASS_FILE\" ;;\nrm) rm -f \"$DCH_FAKE_PASS_FILE\" ;;\n*) exit 0 ;;\nesac\n"
	if err := os.WriteFile(passPath, []byte(passScript), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_FAKE_PASS_FILE", passFile)
	store := &testScopedStore{state: stateRaw}
	plugin := &Plugin{PassPath: passPath}
	_, err = plugin.disable(context.Background(), plugins.HostContext{ProjectPath: projectB, Config: configRaw, State: stateRaw, Store: store})
	if err == nil || !strings.Contains(err.Error(), "project binding") {
		t.Fatalf("disable() error = %v", err)
	}
	if got := readTestFile(t, fragmentPath); got != string(fragment) {
		t.Fatalf("fragment changed: %q", got)
	}
	if got, readErr := os.ReadFile(unit.path); readErr != nil || digest(got) != unit.digest {
		t.Fatalf("unit changed: %v", readErr)
	}
	if got := readTestFile(t, passFile); got != "credential-A" {
		t.Fatalf("Project A credential changed: %q", got)
	}
	if got := readSystemctlLog(t, env); got != "" {
		t.Fatalf("systemctl calls = %q", got)
	}
	if string(store.state) != string(stateRaw) {
		t.Fatalf("saved state changed: %s", store.state)
	}
}

func readSystemctlLog(t *testing.T, env hostHookTestEnv) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(env.root, "systemctl.log"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
