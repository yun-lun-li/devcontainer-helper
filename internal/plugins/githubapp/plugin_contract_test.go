package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dch/internal/plugins"
)

func TestDefinitionProvidesGitHubFieldsAndLegacyAlias(t *testing.T) {
	definition := New().Definition()
	if definition.ID != "github-app" || !reflect.DeepEqual(definition.ServiceAliases, []string{"_github-app-serve"}) {
		t.Fatalf("unexpected plugin definition: %+v", definition)
	}
	fields := map[string]plugins.Field{}
	for _, field := range definition.Fields {
		fields[field.Name] = field
	}
	for _, name := range []string{"app_id", "installation_id", "private_key_pass_entry", "repository", "contents", "pull_requests"} {
		if _, exists := fields[name]; !exists {
			t.Errorf("missing field %q", name)
		}
	}
	if fields["contents"].Path[1] != "contents" {
		t.Fatalf("contents permission path = %v", fields["contents"].Path)
	}
	for _, name := range []string{"repository", "contents", "pull_requests"} {
		if !fields[name].Confirm {
			t.Errorf("field %q is missing from the enable confirmation", name)
		}
	}
	for _, name := range []string{"app_id", "installation_id", "private_key_pass_entry"} {
		if fields[name].Confirm {
			t.Errorf("sensitive field %q is included in the enable confirmation", name)
		}
	}
}

func TestValidateConfigAcceptsTextIDsAndOmitsEmptyPermissionChoices(t *testing.T) {
	raw := json.RawMessage(`{"app_id":"12","installation_id":"34","private_key_pass_entry":"key","repository":"octo/repo","permissions":{"contents":"read","pull_requests":""}}`)
	plugin := New()
	if err := plugin.ValidateConfig(raw); err != nil {
		t.Fatalf("text form config rejected: %v", err)
	}
	cfg, err := decodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg.Permissions["pull_requests"]; exists {
		t.Fatal("empty optional permission was retained")
	}
}

func TestHandleContainerUsesInjectedExecutorAndDoesNotMarkPending(t *testing.T) {
	executor := &recordingExecutor{}
	result, err := New().HandleContainer(context.Background(), plugins.ContainerContext{Executor: executor}, plugins.EventConnect)
	if err != nil {
		t.Fatal(err)
	}
	if result.NeedsBuild || !reflect.DeepEqual(executor.args, AdapterInstallCommand()) {
		t.Fatalf("unexpected container result/command: %+v, %v", result, executor.args)
	}
}

func TestStatusDoesNotCheckSystemdWhenPluginIsDisabled(t *testing.T) {
	status, err := New().Status(context.Background(), plugins.HostContext{ProjectPath: t.TempDir()})
	if err != nil || status.Enabled || status.ID != pluginID {
		t.Fatalf("unexpected disabled status: %+v, %v", status, err)
	}
}

func TestStatusReportsInactiveCredentialService(t *testing.T) {
	setupHostHookTest(t, false)
	config, _ := json.Marshal(testConfig())
	status, err := New().Status(context.Background(), plugins.HostContext{ProjectPath: t.TempDir(), Config: config})
	if err != nil || !status.Enabled || status.ID != pluginID || status.Message == "" {
		t.Fatalf("inactive service status was not reported: %+v, %v", status, err)
	}
}

func TestDisableIsNoOpWhenNoConfigOrCleanupStateRemains(t *testing.T) {
	setupHostHookTest(t, false)
	if _, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: t.TempDir()}, plugins.EventDisable); err != nil {
		t.Fatalf("empty disabled plugin cleanup failed: %v", err)
	}
}

