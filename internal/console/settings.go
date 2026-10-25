package console

import (
	"context"
	"errors"

	"dch/internal/plugins"
)

func pluginSettingsMenu(manager *plugins.Manager) error {
	definitions := manager.Definitions()
	options := make([]string, 0, len(definitions)+1)
	available := make([]plugins.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if len(definition.SharedFields) == 0 {
			continue
		}
		available = append(available, definition)
		options = append(options, definition.Name)
	}
	if len(options) == 0 {
		screenNotice("Settings", "No plugins provide shared settings.")
		return nil
	}
	choice, err := choose("Settings · Plugins", append(options, "Back"))
	if errors.Is(err, errCancelled) || choice == len(options) {
		return nil
	}
	if err != nil {
		return err
	}
	return editPluginSharedSettings(manager, available[choice])
}

func editPluginSharedSettings(manager *plugins.Manager, definition plugins.Definition) error {
	current, err := manager.SharedConfig(context.Background(), definition.ID)
	if err != nil {
		return err
	}
	form := definition
	form.Fields = definition.SharedFields
	candidate, err := promptPluginConfig(form, current)
	if err != nil {
		return err
	}
	if err := manager.SetSharedConfig(context.Background(), definition.ID, candidate); err != nil {
		return err
	}
	screenNotice(definition.Name, "Shared plugin settings saved.")
	return nil
}
