package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"dch/internal/githubapp"
	"dch/internal/plugins"
)

func pluginsMenu(project Project) error {
	manager := plugins.NewManager()
	for {
		status, err := manager.Status(context.Background(), project.Path)
		if err != nil {
			return err
		}
		label := "GitHub App: disabled"
		if status.Enabled {
			label = "GitHub App: enabled"
		}
		if status.ServiceActive {
			label += " · service active"
		} else if status.Enabled {
			label += " · service inactive"
		}
		if status.PendingBuild {
			label += " · Build required"
		}
		options := []string{label, "Configure and enable GitHub App", "Disable GitHub App", "Back"}
		choice, err := choose("Project plugins", options)
		if errors.Is(err, errCancelled) || choice == 3 {
			return nil
		}
		if err != nil {
			return err
		}
		switch choice {
		case 0:
			message := label
			if status.Message != "" {
				message += "\n" + status.Message
			}
			screenNotice("Project plugins", message)
		case 1:
			screenHeading("Configure GitHub App")
			cfg, err := promptGitHubAppConfig()
			if errors.Is(err, errCancelled) {
				continue
			}
			if err == nil {
				err = manager.Enable(context.Background(), project.Path, cfg)
			}
			if err != nil {
				if screenActive {
					screenError(err)
					continue
				}
				return err
			}
			screenNotice("GitHub App", "Enabled and verified. Build the project to apply its container settings.")
		case 2:
			if !status.Enabled {
				continue
			}
			if err := manager.Disable(context.Background(), project.Path); err != nil {
				if screenActive {
					screenError(err)
					continue
				}
				return err
			}
			screenNotice("GitHub App", "Disabled. Build the project to remove its container settings.")
		}
	}
}

func promptGitHubAppConfig() (githubapp.Config, error) {
	var cfg githubapp.Config
	appID, err := promptPositiveID("GitHub App ID")
	if err != nil {
		return cfg, err
	}
	cfg.AppID = appID
	installationID, err := promptPositiveID("Installation ID")
	if err != nil {
		return cfg, err
	}
	cfg.InstallationID = installationID
	cfg.PrivateKeyPassEntry, err = promptText("pass entry containing the private key")
	if err != nil {
		return cfg, err
	}
	cfg.Repository, err = promptText("GitHub repository (owner/repository)")
	if err != nil {
		return cfg, err
	}
	cfg.Permissions = map[string]string{}
	for _, permission := range []string{"contents", "pull_requests"} {
		choice, err := choose(permission+" permission", []string{"Read", "Write", "Disabled"})
		if err != nil {
			return cfg, err
		}
		if choice < 2 {
			cfg.Permissions[permission] = []string{"read", "write"}[choice]
		}
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	scopes := make([]string, 0, len(cfg.Permissions))
	for name, level := range cfg.Permissions {
		scopes = append(scopes, name+":"+level)
	}
	sort.Strings(scopes)
	choice, err := choose("Authorize repository access: "+cfg.Repository+" ("+strings.Join(scopes, ", ")+")", []string{"Enable and verify", "Cancel"})
	if err != nil {
		return cfg, err
	}
	if choice != 0 {
		return cfg, errCancelled
	}
	return cfg, nil
}

func promptPositiveID(label string) (int64, error) {
	value, err := promptText(label)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive number", label)
	}
	return id, nil
}

func promptText(label string) (string, error) {
	fmt.Printf("%s (q to cancel): ", label)
	value, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "q" || value == "Q" || (errors.Is(err, io.EOF) && value == "") {
		return "", errCancelled
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}
