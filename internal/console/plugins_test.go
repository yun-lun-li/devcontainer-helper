package console

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"dch/internal/plugins"
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

func TestPluginEnableConfirmationCanBeCancelled(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("2\n"))
	defer func() { input = previous }()
	if err := confirmPluginEnable(plugins.Definition{Name: "Sample"}); err != errCancelled {
		t.Fatalf("confirmation error = %v, want errCancelled", err)
	}
}
