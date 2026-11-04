package console

import (
	"fmt"
	"strings"
)

func connectProject(p Project) error {
	if !strings.HasPrefix(p.State, "RUNNING") {
		return fmt.Errorf("%s is not RUNNING; use Start in dch console first", p.Name)
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	choice, err := choose("Connect as", []string{"Current user", "root", "Cancel"})
	if err != nil {
		return err
	}
	args := []string{"exec", "--workspace-folder", p.Path, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"}
	switch choice {
	case 0:
		return invokeDevcontainer(args...)
	case 1:
		return invokeProjectDevcontainerAsRoot(p, args...)
	default:
		return errCancelled
	}
}