func TestValidateContainerConfigRejectsComposeAndMissingAdapterMount(t *testing.T) {
	runtime := t.TempDir()
	if err := os.Chmod(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	project := filepath.Join(t.TempDir(), "project")
	runtimeDir, err := RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := AdapterFragment(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(fragment, &merged); err != nil {
		t.Fatal(err)
	}
	if err := New().ValidateContainerConfig(context.Background(), plugins.ContainerContext{ProjectPath: project}, merged); err != nil {
		t.Fatalf("valid adapter config rejected: %v", err)
	}
	delete(merged, "mounts")
	if err := New().ValidateContainerConfig(context.Background(), plugins.ContainerContext{ProjectPath: project}, merged); err == nil {
		t.Fatal("missing adapter mount was accepted")
	}
	merged["dockerComposeFile"] = json.RawMessage(`"compose.yaml"`)
	if err := New().ValidateContainerConfig(context.Background(), plugins.ContainerContext{ProjectPath: project}, merged); err == nil {
		t.Fatal("Compose configuration was accepted")
	}
}

func TestEnablePersistsOwnershipAfterCredentialReadiness(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	store := &testScopedStore{}
	config, _ := json.Marshal(testConfig())
	result, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, Store: store}, plugins.EventEnable)
	if err != nil {
		t.Fatal(err)
	}
	if !result.NeedsBuild || len(store.config) == 0 || len(store.state) == 0 {
		t.Fatalf("enable did not persist scoped config/state: %+v", store)
	}
	state, err := decodeState(store.state)
	if err != nil || state.FragmentDigest == "" || state.UnitDigest == "" || state.UnitPath == "" {
		t.Fatalf("ownership data was not saved: %+v, %v", state, err)
	}
	unit, err := os.ReadFile(state.UnitPath)
	if err != nil || !strings.Contains(string(unit), "_plugin-serve --plugin github-app --project") {
		t.Fatalf("unit lacks generic serve entrypoint: %s, %v", unit, err)
	}
}

func TestEnableRestartsActiveServiceWhenConfigChanges(t *testing.T) {
	env := setupHostHookTest(t, true)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := AdapterFragment(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	fragmentPath := managedFragment(project)
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, fragment, 0600); err != nil {
		t.Fatal(err)
	}
	unit, err := prepareServiceUnit(project, ownershipState{})
	if err != nil {
		t.Fatal(err)
	}
	oldState, _ := json.Marshal(ownershipState{FragmentDigest: digest(fragment), UnitDigest: unit.digest, UnitPath: unit.path})
	oldConfig, _ := json.Marshal(testConfig())
	updated := testConfig()
	updated.AppID = 9
	updatedConfig, _ := json.Marshal(updated)
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	store := &testScopedStore{config: oldConfig, state: oldState}
	result, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: updatedConfig, State: oldState, PreviousConfig: oldConfig, PreviousState: oldState, Store: store}, plugins.EventEnable)
	if err != nil {
		t.Fatal(err)
	}
	log := readTestFile(t, os.Getenv("DCH_TEST_SYSTEMCTL_LOG"))
	if !strings.Contains(log, "restart") || strings.Contains(log, "enable") || !result.NeedsBuild {
		t.Fatalf("active reconfiguration did not restart service: result=%+v log=%s", result, log)
	}
}

func TestEnableReadinessFailureRestoresPriorScopedValuesAndFragment(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	fragmentPath := managedFragment(project)
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		t.Fatal(err)
	}
	previousFragment := []byte(`{"previous":true}`)
	if err := os.WriteFile(fragmentPath, previousFragment, 0600); err != nil {
		t.Fatal(err)
	}
	listener := readyCredentialSocketStatus(t, project, http.StatusServiceUnavailable)
	defer listener.Close()
	previousConfig := json.RawMessage(`{"previous":true}`)
	previousState, _ := json.Marshal(ownershipState{FragmentDigest: digest(previousFragment)})
	store := &testScopedStore{config: previousConfig, state: previousState}
	config, _ := json.Marshal(testConfig())
	_, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, State: previousState, PreviousConfig: previousConfig, PreviousState: previousState, Store: store}, plugins.EventEnable)
	if err == nil {
		t.Fatal("credential check failure was accepted")
	}
	if !bytes.Equal(store.config, previousConfig) || !bytes.Equal(store.state, previousState) {
		t.Fatalf("previous scoped values were not restored: %+v", store)
	}
	got, err := os.ReadFile(fragmentPath)
	if err != nil || !reflect.DeepEqual(got, previousFragment) {
		t.Fatalf("previous fragment was not restored: %s, %v", got, err)
	}
}

