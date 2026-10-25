package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"dch/internal/plugins"
)

func pluginsMenu(project Project, manager *plugins.Manager) error {
	for {
		definitions := manager.Definitions()
		statuses, err := manager.Statuses(context.Background(), project.Path)
		if err != nil {
			return err
		}
		options := pluginOptions(definitions, statuses)
		choice, err := choose("Project plugins", append(options, "Back"))
		if errors.Is(err, errCancelled) || choice == len(options) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := configurePlugin(project, manager, definitions[choice], statusFor(statuses, definitions[choice].ID)); err != nil {
			if menuErr := reportPluginMenuError(err); menuErr != nil {
				return menuErr
			}
		}
	}
}

func pluginOptions(definitions []plugins.Definition, statuses []plugins.PluginStatus) []string {
	options := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		status := statusFor(statuses, definition.ID)
		state := "disabled"
		if status.Enabled {
			state = "enabled"
		}
		if status.PendingBuild {
			state += " · Build required"
		}
		if status.Message != "" {
			state += " · " + status.Message
		}
		options = append(options, definition.Name+": "+state)
	}
	return options
}

func statusFor(statuses []plugins.PluginStatus, id string) plugins.PluginStatus {
	for _, status := range statuses {
		if status.ID == id {
			return status
		}
	}
	return plugins.PluginStatus{ID: id}
}

func configurePlugin(project Project, manager *plugins.Manager, definition plugins.Definition, status plugins.PluginStatus) error {
	choice, err := choosePluginAction(definition, status)
	if err != nil {
		return err
	}
	if choice < 0 {
		return errCancelled
	}
	if status.Enabled && choice == 0 {
		return showPluginStatus(definition, status)
	}
	if status.Enabled && choice == 2 {
		return disableProjectPlugin(project, manager, definition)
	}
	return enableProjectPlugin(project, manager, definition)
}

func choosePluginAction(definition plugins.Definition, status plugins.PluginStatus) (int, error) {
	actions := []string{"Configure and enable"}
	if status.Enabled {
		actions = []string{"Show status", "Configure and enable", "Disable"}
	}
	actions = append(actions, "Back")
	choice, err := choose(definition.Name, actions)
	if err != nil {
		if errors.Is(err, errCancelled) {
			return -1, nil
		}
		return -1, err
	}
	if choice == len(actions)-1 {
		return -1, nil
	}
	return choice, nil
}

func showPluginStatus(definition plugins.Definition, status plugins.PluginStatus) error {
	message := definition.Name + " is enabled"
	if status.Message != "" {
		message += "\n" + status.Message
	}
	screenNotice(definition.Name, message)
	return nil
}

func disableProjectPlugin(project Project, manager *plugins.Manager, definition plugins.Definition) error {
	if err := manager.Disable(context.Background(), project.Path, definition.ID); err != nil {
		return err
	}
	statuses, err := manager.Statuses(context.Background(), project.Path)
	if err != nil {
		return err
	}
	screenNotice(definition.Name, pluginSuccessMessage("Disabled.", statusFor(statuses, definition.ID).PendingBuild))
	return nil
}

func enableProjectPlugin(project Project, manager *plugins.Manager, definition plugins.Definition) error {
	current, err := manager.Config(context.Background(), project.Path, definition.ID)
	if err != nil {
		return err
	}
	raw, err := promptPluginConfig(definition, current)
	if err != nil {
		return err
	}
	if err := confirmPluginEnable(definition, raw); err != nil {
		return err
	}
	if err := manager.Enable(context.Background(), project.Path, definition.ID, raw); err != nil {
		return err
	}
	statuses, err := manager.Statuses(context.Background(), project.Path)
	if err != nil {
		return err
	}
	screenNotice(definition.Name, pluginSuccessMessage("Enabled and verified.", statusFor(statuses, definition.ID).PendingBuild))
	return nil
}

func confirmPluginEnable(definition plugins.Definition, raw json.RawMessage) error {
	summary, err := pluginConfirmationSummary(definition, raw)
	if err != nil {
		return err
	}
	title := "Enable " + definition.Name + "?"
	if summary != "" {
		title += "\n\n" + summary
	}
	choice, err := choose(title, []string{"Enable and validate", "Cancel"})
	if err != nil {
		return err
	}
	if choice != 0 {
		return errCancelled
	}
	return nil
}

