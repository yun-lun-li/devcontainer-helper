package console

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dch/internal/common"
	"dch/internal/plugins"
)

type connectLockPlugin struct {
	containerPrepared          bool
	postContainerStatusBlocked bool
	postContainerStatusStarted chan struct{}
	releasePostContainerStatus chan struct{}
}

func (*connectLockPlugin) Definition() plugins.Definition {
	return plugins.Definition{ID: "connect-lock", Name: "Connect lock"}
}
func (*connectLockPlugin) ValidateConfig(json.RawMessage) error { return nil }
func (*connectLockPlugin) HandleHost(context.Context, plugins.HostContext, plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (plugin *connectLockPlugin) HandleContainer(_ context.Context, _ plugins.ContainerContext, event plugins.Event) (plugins.Result, error) {
	if event == plugins.EventConnect {
		plugin.containerPrepared = true
	}
	return plugins.Result{}, nil
}
func (*connectLockPlugin) ValidateContainerConfig(context.Context, plugins.ContainerContext, map[string]json.RawMessage) error {
	return nil
}
func (plugin *connectLockPlugin) Status(context.Context, plugins.HostContext) (plugins.PluginStatus, error) {
	if plugin.containerPrepared && !plugin.postContainerStatusBlocked {
		plugin.postContainerStatusBlocked = true
		close(plugin.postContainerStatusStarted)
		<-plugin.releasePostContainerStatus
	}
	return plugins.PluginStatus{}, nil
}

type connectEnablePlugin struct{ enableHookStarted chan struct{} }

func (*connectEnablePlugin) Definition() plugins.Definition {
	return plugins.Definition{ID: "connect-enable", Name: "Connect enable"}
}
func (*connectEnablePlugin) ValidateConfig(json.RawMessage) error { return nil }
func (plugin *connectEnablePlugin) HandleHost(_ context.Context, _ plugins.HostContext, event plugins.Event) (plugins.Result, error) {
	if event == plugins.EventEnable {
		close(plugin.enableHookStarted)
	}
	return plugins.Result{NeedsBuild: event == plugins.EventEnable}, nil
}
func (*connectEnablePlugin) HandleContainer(context.Context, plugins.ContainerContext, plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (*connectEnablePlugin) ValidateContainerConfig(context.Context, plugins.ContainerContext, map[string]json.RawMessage) error {
	return nil
}
func (*connectEnablePlugin) Status(context.Context, plugins.HostContext) (plugins.PluginStatus, error) {
	return plugins.PluginStatus{}, nil
}

func TestConnectStartsShellBeforeReleasingOperationLock(t *testing.T) {
	fixture := newConnectLockFixture(t)
	connectDone := make(chan error, 1)
	go func() {
		connectDone <- connectProject(fixture.manager, Project{Name: "project", Path: fixture.projectPath, State: "RUNNING", ID: "container-id"})
	}()
	waitForSignal(t, fixture.plugin.postContainerStatusStarted, "connect did not reach its post-container status check", 3*time.Second)
	enableDone := make(chan error, 1)
	go func() {
		enableDone <- fixture.manager.Enable(context.Background(), fixture.projectPath, "connect-enable", json.RawMessage(`{}`))
	}()
	assertSignalNotReceived(t, fixture.enablePlugin.enableHookStarted, "plugin enable entered its commit hook while connect held the operation lock", 100*time.Millisecond)
	close(fixture.plugin.releasePostContainerStatus)
	if err := waitForFile(fixture.started); err != nil {
		t.Fatalf("shell process did not start: %v", err)
	}
	assertOperationDone(t, enableDone, "plugin enable remained blocked while shell was interactive")
	if err := os.WriteFile(fixture.release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	assertOperationDone(t, connectDone, "connect did not finish after shell release")
}

type connectLockFixture struct {
	manager      *plugins.Manager
	plugin       *connectLockPlugin
	enablePlugin *connectEnablePlugin
	projectPath  string
	started      string
	release      string
}

func newConnectLockFixture(t *testing.T) connectLockFixture {
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".devcontainer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".devcontainer", "devcontainer.json"), []byte(`{"image":"example:latest"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	plugin := &connectLockPlugin{postContainerStatusStarted: make(chan struct{}), releasePostContainerStatus: make(chan struct{})}
	manager := plugins.NewManager()
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	enablePlugin := &connectEnablePlugin{enableHookStarted: make(chan struct{})}
	if err := manager.Register(enablePlugin); err != nil {
		t.Fatal(err)
	}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{projectPath}, ProjectPlugins: map[string]common.ProjectPlugins{
		projectPath: {Enabled: map[string]json.RawMessage{"connect-lock": json.RawMessage(`{}`)}},
	}}); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	started, release := filepath.Join(root, "started"), filepath.Join(root, "release")
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0600) })
	script := "#!/bin/sh\ntouch " + shellQuote(started) + "\nwhile [ ! -f " + shellQuote(release) + " ]; do sleep 0.01; done\n"
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return connectLockFixture{manager: manager, plugin: plugin, enablePlugin: enablePlugin, projectPath: projectPath, started: started, release: release}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, message string, timeout time.Duration) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(timeout):
		t.Fatal(message)
	}
}

func assertSignalNotReceived(t *testing.T, signal <-chan struct{}, message string, timeout time.Duration) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(message)
	case <-time.After(timeout):
	}
}

func assertOperationDone(t *testing.T, done <-chan error, timeoutMessage string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal(timeoutMessage)
	}
}

func waitForFile(path string) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return os.ErrNotExist
}