func TestEnsureRestartsWithUpdatedOwnedSystemdUnit(t *testing.T) {
	env := setupHostHookTest(t, true)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := AdapterFragment(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	fragmentPath := managedFragment(project)
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, fragment, 0600); err != nil {
		t.Fatal(err)
	}
	unit, err := prepareServiceUnit(project, ownershipState{})
	if err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", oldPath+string(os.PathListSeparator)+"updated-path")
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	config, _ := json.Marshal(testConfig())
	state, _ := json.Marshal(ownershipState{FragmentDigest: digest(fragment), UnitDigest: unit.digest, UnitPath: unit.path})
	store := &testScopedStore{state: state}
	result, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, State: state, Store: store}, plugins.EventStart)
	if err != nil {
		t.Fatal(err)
	}
	if result.State == nil || !strings.Contains(readTestFile(t, os.Getenv("DCH_TEST_SYSTEMCTL_LOG")), "restart") {
		t.Fatalf("unit change did not restart and save state: %+v", result)
	}
	updated, err := os.ReadFile(unit.path)
	if err != nil || !strings.Contains(string(updated), "updated-path") {
		t.Fatalf("unit environment was not refreshed: %s, %v", updated, err)
	}
}

func TestEnsureRestartFailureRestoresOwnedUnitAndPrivateState(t *testing.T) {
	env := setupHostHookTest(t, true)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	runtimeDir, err := RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := AdapterFragment(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	fragmentPath := managedFragment(project)
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, fragment, 0600); err != nil {
		t.Fatal(err)
	}
	unit, err := prepareServiceUnit(project, ownershipState{})
	if err != nil {
		t.Fatal(err)
	}
	oldUnit, err := os.ReadFile(unit.path)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", oldPath+string(os.PathListSeparator)+"updated-path")
	failMarker := filepath.Join(env.root, "restart-failed")
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte("#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\ncase \"$1\" in\n  is-active) exit 0 ;;\n  restart) if [ ! -f \""+failMarker+"\" ]; then touch \""+failMarker+"\"; exit 1; fi; exit 0 ;;\n  *) exit 0 ;;\nesac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	listener := readyCredentialSocket(t, project)
	defer listener.Close()
	state, _ := json.Marshal(ownershipState{FragmentDigest: digest(fragment), UnitDigest: unit.digest, UnitPath: unit.path})
	store := &testScopedStore{state: state}
	config, _ := json.Marshal(testConfig())
	_, err = New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, State: state, Store: store}, plugins.EventStart)
	if err == nil {
		t.Fatal("failed restart was accepted")
	}
	gotUnit, readErr := os.ReadFile(unit.path)
	if readErr != nil || !bytes.Equal(gotUnit, oldUnit) || !bytes.Equal(store.state, state) {
		t.Fatalf("old unit/state were not restored: state=%s unit=%s err=%v", store.state, gotUnit, readErr)
	}
}

func TestEnableCleanupFailureKeepsSettingsAndOwnershipForRetry(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.root, "bin", "systemctl"), []byte("#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\ncase \"$1\" in\n  is-active) exit 1 ;;\n  stop) exit 1 ;;\n  *) exit 0 ;;\nesac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	listener := readyCredentialSocketStatus(t, project, http.StatusServiceUnavailable)
	defer listener.Close()
	store := &testScopedStore{}
	config, _ := json.Marshal(testConfig())
	_, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, Store: store}, plugins.EventEnable)
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("cleanup failure was not reported: %v", err)
	}
	state, stateErr := decodeState(store.state)
	if stateErr != nil || !bytes.Equal(store.config, config) || state.FragmentDigest == "" || state.UnitDigest == "" {
		t.Fatalf("retryable scoped settings were lost: %+v, %v", store, stateErr)
	}
}

func TestEnableRejectsRemoteDockerBeforeWritingProjectResources(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "tcp://remote.example:2375")
	store := &testScopedStore{}
	config, _ := json.Marshal(testConfig())
	if _, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, Store: store}, plugins.EventEnable); err == nil {
		t.Fatal("remote Docker host was accepted")
	}
	if len(store.config) != 0 {
		t.Fatal("config was persisted despite Docker validation failure")
	}
	if _, err := os.Stat(managedFragment(project)); !os.IsNotExist(err) {
		t.Fatalf("fragment was written before Docker validation: %v", err)
	}
}

