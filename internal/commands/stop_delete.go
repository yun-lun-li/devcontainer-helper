package commands

import (
	"fmt"
	"os/exec"
	"strings"
)

// stopDeleteProject removes only containers whose Dev Container labels match
// this registered workspace and its configuration file.
func stopDeleteProject(p Project) error {
	containers, err := inspectContainers()
	if err != nil {
		return err
	}
	matched := matchingContainers(p.Path, containers)
	if len(matched) == 0 {
		return fmt.Errorf("no matching container found for %s", p.Name)
	}
	for _, c := range matched {
		if c.ID == "" {
			return fmt.Errorf("matching container has no ID")
		}
	}
	question := fmt.Sprintf("Stop and delete %d container(s) for %s? Changes inside their writable layers will be lost.", len(matched), p.Name)
	ok, err := confirm(question, false)
	if err != nil || !ok {
		return err
	}
	screenHeading("Stop & Delete  /  " + p.Name)
	for _, c := range matched {
		if c.State.Running {
			if err := runDockerAction("stop", c.ID); err != nil {
				screenWait()
				return err
			}
		}
		if err := runDockerAction("rm", c.ID); err != nil {
			screenWait()
			return err
		}
	}
	fmt.Printf("\nRemoved %d container(s). Project registration and files were kept.\n", len(matched))
	screenWait()
	return nil
}

func runDockerAction(action, id string) error {
	fmt.Printf("$ docker %s %s\n", action, id)
	cmd := exec.Command("docker", action, id)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if err != nil {
		return fmt.Errorf("docker %s %s failed: %s: %w", action, id, strings.TrimSpace(string(out)), err)
	}
	return nil
}
