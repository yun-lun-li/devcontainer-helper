package console

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
	screenHeading("devcontainer " + args[0])
	fmt.Printf("$ devcontainer %s\n\n", strings.Join(args, " "))
	cmd := exec.Command("devcontainer", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	screenWait()
	if err != nil {
		return fmt.Errorf("devcontainer command failed: %w", err)
	}
	return nil
}