func TestDisableRefusesEditedUnitBeforeSystemctl(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	unit, err := unitPath(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, []byte("user modified"), 0600); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(testConfig())
	state, _ := json.Marshal(ownershipState{UnitPath: unit, UnitDigest: digest([]byte("dch generated"))})
	if _, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, State: state}, plugins.EventDisable); err == nil {
		t.Fatal("edited unit was removed")
	}
	if _, err := os.Stat(os.Getenv("DCH_TEST_SYSTEMCTL_LOG")); !os.IsNotExist(err) {
		t.Fatalf("systemctl ran before unit ownership check: %v", err)
	}
}

func TestDisableRefusesEditedFragmentBeforeStoppingService(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	unit, err := prepareServiceUnit(project, ownershipState{})
	if err != nil {
		t.Fatal(err)
	}
	fragmentPath := managedFragment(project)
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, []byte("manual edit"), 0600); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(testConfig())
	state, _ := json.Marshal(ownershipState{FragmentDigest: digest([]byte("generated")), UnitDigest: unit.digest, UnitPath: unit.path})
	if _, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, State: state}, plugins.EventDisable); err == nil {
		t.Fatal("edited fragment was removed")
	}
	if _, err := os.Stat(os.Getenv("DCH_TEST_SYSTEMCTL_LOG")); !os.IsNotExist(err) {
		t.Fatalf("service stopped before fragment ownership check: %v", err)
	}
}

func TestEnablePreservesForeignSystemdUnit(t *testing.T) {
	env := setupHostHookTest(t, false)
	project := filepath.Join(env.root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	unit, err := unitPath(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("[Service]\nExecStart=another-program\n")
	if err := os.WriteFile(unit, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(testConfig())
	store := &testScopedStore{}
	if _, err := New().HandleHost(context.Background(), plugins.HostContext{ProjectPath: project, Config: config, Store: store}, plugins.EventEnable); err == nil {
		t.Fatal("foreign unit was replaced")
	}
	got, err := os.ReadFile(unit)
	if err != nil || !bytes.Equal(got, foreign) || len(store.config) != 0 {
		t.Fatalf("foreign unit or config changed: unit=%s config=%s err=%v", got, store.config, err)
	}
}

type recordingExecutor struct{ args []string }

func (executor *recordingExecutor) Exec(_ context.Context, args ...string) ([]byte, error) {
	executor.args = args
	return nil, nil
}

type testScopedStore struct {
	config json.RawMessage
	state  json.RawMessage
}

func (store *testScopedStore) SaveConfig(raw json.RawMessage) error {
	store.config = append(json.RawMessage(nil), raw...)
	return nil
}

func (store *testScopedStore) SaveState(raw json.RawMessage) error {
	store.state = append(json.RawMessage(nil), raw...)
	return nil
}

type hostHookTestEnv struct{ root string }

func setupHostHookTest(t *testing.T, active bool) hostHookTestEnv {
	t.Helper()
	root := t.TempDir()
	runtimeDir, err := os.MkdirTemp("/tmp", "dchr")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	configHome := filepath.Join(root, "config")
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{configHome, binDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	logPath := filepath.Join(root, "systemctl.log")
	activeResult := "exit 1"
	if active {
		activeResult = "exit 0"
	}
	script := "#!/bin/sh\nshift\necho \"$1\" >> \"$DCH_TEST_SYSTEMCTL_LOG\"\ncase \"$1\" in\n  is-active) " + activeResult + " ;;\n  *) exit 0 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DCH_CONFIG", filepath.Join(root, "dch-config"))
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("DOCKER_HOST", "unix:///tmp/docker.sock")
	t.Setenv("DCH_TEST_SYSTEMCTL_LOG", logPath)
	return hostHookTestEnv{root: root}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readyCredentialSocket(t *testing.T, project string) net.Listener {
	return readyCredentialSocketStatus(t, project, http.StatusOK)
}

func readyCredentialSocketStatus(t *testing.T, project string, status int) net.Listener {
	t.Helper()
	runtimePath, err := RuntimeDir(project)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(runtimePath, "credential.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("token"))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener
}

func testConfig() Config {
	return Config{AppID: 1, InstallationID: 2, PrivateKeyPassEntry: "github/key", Repository: "owner/repo", Permissions: map[string]string{"contents": "read"}}
}
