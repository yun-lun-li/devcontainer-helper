package commands

import (
	"bufio"
	"strings"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := Run([]string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("Run returned %v, want unknown-command error", err)
	}
}

func TestRunOpensMainMenuWithoutArguments(t *testing.T) {
	previous := input
	input = bufio.NewReader(strings.NewReader("2\n3\n3\n"))
	t.Cleanup(func() { input = previous })
	if err := Run(nil); err != nil {
		t.Fatalf("Run(nil) returned %v", err)
	}
}
