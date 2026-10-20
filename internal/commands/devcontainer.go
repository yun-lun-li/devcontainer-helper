package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func requireExecutable(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found in PATH; install it first", name)
	}
	return nil
}

func invokeDevcontainer(args ...string) error {
	fmt.Printf("$ devcontainer %s\n\n", strings.Join(args, " "))
	cmd := exec.Command("devcontainer", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("devcontainer command failed: %w", err)
	}
	return nil
}

// choose shows a native arrow-key menu for Linux/macOS terminals (via stty).
// For redirected stdin, or when stty is unavailable, it falls back to numbered choices.
