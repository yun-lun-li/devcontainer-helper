package commands

import (
	"fmt"
	"strings"
)

func connect() error {
	p, err := selectRunnableProject("connect")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(p.State, "RUNNING") {
		return fmt.Errorf("%s is not RUNNING; use `dch up` first", p.Name)
	}
	// Use devcontainer exec so remoteUser and remoteEnv are applied.
	// Prefer bash when present; /bin/sh is the portable fallback.
	return invokeDevcontainer("exec", "--workspace-folder", p.Path, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi")
}
