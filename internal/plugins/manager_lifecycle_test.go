package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"dch/internal/common"
)

func TestUnknownEnabledIDFailsBeforeAnyHostHook(t *testing.T) {
	manager, project := newManagerFixture(t)
	var calls int
	plugin := &fakePlugin{definition: Definition{ID: "alpha"}}
	plugin.host = func(context.Context, HostContext, Event) (Result, error) {
		calls++
		return Result{}, nil
	}
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {Enabled: map[string]json.RawMessage{"future-plugin": json.RawMessage(`{}`)}}}}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveProject(context.Background(), project); err == nil {
		t.Fatal("expected unknown enabled ID to fail")
	}
	if calls != 0 {
		t.Fatalf("hook ran before unknown ID validation: %d calls", calls)
	}
}

func TestEnabledPluginsKeepIndependentConfigAndState(t *testing.T) {
	manager, project := newManagerFixture(t)
	alpha := statePlugin("alpha")
	beta := statePlugin("beta")
	registerPlugins(t, manager, beta, alpha)
	configs := map[string]string{"alpha": `{"account":"a"}`, "beta": `{"account":"b"}`}
	for id, raw := range configs {
		if err := manager.Enable(context.Background(), project, id, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
		stored, err := manager.Config(context.Background(), project, id)
		if err != nil || !sameJSON(stored, raw) {
			t.Fatalf("config for %s = %s, %v", id, stored, err)
		}
	}
	config, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	for id := range configs {
		if !sameJSON(config.ProjectPlugins[project].States[id].Data, `{"plugin":"`+id+`"}`) {
			t.Fatalf("private state for %s = %s", id, config.ProjectPlugins[project].States[id].Data)
		}
	}
}

func TestPendingBuildIsUnionAndRequiresExplicitCompletion(t *testing.T) {
	manager, project := newManagerFixture(t)
	alpha := &fakePlugin{definition: Definition{ID: "alpha"}}
	alpha.host = func(_ context.Context, _ HostContext, event Event) (Result, error) {
		if event == EventEnable {
			return Result{NeedsBuild: true}, nil
		}
		return Result{}, nil
	}
	beta := &fakePlugin{definition: Definition{ID: "beta"}}
	beta.container = func(_ context.Context, _ ContainerContext, event Event) (Result, error) {
		if event == EventBuild {
			return Result{}, errors.New("container install failed")
		}
		return Result{}, nil
	}
	registerPlugins(t, manager, alpha, beta)
	for _, id := range []string{"alpha", "beta"} {
		if err := manager.Enable(context.Background(), project, id, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	status, err := manager.Status(context.Background(), project)
	if err != nil || !status.PendingBuild {
		t.Fatalf("pending union status = %#v, %v", status, err)
	}
	if err := manager.HandleContainer(context.Background(), project, EventBuild, fakeExecutor{}); err == nil {
		t.Fatal("expected second plugin container hook to fail")
	}
	status, err = manager.Status(context.Background(), project)
	if err != nil || !status.PendingBuild {
		t.Fatalf("failed container hook cleared pending state: %#v, %v", status, err)
	}
	if err := manager.MarkBuilt(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	status, err = manager.Status(context.Background(), project)
	if err != nil || status.PendingBuild {
		t.Fatalf("pending state after MarkBuilt = %#v, %v", status, err)
	}
}

func TestDisableFailureKeepsEnabledConfigForRetry(t *testing.T) {
	manager, project := newManagerFixture(t)
	plugin := &fakePlugin{definition: Definition{ID: "retry"}}
	callCount := 0
	plugin.host = func(_ context.Context, host HostContext, event Event) (Result, error) {
		if event != EventDisable {
			return Result{}, nil
		}
		callCount++
		if callCount == 1 {
			return Result{}, errors.New("cleanup incomplete")
		}
		if err := host.Store.SaveState(nil); err != nil {
			return Result{}, err
		}
		return Result{NeedsBuild: true}, nil
	}
	registerPlugins(t, manager, plugin)
	seedPlugins(t, project, "retry")
	if err := manager.Disable(context.Background(), project, "retry"); err == nil {
		t.Fatal("expected cleanup failure")
	}
	config, err := manager.Config(context.Background(), project, "retry")
	if err != nil || !sameJSON(config, `{}`) {
		t.Fatalf("enabled config after cleanup failure = %s, %v", config, err)
	}
	if err := manager.Disable(context.Background(), project, "retry"); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(context.Background(), project)
	if err != nil || status.Plugins[0].Enabled || !status.PendingBuild {
		t.Fatalf("status after retry = %#v, %v", status, err)
	}
}

func TestRemoveProjectKeepsRegistrationUntilEveryCleanupSucceeds(t *testing.T) {
	manager, project := newManagerFixture(t)
	cleanupFails := registerRemovePlugins(t, manager)
	seedPlugins(t, project, "alpha", "beta")
	if err := manager.RemoveProject(context.Background(), project); err == nil {
		t.Fatal("expected cleanup failure")
	}
	assertProjectRetainsRetryState(t, project)
	*cleanupFails = false
	if err := manager.RemoveProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	assertProjectRegistrationRemoved(t)
}

func registerRemovePlugins(t *testing.T, manager *Manager) *bool {
	t.Helper()
	alpha := &fakePlugin{definition: Definition{ID: "alpha"}}
	alpha.host = func(_ context.Context, host HostContext, event Event) (Result, error) {
		if event == EventDisable {
			if err := host.Store.SaveConfig(nil); err != nil {
				return Result{}, err
			}
			return Result{NeedsBuild: true}, nil
		}
		return Result{}, nil
	}
	beta := &fakePlugin{definition: Definition{ID: "beta"}}
	cleanupFails := true
	beta.host = func(_ context.Context, host HostContext, event Event) (Result, error) {
		if event != EventDisable {
			return Result{}, nil
		}
		if cleanupFails {
			return Result{}, errors.New("service stop failed")
		}
		return Result{}, host.Store.SaveConfig(nil)
	}
	registerPlugins(t, manager, beta, alpha)
	return &cleanupFails
}

func assertProjectRetainsRetryState(t *testing.T, project string) {
	t.Helper()
	config, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Projects) != 1 || !sameJSON(config.ProjectPlugins[project].Enabled["beta"], `{}`) {
		t.Fatalf("failed cleanup lost retry state: %#v", config)
	}
	if _, stillEnabled := config.ProjectPlugins[project].Enabled["alpha"]; stillEnabled {
		t.Fatal("earlier successful cleanup was rolled back")
	}
}

func assertProjectRegistrationRemoved(t *testing.T) {
	t.Helper()
	config, err := common.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Projects) != 0 {
		t.Fatalf("project registration remains after cleanup: %v", config.Projects)
	}
}

func TestServiceAliasesRequireHostServiceAndServeEnabledPlugin(t *testing.T) {
	manager, project := newManagerFixture(t)
	invalid := &fakePlugin{definition: Definition{ID: "invalid", ServiceAliases: []string{"_invalid-serve"}}}
	if err := manager.Register(invalid); err == nil {
		t.Fatal("expected aliases without HostService to fail registration")
	}
	service := &fakeServicePlugin{fakePlugin: fakePlugin{definition: Definition{ID: "service", ServiceAliases: []string{"_service-serve"}}}, served: make(chan struct{}, 1)}
	if err := manager.Register(service); err != nil {
		t.Fatal(err)
	}
	id, ok := manager.ServiceAlias("_service-serve")
	if !ok || id != "service" {
		t.Fatalf("service alias = %q, %v", id, ok)
	}
	if err := manager.Serve(context.Background(), project, id); err == nil {
		t.Fatal("expected disabled service plugin to refuse serving")
	}
}

func statePlugin(id string) *fakePlugin {
	plugin := &fakePlugin{definition: Definition{ID: id}}
	plugin.host = func(_ context.Context, host HostContext, event Event) (Result, error) {
		if event == EventEnable {
			state, err := json.Marshal(map[string]string{"plugin": id})
			if err != nil {
				return Result{}, err
			}
			return Result{State: state}, nil
		}
		return Result{}, nil
	}
	return plugin
}
