package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dch/internal/plugins"
)

const pluginID = "github-app"

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Definition() plugins.Definition {
	return plugins.Definition{
		ID: pluginID, Name: "GitHub App", Description: "Provide GitHub credentials to this project.",
		ServiceAliases: []string{"_github-app-serve"},
		Notices: []string{
			"Store the RSA private key with pass as dch/github-app/<AppID>/private-key.",
			"Without a legacy repository setting, tokens can access every repository in the GitHub App installation, limited by the selected permissions.",
			"Use the numeric installation ID from the GitHub App installation settings URL; do not enter the App ID or Client ID.",
		},
		Fields: []plugins.Field{
			{Name: "app_id", Label: "App ID", Required: true, Kind: plugins.FieldText},
			{Name: "installation_id", Label: "Installation ID", Required: true, Kind: plugins.FieldText},
			{Name: "contents", Label: "Contents permission", Confirm: true, Kind: plugins.FieldChoice, Options: permissionChoices(), Path: []string{"permissions", "contents"}},
			{Name: "pull_requests", Label: "Pull requests permission", Confirm: true, Kind: plugins.FieldChoice, Options: permissionChoices(), Path: []string{"permissions", "pull_requests"}},
		},
	}
}

func permissionChoices() []plugins.Choice {
	return []plugins.Choice{{Value: "", Label: "Not requested"}, {Value: "read", Label: "Read"}, {Value: "write", Label: "Write"}}
}

func (p *Plugin) ValidateConfig(raw json.RawMessage) error {
	cfg, err := decodeConfig(raw)
	if err != nil {
		return err
	}
	return cfg.Validate()
}

func decodeConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		var fieldErr configFieldError
		if errors.As(err, &fieldErr) {
			return Config{}, fieldErr
		}
		return Config{}, fmt.Errorf("invalid GitHub App configuration")
	}
	for name, value := range cfg.Permissions {
		if value == "" {
			delete(cfg.Permissions, name)
		}
	}
	return cfg, nil
}

func (p *Plugin) HandleHost(ctx context.Context, host plugins.HostContext, event plugins.Event) (plugins.Result, error) {
	switch event {
	case plugins.EventEnable:
		return p.enable(ctx, host)
	case plugins.EventDisable:
		return p.disable(ctx, host)
	case plugins.EventStart, plugins.EventBuild, plugins.EventConnect:
		return p.ensure(ctx, host)
	default:
		return plugins.Result{}, nil
	}
}

func (p *Plugin) HandleContainer(ctx context.Context, container plugins.ContainerContext, event plugins.Event) (plugins.Result, error) {
	if event != plugins.EventStart && event != plugins.EventBuild && event != plugins.EventConnect {
		return plugins.Result{}, nil
	}
	if _, err := container.Executor.Exec(ctx, AdapterInstallCommand()...); err != nil {
		return plugins.Result{}, fmt.Errorf("install GitHub adapter: %w", err)
	}
	return plugins.Result{}, nil
}

func (p *Plugin) ValidateContainerConfig(_ context.Context, host plugins.ContainerContext, merged map[string]json.RawMessage) error {
	if _, exists := merged["dockerComposeFile"]; exists {
		return fmt.Errorf("GitHub App plugin does not support Docker Compose")
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("cannot validate GitHub App adapter settings")
	}
	runtimeDir, err := RuntimeDir(host.ProjectPath)
	if err != nil {
		return err
	}
	return validateContainerConfig(raw, runtimeDir)
}

func (p *Plugin) Status(ctx context.Context, host plugins.HostContext) (plugins.PluginStatus, error) {
	status := plugins.PluginStatus{ID: pluginID, Name: "GitHub App", Enabled: len(host.Config) > 0}
	if !status.Enabled {
		return status, nil
	}
	err := runSystemctl(ctx, "is-active", "--quiet", unitName(host.ProjectPath))
	if err == nil {
		status.Message = "Credential service is active"
	} else {
		status.Message = strings.TrimSpace(err.Error())
	}
	return status, nil
}
