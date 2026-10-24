package console

import (
	"context"
	"dch/internal/plugins"
)

func buildProject(p Project) error {
	if hasExistingContainer(p) {
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
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	manager := plugins.NewManager()
	status, err := manager.Status(context.Background(), p.Path)
	if err != nil {
		return err
	}
	if status.Enabled {
		if err := prepareProjectPlugin(p); err != nil {
			return err
		}
	}
	cacheChoice, err := choose("Build cache", []string{
		"Use build cache",
		"Build without cache",
	})
	if err != nil {
		return err
	}
	args := []string{"up", "--workspace-folder", p.Path, "--remove-existing-container"}
	if cacheChoice == 1 {
		args = append(args, "--build-no-cache")
	}
	if err := invokeProjectDevcontainer(p, args...); err != nil {
		return err
	}
	if status.Enabled {
		if err := runAdapterInstall(p); err != nil {
			return err
		}
	}
	return manager.MarkBuilt(p.Path)
}
