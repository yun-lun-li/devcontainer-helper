package console

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dch/internal/common"
	"dch/internal/plugins"
	"dch/internal/plugins/githubapp"
)

func TestPromptPluginConfigUsesGenericFieldsAndNestedPaths(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("Alice\n2\n"))
	defer func() { input = previous }()
	definition := plugins.Definition{ID: "sample", Name: "Sample", Fields: []plugins.Field{
		{Name: "name", Label: "Display name", Required: true, Kind: plugins.FieldText},
		{Name: "access", Label: "Access", Kind: plugins.FieldChoice, Options: []plugins.Choice{{Value: "read", Label: "Read"}, {Value: "write", Label: "Write"}}, Path: []string{"permissions", "contents"}},
	}}

	got, err := promptPluginConfig(definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(got, &config); err != nil {
		t.Fatal(err)
	}
	var name string
	var permissions map[string]string
	if err := json.Unmarshal(config["name"], &name); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(config["permissions"], &permissions); err != nil {
		t.Fatal(err)
	}
	if name != "Alice" || permissions["contents"] != "write" {
		t.Fatalf("config = %s, want generic fields and nested permissions", got)
	}
}

func TestGitHubAppSharedSettingsPromptUsesMetadataAndValidation(t *testing.T) {
	tests := []struct {
		name    string
		current json.RawMessage
		input   string
	}{
		{name: "client ID", input: "Iv1example\n"},
		{name: "keeps current client ID", current: json.RawMessage(`{"client_id":"Iv1example"}`), input: "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := input
			input = bufio.NewReader(strings.NewReader(tt.input))
			defer func() { input = previous }()

			plugin := githubapp.New()
			definition := plugin.Definition()
			definition.Fields = definition.SharedFields
			got, err := promptPluginConfig(definition, tt.current)
			if err != nil {
				t.Fatalf("promptPluginConfig() error = %v", err)
			}
			var values map[string]string
			if err := json.Unmarshal(got, &values); err != nil {
				t.Fatal(err)
			}
			if err := plugin.ValidateSharedSettings(got); err != nil {
				t.Fatalf("GitHub App rejected shared settings %s: %v", got, err)
			}
			if values["client_id"] != "Iv1example" {
				t.Fatalf("shared settings = %s, want Client ID preserved", got)
			}
		})
	}
}

func TestPromptPluginConfigAllowsCancel(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("q\n"))
	defer func() { input = previous }()
	definition := plugins.Definition{Name: "Sample", Fields: []plugins.Field{{Name: "token", Label: "Token", Required: true, Kind: plugins.FieldText}}}
	if _, err := promptPluginConfig(definition, nil); err != errCancelled {
		t.Fatalf("cancel error = %v, want errCancelled", err)
	}
}

func TestPromptPluginConfigKeepsCurrentAndUnmodeledValues(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("\n2\n"))
	defer func() { input = previous }()
	definition := plugins.Definition{Name: "Sample", Fields: []plugins.Field{
		{Name: "display", Label: "Display name", Required: true, Kind: plugins.FieldText},
		{Name: "access", Label: "Access", Kind: plugins.FieldChoice, Options: []plugins.Choice{{Value: "read", Label: "Read"}, {Value: "write", Label: "Write"}}, Path: []string{"permissions", "contents"}},
	}}
	current := json.RawMessage(`{"display":"existing","unknown":9007199254740993,"permissions":{"contents":"read"}}`)
	got, err := promptPluginConfig(definition, current)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(got, &config); err != nil {
		t.Fatal(err)
	}
	var display string
	var permissions map[string]string
	if err := json.Unmarshal(config["display"], &display); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(config["permissions"], &permissions); err != nil {
		t.Fatal(err)
	}
	if display != "existing" || string(config["unknown"]) != "9007199254740993" || permissions["contents"] != "write" {
		t.Fatalf("config = %s, want current and unknown values retained with updated field", got)
	}
}

func TestPromptPluginConfigAllowsBlankOptionalTextField(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("\n"))
	defer func() { input = previous }()
	definition := plugins.Definition{Name: "Sample", Fields: []plugins.Field{{Name: "note", Label: "Note", Kind: plugins.FieldText}}}
	got, err := promptPluginConfig(definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{}" {
		t.Fatalf("config = %s, want empty optional field omitted", got)
	}
}

func TestPluginOptionsReflectIndependentDefinitionsAndPendingState(t *testing.T) {
	definitions := []plugins.Definition{{ID: "first", Name: "First"}, {ID: "second", Name: "Second"}}
	statuses := []plugins.PluginStatus{
		{ID: "first", Enabled: true},
		{ID: "second", PendingBuild: true, Message: "waiting"},
	}
	got := pluginOptions(definitions, statuses)
	if len(got) != 2 || got[0] != "First: enabled" || got[1] != "Second: disabled · Build required · waiting" {
		t.Fatalf("plugin options = %v", got)
	}
}

func TestInteractivePluginActionsAreIndependentOfSharedFieldMetadata(t *testing.T) {
	definition := plugins.Definition{ID: "sample", Name: "Sample"}
	got := choosePluginActions(definition, plugins.PluginStatus{Enabled: true}, true)
	want := []string{"Show status", "Configure and enable", "Reauthorize", "Disable"}
	if len(got) != len(want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("actions = %v, want %v", got, want)
		}
	}
	got = choosePluginActions(definition, plugins.PluginStatus{Enabled: true}, false)
	if len(got) != 3 || got[2] != "Disable" {
		t.Fatalf("non-interactive actions = %v", got)
	}
}

