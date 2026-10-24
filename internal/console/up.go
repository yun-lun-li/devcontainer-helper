package console

import (
	"context"
	"fmt"
	"strings"

	"dch/internal/plugins"
)

func startProject(p Project) error {
	manager := plugins.NewManager()
	status, err := manager.Status(context.Background(), p.Path)
	if err != nil {
		return err
	}
	if status.PendingBuild && hasExistingContainer(p) {
		return fmt.Errorf("%s has project plugin changes; Build the project before starting", p.Name)
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	if status.Enabled {
		if err := prepareProjectPlugin(p); err != nil {
			return err
		}
	}
	if err := invokeProjectDevcontainer(p, "up", "--workspace-folder", p.Path); err != nil {
		return err
	}
	if status.Enabled {
		if err := runAdapterInstall(p); err != nil {
			return err
		}
	}
	if p.State == "NOT CREATED" {
		return manager.MarkBuilt(p.Path)
	}
	return nil
}

func hasExistingContainer(p Project) bool {
	return p.ID != "" || strings.HasPrefix(p.State, "RUNNING") || strings.HasPrefix(p.State, "STOPPED")
}
