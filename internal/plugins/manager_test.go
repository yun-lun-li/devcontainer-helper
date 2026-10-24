package plugins

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dch/internal/common"
	"dch/internal/githubapp"
)

func TestEnableValidatesCredentialOverUnixSocketAndMarksBuildPending(t *testing.T) {
	env := setupManagerTest(t, "ok")
	projectPath := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	env.setSocket(t, projectPath)
	cfg := validConfig()
	if err := NewManager().Enable(context.Background(), projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	normalized, _ := common.NormalizePath(projectPath)
	state := loaded.ProjectPlugins[normalized]
	if state.Enabled[githubAppID].AppID != cfg.AppID || !state.States[githubAppID].BuildPending || state.States[githubAppID].FragmentDigest == "" {
		t.Fatalf("enable did not persist config and Build state: %+v", state)
	}
	if _, err := os.Stat(managedFragment(normalized)); err != nil {
		t.Fatalf("managed fragment missing: %v", err)
	}
}

func TestEnableFailureRestoresPriorProjectConfigAndFragment(t *testing.T) {
	env := setupManagerTest(t, "fail")
	projectPath := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".devcontainer", "devcontainer.local.d"), 0700); err != nil {
		t.Fatal(err)
	}
	env.setSocket(t, projectPath)
	normalized, _ := common.NormalizePath(projectPath)
	oldCfg := validConfig()
	oldCfg.AppID = 3
	oldFragment := []byte(`{"old":true}`)
	fragmentPath := managedFragment(normalized)
	if err := os.WriteFile(fragmentPath, oldFragment, 0600); err != nil {
		t.Fatal(err)
	}
	old := common.ProjectPlugins{Enabled: map[string]githubapp.Config{githubAppID: oldCfg}, States: map[string]common.PluginState{githubAppID: {FragmentDigest: digest(oldFragment), BuildFingerprint: "old", BuildPending: false}}}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{}, ProjectPlugins: map[string]common.ProjectPlugins{normalized: old}}); err != nil {
		t.Fatal(err)
	}
	cfg := validConfig()
	cfg.AppID = 99
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := NewManager().Enable(ctx, projectPath, cfg); err == nil {
		t.Fatal("Enable succeeded despite credential failure")
	}
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.ProjectPlugins[normalized]
	if got.Enabled[githubAppID].AppID != oldCfg.AppID || got.States[githubAppID].BuildFingerprint != "old" || got.States[githubAppID].BuildPending {
		t.Fatalf("failed enable changed existing project config: %+v", got)
	}
	fragment, err := os.ReadFile(fragmentPath)
	if err != nil || string(fragment) != string(oldFragment) {
		t.Fatalf("failed enable did not restore fragment: %q, %v", fragment, err)
	}
}

func TestEnableCleanupFailureKeepsRetryableSettings(t *testing.T) {
	env := setupManagerTest(t, "fail")
	t.Setenv("DCH_TEST_FAIL_STOP_ONCE", filepath.Join(env.root, "stop-failed"))
	projectPath := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	env.setSocket(t, projectPath)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := NewManager().Enable(ctx, projectPath, validConfig()); err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("Enable error = %v, want cleanup failure", err)
	}
	normalized, _ := common.NormalizePath(projectPath)
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.ProjectPlugins[normalized].Enabled[githubAppID]; !ok {
		t.Fatal("retryable config was not retained")
	}
	if err := NewManager().Disable(context.Background(), projectPath); err != nil {
		t.Fatalf("retry disable: %v", err)
	}
	loaded, err = common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.ProjectPlugins[normalized].Enabled[githubAppID]; ok {
		t.Fatal("retry cleanup left plugin enabled")
	}
}

