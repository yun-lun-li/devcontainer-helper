package console

import (
	"context"
	"dch/internal/plugins"
)

func buildProject(manager *plugins.Manager, p Project) error {
	if err := confirmRebuild(p); err != nil {
		return err
	}
	cacheChoice, err := choose("Build cache", []string{
		"Use build cache",
		"Build without cache",
	})
	if err != nil {
		return err
	}
	return manager.WithProjectOperation(context.Background(), p.Path, func(ctx context.Context) error {
		return buildProjectOperation(ctx, manager, p, cacheChoice)
	})
}

func buildProjectOperation(ctx context.Context, manager *plugins.Manager, p Project, cacheChoice int) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	if err := manager.Prepare(ctx, p.Path, plugins.EventBuild); err != nil {
		return err
	}
	args := []string{"up", "--workspace-folder", p.Path, "--remove-existing-container"}
	if cacheChoice == 1 {
		args = append(args, "--build-no-cache")
	}
	if err := invokeProjectDevcontainer(ctx, manager, p, args...); err != nil {
		return err
	}
	if err := manager.HandleContainer(ctx, p.Path, plugins.EventBuild, projectExecutor{manager: manager, project: p}); err != nil {
		return err
	}
	return manager.MarkBuilt(ctx, p.Path)
}

func confirmRebuild(p Project) error {
	if !hasExistingContainer(p) {
		return nil
	}
	choice, err := choose("Rebuild container? Writable-layer data may be discarded", []string{
		"Remove the existing container and rebuild",
		"Cancel",
	})
	if err != nil {
		return err
	}
	if choice != 0 {
		return errCancelled
	}
	return nil
}
