package console

import (
	"context"
	"fmt"
	"strings"

	"dch/internal/githubapp"
	"dch/internal/plugins"
)

func connectProject(p Project) error {
	status, err := plugins.NewManager().Status(context.Background(), p.Path)
	if err != nil {
		return err
	}
	if status.PendingBuild && hasExistingContainer(p) {
		return fmt.Errorf("%s has project plugin changes; Build the project before connecting", p.Name)
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	if !strings.HasPrefix(p.State, "RUNNING") {
		return fmt.Errorf("%s is not RUNNING; use Start in dch console first", p.Name)
	}
	if status.Enabled {
		if err := prepareProjectPlugin(p); err != nil {
			return err
		}
		if err := runAdapterInstall(p); err != nil {
			return err
		}
	}
	// Use devcontainer exec so remoteUser and remoteEnv are applied.
	// Prefer bash when present; /bin/sh is the portable fallback.
	return invokeProjectDevcontainer(p, "exec", "--workspace-folder", p.Path, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi")
}

func runAdapterInstall(p Project) error {
	args := append([]string{"exec", "--workspace-folder", p.Path}, githubapp.AdapterInstallCommand()...)
	return invokeProjectDevcontainerQuiet(p, args...)
}
