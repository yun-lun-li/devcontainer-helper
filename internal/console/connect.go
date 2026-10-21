package console

import (
	"fmt"
	"strings"
)

func connectProject(p Project) error {
	if err := requireExecutable("devcontainer"); err != nil {
		return err
	}
	if !strings.HasPrefix(p.State, "RUNNING") {
		return fmt.Errorf("%s is not RUNNING; use Start in dch console first", p.Name)
	}
	// Use devcontainer exec so remoteUser and remoteEnv are applied.
	// Prefer bash when present; /bin/sh is the portable fallback.
	return invokeDevcontainer("exec", "--workspace-folder", p.Path, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi")
}
