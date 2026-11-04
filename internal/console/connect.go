package console

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dch/internal/common"
)

func connectProject(p Project) error {
	if !strings.HasPrefix(p.State, "RUNNING") {
		return fmt.Errorf("%s is not RUNNING; use Start in dch console first", p.Name)
	}
	choice, err := choose("Connect as", []string{"Current user", "root", "Cancel"})
	if err != nil {
		return err
	}
	args := []string{"exec", "--workspace-folder", p.Path, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"}
	switch choice {
	case 0:
		if err := requireExecutable("devcontainer"); err != nil {
			return err
		}
		return invokeDevcontainer(args...)
	case 1:
		if err := requireExecutable("docker"); err != nil {
			return err
		}
		workdir, err := projectWorkspaceFolder(p)
		if err != nil {
			return err
		}
		return invokeDockerExec(p.ID, "root", workdir)
	default:
		return errCancelled
	}
}

func projectWorkspaceFolder(p Project) (string, error) {
	config, err := common.MergedDevcontainerConfig(p.Path)
	if err != nil {
		return "", err
	}
	var settings struct {
		WorkspaceFolder string `json:"workspaceFolder"`
	}
	if err := json.Unmarshal(config, &settings); err != nil {
		return "", fmt.Errorf("decode merged devcontainer config: %w", err)
	}
	workspaceFolder := settings.WorkspaceFolder
	if workspaceFolder == "" {
		workspaceFolder = filepath.Join("/workspaces", filepath.Base(p.Path))
	}
	workspaceFolder = strings.ReplaceAll(workspaceFolder, "${localWorkspaceFolderBasename}", filepath.Base(p.Path))
	return workspaceFolder, nil
}

func invokeDockerExec(containerID, user, workdir string) error {
	args := []string{"exec", "-u", user, "-w", workdir, "-it", containerID, "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"}
	screenHeading("docker exec")
	fmt.Printf("$ docker %s\n\n", strings.Join(args, " "))
	cmd := exec.Command("docker", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	screenWait()
	if err != nil {
		return fmt.Errorf("docker exec failed: %w", err)
	}
	return nil
}
