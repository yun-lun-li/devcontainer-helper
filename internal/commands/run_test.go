package commands

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := Run([]string{"unknown"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("Run returned %v, want unknown-command error", err)
	}
}

func TestRunDispatchesProjectsConsole(t *testing.T) {
	t.Setenv("DCH_CONFIG", filepath.Join(t.TempDir(), "config"))
	called := 0
	for _, args := range [][]string{nil, {"console"}} {
		if err := Run(args, nil, func() error { called++; return nil }); err != nil {
			t.Fatalf("Run(%v) returned %v", args, err)
		}
	}
	if called != 2 {
		t.Fatalf("console invoked %d times, want 2", called)
	}
}