func TestEnableRestartsActiveServiceWhenConfigurationChanges(t *testing.T) {
	env := setupManagerTest(t, "ok")
	projectPath := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	env.setSocket(t, projectPath)
	manager := NewManager()
	cfg := validConfig()
	if err := manager.Enable(context.Background(), projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	first, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	normalized, _ := common.NormalizePath(projectPath)
	if _, ok := first.ProjectPlugins[normalized].Enabled[githubAppID]; !ok {
		t.Fatalf("first enable did not persist enabled state: %+v", first.ProjectPlugins)
	}
	firstFingerprint := first.ProjectPlugins[normalized].States[githubAppID].BuildFingerprint
	cfg.AppID = 42
	if err := manager.Enable(context.Background(), projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(env.root, "systemctl.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "restart") {
		debug, _ := os.ReadFile(filepath.Join(env.root, "service.debug"))
		t.Fatalf("active service was not restarted: %s / %s", log, debug)
	}
	normalized, _ = common.NormalizePath(projectPath)
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.ProjectPlugins[normalized].Enabled[githubAppID].AppID; got != 42 {
		t.Fatalf("AppID = %d, want 42", got)
	}
	if got := loaded.ProjectPlugins[normalized].States[githubAppID].BuildFingerprint; got == firstFingerprint {
		t.Fatal("configuration change did not update Build fingerprint")
	}
}

func TestStatusReportsPendingBuildAndInactiveService(t *testing.T) {
	env := setupManagerTest(t, "ok")
	path := filepath.Join(env.root, "project")
	normalized, _ := common.NormalizePath(path)
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{}, ProjectPlugins: map[string]common.ProjectPlugins{
		normalized: {Enabled: map[string]githubapp.Config{githubAppID: {}}, States: map[string]common.PluginState{githubAppID: {BuildPending: true}}},
	}}); err != nil {
		t.Fatal(err)
	}
	status, err := NewManager().Status(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.PendingBuild || status.ServiceActive {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestDisableRefusesEditedFragmentBeforeStoppingService(t *testing.T) {
	env := setupManagerTest(t, "ok")
	projectPath := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Dir(managedFragment(projectPath)), 0700); err != nil {
		t.Fatal(err)
	}
	fragmentPath := managedFragment(projectPath)
	if err := os.WriteFile(fragmentPath, []byte(`{"manual":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	normalized, _ := common.NormalizePath(projectPath)
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{}, ProjectPlugins: map[string]common.ProjectPlugins{
		normalized: {Enabled: map[string]githubapp.Config{githubAppID: validConfig()}, States: map[string]common.PluginState{githubAppID: {FragmentDigest: digest([]byte(`{"generated":true}`))}}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := NewManager().Disable(context.Background(), projectPath); err == nil {
		t.Fatal("Disable accepted a manually edited fragment")
	}
	data, err := os.ReadFile(fragmentPath)
	if err != nil || string(data) != `{"manual":true}` {
		t.Fatalf("edited fragment changed: %q %v", data, err)
	}
	log, err := os.ReadFile(filepath.Join(env.root, "systemctl.log"))
	if err == nil && strings.Contains(string(log), "stop") {
		t.Fatalf("service was stopped before ownership check: %s", log)
	}
	loaded, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.ProjectPlugins[normalized].Enabled[githubAppID]; !ok {
		t.Fatal("enabled config was lost")
	}
}

func TestEnablePreservesForeignSystemdUnit(t *testing.T) {
	env := setupManagerTest(t, "ok")
	projectPath := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	env.setSocket(t, projectPath)
	unit, err := unitPath(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("foreign unit\n")
	if err := os.WriteFile(unit, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewManager().Enable(context.Background(), projectPath, validConfig()); err == nil {
		t.Fatal("Enable overwrote a foreign unit")
	}
	data, err := os.ReadFile(unit)
	if err != nil || string(data) != string(foreign) {
		t.Fatalf("foreign unit changed: %q, %v", data, err)
	}
	log, _ := os.ReadFile(filepath.Join(env.root, "systemctl.log"))
	if strings.Contains(string(log), "stop") || strings.Contains(string(log), "disable") {
		t.Fatalf("foreign unit was managed: %s", log)
	}
}

func TestCredentialServiceHelper(t *testing.T) {
	if os.Getenv("DCH_TEST_HELPER") != "1" {
		return
	}
	socket := os.Getenv("DCH_TEST_SOCKET")
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(2)
	}
	_ = os.Chmod(socket, 0600)
	status := http.StatusOK
	if os.Getenv("DCH_TEST_MODE") == "fail" {
		status = http.StatusServiceUnavailable
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status); _, _ = w.Write([]byte("token")) })}
	_ = server.Serve(listener)
	os.Exit(0)
}

type managerTestEnv struct {
	root    string
	runtime string
}

func (env managerTestEnv) setSocket(t *testing.T, projectPath string) {
	t.Helper()
	dir, err := githubapp.RuntimeDir(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_TEST_SOCKET", filepath.Join(dir, "credential.sock"))
}

func setupManagerTest(t *testing.T, mode string) managerTestEnv {
	t.Helper()
	root := t.TempDir()
	runtime, err := os.MkdirTemp("/tmp", "dchrt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
	configHome := filepath.Join(root, "config-home")
	if err := os.Mkdir(configHome, 0700); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(root, "service.pid")
	logPath := filepath.Join(root, "systemctl.log")
	debugPath := filepath.Join(root, "service.debug")
	script := `#!/bin/sh
shift
action=$1
echo "$action" >> "$DCH_TEST_LOG"
case "$action" in
  daemon-reload) exit 0 ;;
  is-active) [ -f "$DCH_TEST_PID" ] && kill -0 "$(cat "$DCH_TEST_PID")" 2>/dev/null ; status=$?; echo "active=$status pid=$(cat "$DCH_TEST_PID" 2>/dev/null)" >> "$DCH_TEST_DEBUG"; exit "$status" ;;
  stop) if [ -n "$DCH_TEST_FAIL_STOP_ONCE" ] && [ ! -f "$DCH_TEST_FAIL_STOP_ONCE.done" ]; then touch "$DCH_TEST_FAIL_STOP_ONCE.done"; exit 1; fi; if [ -f "$DCH_TEST_PID" ]; then kill "$(cat "$DCH_TEST_PID")" 2>/dev/null || true; rm -f "$DCH_TEST_PID"; fi; exit 0 ;;
  disable) if [ -f "$DCH_TEST_PID" ]; then kill "$(cat "$DCH_TEST_PID")" 2>/dev/null || true; rm -f "$DCH_TEST_PID"; fi; exit 0 ;;
  enable|restart|start)
    if [ -f "$DCH_TEST_PID" ]; then kill "$(cat "$DCH_TEST_PID")" 2>/dev/null || true; rm -f "$DCH_TEST_PID"; fi
    DCH_TEST_HELPER=1 "$DCH_TEST_BINARY" -test.run '^TestCredentialServiceHelper$' >> "$DCH_TEST_DEBUG" 2>&1 &
    echo $! > "$DCH_TEST_PID"
    sleep 0.03
    exit 0 ;;
esac
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+oldPath)
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("DCH_TEST_BINARY", binary)
	t.Setenv("DCH_TEST_PID", pidPath)
	t.Setenv("DCH_TEST_LOG", logPath)
	t.Setenv("DCH_TEST_DEBUG", debugPath)
	t.Setenv("DCH_TEST_FAIL_STOP_ONCE", "")
	t.Setenv("DCH_TEST_SOCKET", "")
	t.Setenv("DCH_TEST_MODE", mode)
	t.Setenv("DCH_TEST_HELPER", "")
	return managerTestEnv{root: root, runtime: runtime}
}

func validConfig() githubapp.Config {
	return githubapp.Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "github/key", Repository: "owner/repo", Permissions: map[string]string{"contents": "read"}}
}
