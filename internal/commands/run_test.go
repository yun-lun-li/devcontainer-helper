package commands

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := Run([]string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("Run returned %v, want unknown-command error", err)
	}
}

func TestRunOpensProjectsConsole(t *testing.T) {
	t.Setenv("DCH_CONFIG", filepath.Join(t.TempDir(), "config"))
	for _, args := range [][]string{nil, {"console"}} {
		if err := Run(args); err != nil {
			t.Fatalf("Run(%v) returned %v", args, err)
		}
	}
}