func pluginConfirmationSummary(definition plugins.Definition, raw json.RawMessage) (string, error) {
	config := map[string]any{}
	if len(raw) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&config); err != nil {
			return "", fmt.Errorf("decode %s configuration: %w", definition.Name, err)
		}
	}
	lines := []string{}
	for _, field := range definition.Fields {
		if !field.Confirm {
			continue
		}
		value := getPluginField(config, field)
		if field.Kind == plugins.FieldChoice {
			value = pluginChoiceLabel(field.Options, value)
		}
		if value != "" {
			lines = append(lines, field.Label+": "+value)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	return "Configuration to authorize:\n" + strings.Join(lines, "\n"), nil
}

func pluginChoiceLabel(options []plugins.Choice, value string) string {
	for _, option := range options {
		if option.Value == value {
			return option.Label
		}
	}
	return value
}

func reportPluginMenuError(err error) error {
	if errors.Is(err, errCancelled) {
		return nil
	}
	if screenActive {
		screenError(err)
		return nil
	}
	return err
}

func promptPluginConfig(definition plugins.Definition, current json.RawMessage) (json.RawMessage, error) {
	config := map[string]any{}
	if len(current) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(current))
		decoder.UseNumber()
		if err := decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("decode %s configuration: %w", definition.Name, err)
		}
		if config == nil {
			config = map[string]any{}
		}
	}
	for _, notice := range definition.Notices {
		fmt.Println(notice)
	}
	for _, field := range definition.Fields {
		value, err := promptPluginField(field, config)
		if err != nil {
			return nil, err
		}
		setPluginField(config, field, value)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode %s configuration: %w", definition.Name, err)
	}
	return encoded, nil
}

func promptPluginField(field plugins.Field, config map[string]any) (any, error) {
	current := getPluginField(config, field)
	if field.Kind == plugins.FieldChoice {
		labels := make([]string, len(field.Options))
		for i, option := range field.Options {
			labels[i] = option.Label
			if option.Value == current {
				labels[i] += " (current)"
			}
		}
		choice, err := choose(field.Label, labels)
		if err != nil {
			return nil, err
		}
		selected := field.Options[choice].Value
		if selected == "" {
			if field.Required {
				return nil, fmt.Errorf("%s is required", field.Label)
			}
			return nil, nil
		}
		return selected, nil
	}
	if field.Kind != plugins.FieldText {
		return nil, fmt.Errorf("plugin field %q has unsupported kind %q", field.Name, field.Kind)
	}
	value, err := promptPluginText(field, current)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func getPluginField(config map[string]any, field plugins.Field) string {
	path := field.Path
	if len(path) == 0 {
		path = []string{field.Name}
	}
	var current any = config
	for _, key := range path {
		nested, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = nested[key]
	}
	switch value := current.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return ""
	}
}

func promptPluginText(field plugins.Field, current string) (any, error) {
	label := field.Label
	if current != "" {
		label += " [" + current + "] (Enter to keep"
	} else if !field.Required {
		label += " (optional"
	}
	label += ", q to cancel): "
	fmt.Print(label)
	value, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	value = strings.TrimSpace(value)
	if value == "q" || value == "Q" || (errors.Is(err, io.EOF) && value == "") {
		return nil, errCancelled
	}
	if value == "" {
		if current != "" {
			return current, nil
		}
		if field.Required {
			return nil, fmt.Errorf("%s is required", field.Label)
		}
		return nil, nil
	}
	return value, nil
}

func pluginSuccessMessage(message string, pendingBuild bool) string {
	if pendingBuild {
		return message + " Build the project to apply its container settings."
	}
	return message
}

func setPluginField(config map[string]any, field plugins.Field, value any) {
	path := field.Path
	if len(path) == 0 {
		path = []string{field.Name}
	}
	current := config
	for _, key := range path[:len(path)-1] {
		nested, ok := current[key].(map[string]any)
		if !ok {
			nested = map[string]any{}
			current[key] = nested
		}
		current = nested
	}
	if value == nil {
		delete(current, path[len(path)-1])
		return
	}
	current[path[len(path)-1]] = value
}
