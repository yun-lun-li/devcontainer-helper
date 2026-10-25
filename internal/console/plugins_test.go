package console

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

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

func TestGitHubAppPromptOutputPassesPluginValidation(t *testing.T) {
	tests := []struct {
		name    string
		current json.RawMessage
		input   string
		wantErr string
	}{
		{name: "text IDs", input: "123\n456\n2\n1\n"},
		{name: "numeric current IDs", current: json.RawMessage(`{"app_id":123,"installation_id":456}`), input: "\n\n2\n1\n"},
		{name: "App ID URL", input: "https://github.com/apps/example\n456\n2\n1\n", wantErr: "app_id must be a decimal numeric App ID (not Client ID)"},
		{name: "Installation ID Client ID", input: "123\nIv1.example\n2\n1\n", wantErr: "installation_id must be the numeric installation ID from the installation settings URL, not the App ID or Client ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := input
			input = bufio.NewReader(strings.NewReader(tt.input))
			defer func() { input = previous }()

			definition := githubapp.New().Definition()
			got, err := promptPluginConfig(definition, tt.current)
			if err != nil {
				t.Fatalf("promptPluginConfig() error = %v", err)
			}
			err = githubapp.New().ValidateConfig(got)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("GitHub App validation error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GitHub App rejected prompt output %s: %v", got, err)
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
