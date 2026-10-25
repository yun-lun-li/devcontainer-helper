package console

import (
	"context"
	"fmt"
	"strings"

	"dch/internal/plugins"
)

func connectProject(manager *plugins.Manager, p Project) error {
	var waitForShell func() error
	err := manager.WithProjectOperation(context.Background(), p.Path, func(ctx context.Context) error {
		if err := prepareProjectConnect(ctx, manager, p); err != nil {
			return err
		}
		args := []string{"exec", "--workspace-folder", p.Path, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"}
		prepared, cleanup, err := prepareProjectDevcontainer(ctx, manager, p, args...)
		if err != nil {
			return err
		}
		if err := rejectPendingConnect(ctx, manager, p); err != nil {
			cleanup()
			return err
		}
		waitForShell, err = startDevcontainerWithCleanup(cleanup, prepared...)
		return err
	})
	if err != nil {
		return err
	}
	return waitForShell()
}

func rejectPendingConnect(ctx context.Context, manager *plugins.Manager, p Project) error {
	status, err := manager.Status(ctx, p.Path)
	if err != nil {
		return err
	}
	if status.PendingBuild && hasExistingContainer(p) {
		return fmt.Errorf("%s has project plugin changes; Build the project before connecting", p.Name)
	}
	return nil
}

func prepareProjectConnect(ctx context.Context, manager *plugins.Manager, p Project) error {
	if err := rejectPendingConnect(ctx, manager, p); err != nil {
		return err
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	if !strings.HasPrefix(p.State, "RUNNING") {
		return fmt.Errorf("%s is not RUNNING; use Start in dch console first", p.Name)
	}
	if err := manager.Prepare(ctx, p.Path, plugins.EventConnect); err != nil {
		return err
	}
	if err := rejectPendingConnect(ctx, manager, p); err != nil {
		return err
	}
	merged, err := mergedProjectConfig(p)
	if err != nil {
		return err
	}
	if err := validateMergedProjectConfig(ctx, manager, p, merged); err != nil {
		return err
	}
	if err := manager.HandleContainer(ctx, p.Path, plugins.EventConnect, projectExecutor{manager: manager, project: p}); err != nil {
		return err
	}
	return rejectPendingConnect(ctx, manager, p)
}
