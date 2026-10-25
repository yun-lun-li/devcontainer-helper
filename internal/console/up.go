package console

import (
	"context"
	"fmt"
	"strings"

	"dch/internal/plugins"
)

func startProject(manager *plugins.Manager, p Project) error {
	return manager.WithProjectOperation(context.Background(), p.Path, func(ctx context.Context) error {
		return startProjectOperation(ctx, manager, p)
	})
}

func startProjectOperation(ctx context.Context, manager *plugins.Manager, p Project) error {
	status, err := manager.Status(ctx, p.Path)
	if err != nil {
		return err
	}
	if status.PendingBuild && hasExistingContainer(p) {
		return fmt.Errorf("%s has project plugin changes; Build the project before starting", p.Name)
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	if err := manager.Prepare(ctx, p.Path, plugins.EventStart); err != nil {
		return err
	}
	status, err = manager.Status(ctx, p.Path)
	if err != nil {
		return err
	}
	if status.PendingBuild && hasExistingContainer(p) {
		return fmt.Errorf("%s has project plugin changes; Build the project before starting", p.Name)
	}
	if err := invokeProjectDevcontainer(ctx, manager, p, "up", "--workspace-folder", p.Path); err != nil {
		return err
	}
	if err := manager.HandleContainer(ctx, p.Path, plugins.EventStart, projectExecutor{manager: manager, project: p}); err != nil {
		return err
	}
	if p.State == "NOT CREATED" {
		return manager.MarkBuilt(ctx, p.Path)
	}
	return nil
}

func hasExistingContainer(p Project) bool {
	return p.ID != "" || strings.HasPrefix(p.State, "RUNNING") || strings.HasPrefix(p.State, "STOPPED")
}