func TestCleanupActionDoesNotShiftEnabledInteractiveActions(t *testing.T) {
	definition := plugins.Definition{ID: "sample", Name: "Sample"}
	if got := choosePluginActions(definition, plugins.PluginStatus{}, false); len(got) != 1 || got[0] != "Configure and enable" {
		t.Fatalf("clean disabled actions = %v", got)
	}
	got := choosePluginActions(definition, plugins.PluginStatus{CleanupPending: true}, true)
	want := []string{"Show status", "Configure and enable", "Clean up"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup actions = %v, want %v", got, want)
	}
	got = choosePluginActions(definition, plugins.PluginStatus{Enabled: true}, true)
	want = []string{"Show status", "Configure and enable", "Reauthorize", "Disable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("enabled actions = %v, want %v", got, want)
	}
}

func TestDisabledCleanupPendingPluginCanRunCleanupFromItsMenu(t *testing.T) {
	project := t.TempDir()
	t.Setenv("DCH_CONFIG", filepath.Join(t.TempDir(), "config"))
	if err := common.SaveConfig(common.Config{Version: 1, Projects: []string{project}, ProjectPlugins: map[string]common.ProjectPlugins{project: {States: map[string]common.PluginState{"sample": {Data: json.RawMessage(`{"retry":true}`)}}}}}); err != nil {
		t.Fatal(err)
	}
	called := false
	manager := plugins.NewManager()
	if err := manager.Register(&cleanupTestPlugin{called: &called}); err != nil {
		t.Fatal(err)
	}
	previous := input
	input = bufio.NewReader(strings.NewReader("3\n"))
	defer func() { input = previous }()
	status := plugins.PluginStatus{ID: "sample", CleanupPending: true}
	if err := configurePlugin(Project{Path: project}, manager, manager.Definitions()[0], status); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("cleanup hook was not called")
	}
	remaining, err := manager.Statuses(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if remaining[0].CleanupPending {
		t.Fatal("cleanup remains pending after cleanup hook")
	}
}

func TestPluginStatusMessageDoesNotCallDisabledPluginEnabled(t *testing.T) {
	message := pluginStatusMessage(plugins.Definition{Name: "Sample"}, plugins.PluginStatus{CleanupPending: true})
	if strings.Contains(message, "enabled") || !strings.Contains(strings.ToLower(message), "cleanup") {
		t.Fatalf("status message = %q", message)
	}
}

type cleanupTestPlugin struct{ called *bool }

func (p *cleanupTestPlugin) Definition() plugins.Definition {
	return plugins.Definition{ID: "sample", Name: "Sample"}
}
func (p *cleanupTestPlugin) ValidateConfig(json.RawMessage) error { return nil }
func (p *cleanupTestPlugin) HandleHost(_ context.Context, _ plugins.HostContext, event plugins.Event) (plugins.Result, error) {
	if event == plugins.EventDisable {
		*p.called = true
		return plugins.Result{State: json.RawMessage{}}, nil
	}
	return plugins.Result{}, nil
}
func (p *cleanupTestPlugin) HandleContainer(context.Context, plugins.ContainerContext, plugins.Event) (plugins.Result, error) {
	return plugins.Result{}, nil
}
func (p *cleanupTestPlugin) ValidateContainerConfig(context.Context, plugins.ContainerContext, map[string]json.RawMessage) error {
	return nil
}
func (p *cleanupTestPlugin) Status(_ context.Context, host plugins.HostContext) (plugins.PluginStatus, error) {
	return plugins.PluginStatus{CleanupPending: len(host.State) > 0}, nil
}

func TestPluginEnableConfirmationCanBeCancelled(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("2\n"))
	defer func() { input = previous }()
	if err := confirmPluginEnable(plugins.Definition{Name: "Sample"}, nil); err != errCancelled {
		t.Fatalf("confirmation error = %v, want errCancelled", err)
	}
}

func TestPluginEnableConfirmationShowsOnlyMarkedValues(t *testing.T) {
	previousInput, previousOutput := input, os.Stdout
	input = bufio.NewReader(strings.NewReader("2\n"))
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		input = previousInput
		os.Stdout = previousOutput
	}()

	definition := plugins.Definition{Name: "Sample", Fields: []plugins.Field{
		{Name: "repository", Label: "Repository", Confirm: true},
		{Name: "contents", Label: "Contents permission", Kind: plugins.FieldChoice, Confirm: true,
			Options: []plugins.Choice{{Value: "read", Label: "Read"}}, Path: []string{"permissions", "contents"}},
		{Name: "secret", Label: "Secret", Confirm: false},
	}}
	err = confirmPluginEnable(definition, json.RawMessage(`{"repository":"octo/project","permissions":{"contents":"read"},"secret":"must-not-display"}`))
	writer.Close()
	output, readErr := io.ReadAll(reader)
	reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != errCancelled {
		t.Fatalf("confirmation error = %v, want errCancelled", err)
	}
	for _, want := range []string{"Repository: octo/project", "Contents permission: Read"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("confirmation output %q does not contain %q", output, want)
		}
	}
	if strings.Contains(string(output), "must-not-display") {
		t.Fatalf("confirmation output exposed unmarked value: %q", output)
	}
}
