package console

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dch/internal/common"
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

func invokeProjectDevcontainer(p Project, args ...string) error {
	config, err := common.MergedDevcontainerConfig(p.Path)
	if err != nil {
		return err
	}
	configDir := filepath.Join(p.Path, ".devcontainer")
	mergedFile, err := os.CreateTemp(configDir, ".dch-merged-*.json")
	if err != nil {
		return fmt.Errorf("create temporary merged devcontainer config: %w", err)
	}
	mergedPath := mergedFile.Name()
	defer os.Remove(mergedPath)
	if _, err := mergedFile.Write(config); err != nil {
		mergedFile.Close()
		return fmt.Errorf("write temporary merged devcontainer config: %w", err)
	}
	if err := mergedFile.Close(); err != nil {
		return fmt.Errorf("close temporary merged devcontainer config: %w", err)
	}

	args = append(args, "--config", mergedPath)
	if args[0] == "up" {
		basePath, err := filepath.Abs(filepath.Join(configDir, "devcontainer.json"))
		if err != nil {
			return fmt.Errorf("resolve devcontainer config path: %w", err)
		}
		workspacePath, err := filepath.Abs(p.Path)
		if err != nil {
			return fmt.Errorf("resolve workspace path: %w", err)
		}
		args = append(args,
			"--id-label", "devcontainer.local_folder="+workspacePath,
			"--id-label", "devcontainer.config_file="+basePath,
		)
	}
	return invokeDevcontainer(args...)
}
