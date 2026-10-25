package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dch/internal/common"
)

func TestEnableKeepsPluginConfigAndScopedState(t *testing.T) {
	manager, project := newManagerFixture(t)
	plugin := &fakePlugin{definition: Definition{ID: "alpha", Name: "Alpha"}}
	plugin.host = func(ctx context.Context, host HostContext, event Event) (Result, error) {
		if event != EventEnable || string(host.Config) != `{"choice":"one"}` {
			t.Fatalf("enable context = %#v, event %q", host, event)
		}
		if err := host.Store.SaveConfig(json.RawMessage(`{"choice":"normalized"}`)); err != nil {
			return Result{}, err
		}
		if err := host.Store.SaveState(json.RawMessage(`{"phase":"ready"}`)); err != nil {
			return Result{}, err
		}
		return Result{NeedsBuild: true}, nil
	}
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	if err := manager.Enable(context.Background(), project, "alpha", json.RawMessage(`{"choice":"one"}`)); err != nil {
		t.Fatal(err)
	}
	config, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	stored := config.ProjectPlugins[project]
	if !sameJSON(stored.Enabled["alpha"], `{"choice":"normalized"}`) || !sameJSON(stored.States["alpha"].Data, `{"phase":"ready"}`) || !stored.States["alpha"].BuildPending {
		t.Fatalf("stored plugin data = %#v", stored)
	}
}

func TestServeCanRunWhileEnableWaitsForServiceReadiness(t *testing.T) {
	manager, project := newManagerFixture(t)
	served := make(chan struct{}, 1)
	plugin := &fakeServicePlugin{fakePlugin: fakePlugin{definition: Definition{ID: "service", ServiceAliases: []string{"_service-serve"}}}, served: served}
	plugin.host = func(ctx context.Context, host HostContext, event Event) (Result, error) {
		if err := host.Store.SaveConfig(host.Config); err != nil {
			return Result{}, err
		}
		ready := make(chan error, 1)
		go func() { ready <- manager.Serve(ctx, project, "service") }()
		select {
		case err := <-ready:
			return Result{}, err
		case <-time.After(time.Second):
			return Result{}, errors.New("Serve blocked on the enable operation lock")
		}
	}
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.ServiceAlias("_service-serve"); !ok {
		t.Fatal("service alias was not registered")
	}
	if err := manager.Enable(context.Background(), project, "service", json.RawMessage(`{"token":"safe"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-served:
	default:
		t.Fatal("host service did not start")
	}
}

func TestProjectOperationSerializesHooksAndAllowsNestedManagerCalls(t *testing.T) {
	manager, project := newManagerFixture(t)
	if err := manager.Register(&fakePlugin{definition: Definition{ID: "alpha"}}); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	operationDone := make(chan error, 1)
	go func() {
		operationDone <- manager.WithProjectOperation(context.Background(), project, func(ctx context.Context) error {
			if _, err := manager.Status(ctx, project); err != nil {
				return err
			}
			close(entered)
			<-release
			return manager.MarkBuilt(ctx, project)
		})
	}()
	<-entered
	enableStarted := make(chan struct{})
	enableDone := make(chan error, 1)
	go func() {
		close(enableStarted)
		enableDone <- manager.Enable(context.Background(), project, "alpha", json.RawMessage(`{}`))
	}()
	<-enableStarted
	assertActionBlocked(t, enableDone)
	close(release)
	if err := <-operationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-enableDone; err != nil {
		t.Fatal(err)
	}
}

func TestPrepareSortsPluginsAndStopsAtFirstFailure(t *testing.T) {
	manager, project := newManagerFixture(t)
	var calls []string
	alpha := &fakePlugin{definition: Definition{ID: "alpha"}}
	alpha.host = func(context.Context, HostContext, Event) (Result, error) {
		calls = append(calls, "alpha")
		return Result{NeedsBuild: true, State: json.RawMessage(`{"completed":true}`)}, nil
	}
	zulu := &fakePlugin{definition: Definition{ID: "zulu"}}
	zulu.host = recordHost(&calls, "zulu", errors.New("host unavailable"))
	registerPlugins(t, manager, alpha, zulu)
	seedPlugins(t, project, "alpha", "zulu")

	err := manager.Prepare(context.Background(), project, EventStart)
	if err == nil || !contains(err.Error(), `plugin "zulu" event "start"`) {
		t.Fatalf("Prepare error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"alpha", "zulu"}) {
		t.Fatalf("host calls = %v", calls)
	}
	config, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	state := config.ProjectPlugins[project].States["alpha"]
	if !state.BuildPending || !sameJSON(state.Data, `{"completed":true}`) {
		t.Fatalf("first plugin result was not committed before the failure: %#v", state)
	}
}

func TestUnknownStateIDFailsBeforeAnyHostHook(t *testing.T) {
	manager, project := newManagerFixture(t)
	plugin := &fakePlugin{definition: Definition{ID: "alpha"}}
	var calls []string
	plugin.host = recordHost(&calls, "alpha", nil)
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	seedState(t, project, "future-plugin")
	if err := manager.Prepare(context.Background(), project, EventStart); err == nil {
		t.Fatal("expected unknown state ID to fail")
	}
	if len(calls) != 0 {
		t.Fatalf("hooks ran before unknown ID check: %v", calls)
	}
}

func TestContainerHooksKeepPerPluginChangesAndMarkBuildComplete(t *testing.T) {
	manager, project := newManagerFixture(t)
	alpha := &fakePlugin{definition: Definition{ID: "alpha"}}
	alpha.container = func(_ context.Context, container ContainerContext, event Event) (Result, error) {
		if event != EventBuild || container.Executor == nil {
			return Result{}, errors.New("container context missing")
		}
		return Result{State: json.RawMessage(`{"installed":true}`)}, nil
	}
	if err := manager.Register(alpha); err != nil {
		t.Fatal(err)
	}
	seedPlugins(t, project, "alpha")
	if err := manager.HandleContainer(context.Background(), project, EventBuild, fakeExecutor{}); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkBuilt(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingBuild {
		t.Fatal("pending build remained after MarkBuilt")
	}
	config, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(config.ProjectPlugins[project].States["alpha"].Data, `{"installed":true}`) {
		t.Fatalf("container state = %s", config.ProjectPlugins[project].States["alpha"].Data)
	}
}

type fakePlugin struct {
	definition Definition
	host       func(context.Context, HostContext, Event) (Result, error)
	container  func(context.Context, ContainerContext, Event) (Result, error)
}

func (p *fakePlugin) Definition() Definition { return p.definition }

func (p *fakePlugin) ValidateConfig(json.RawMessage) error { return nil }

func (p *fakePlugin) HandleHost(ctx context.Context, host HostContext, event Event) (Result, error) {
	if p.host == nil {
		return Result{}, nil
	}
	return p.host(ctx, host, event)
}

func (p *fakePlugin) HandleContainer(ctx context.Context, container ContainerContext, event Event) (Result, error) {
	if p.container == nil {
		return Result{}, nil
	}
	return p.container(ctx, container, event)
}

func (p *fakePlugin) ValidateContainerConfig(context.Context, ContainerContext, map[string]json.RawMessage) error {
	return nil
}

func (p *fakePlugin) Status(context.Context, HostContext) (PluginStatus, error) {
	return PluginStatus{}, nil
}

type fakeExecutor struct{}

func (fakeExecutor) Exec(context.Context, ...string) ([]byte, error) { return nil, nil }

type fakeServicePlugin struct {
	fakePlugin
	served chan struct{}
}

func (p *fakeServicePlugin) ServeHost(context.Context, HostContext) error {
	p.served <- struct{}{}
	return nil
}

func newManagerFixture(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("DCH_CONFIG", filepath.Join(root, "config"))
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}}); err != nil {
		t.Fatal(err)
	}
	return NewManager(), project
}

func registerPlugins(t *testing.T, manager *Manager, plugins ...Plugin) {
	t.Helper()
	for _, plugin := range plugins {
		if err := manager.Register(plugin); err != nil {
			t.Fatal(err)
		}
	}
}

func seedPlugins(t *testing.T, project string, ids ...string) {
	t.Helper()
	plugins := common.ProjectPlugins{Enabled: map[string]json.RawMessage{}, States: map[string]common.PluginState{}}
	for _, id := range ids {
		plugins.Enabled[id] = json.RawMessage(`{}`)
		plugins.States[id] = common.PluginState{BuildPending: true}
	}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: plugins}}); err != nil {
		t.Fatal(err)
	}
}

func seedState(t *testing.T, project, id string) {
	t.Helper()
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {States: map[string]common.PluginState{id: {BuildPending: true}}}}}); err != nil {
		t.Fatal(err)
	}
}

func recordHost(calls *[]string, name string, failure error) func(context.Context, HostContext, Event) (Result, error) {
	return func(context.Context, HostContext, Event) (Result, error) {
		*calls = append(*calls, name)
		return Result{}, failure
	}
}

func contains(value, fragment string) bool {
	return strings.Contains(value, fragment)
}

func sameJSON(got json.RawMessage, want string) bool {
	var compact bytes.Buffer
	if err := json.Compact(&compact, got); err != nil {
		return false
	}
	return compact.String() == want
}

func assertActionBlocked(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("operation proceeded while the project lock was held: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
}
