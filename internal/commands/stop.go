package commands

import (
	"fmt"
	"os/exec"
	"strings"
)

// stopProject stops matching running containers, then offers to delete them.
func stopProject(p Project) error {
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
	screenHeading("Stop  /  " + p.Name)
	stopped := 0
	for _, c := range matched {
		if c.State.Running {
			if err := runDockerAction("stop", c.ID); err != nil {
				screenWait()
				return err
			}
			stopped++
		}
	}
	if stopped == 0 {
		screenNotice("Already stopped", "No running containers were found for this project.")
		return nil
	}
	question := fmt.Sprintf("Stopped %d container(s). Delete all %d matching container(s) for %s? Changes inside their writable layers will be lost.", stopped, len(matched), p.Name)
	deleteContainers, err := confirm(question, false)
	if err != nil {
		return err
	}
	if !deleteContainers {
		screenNotice("Containers stopped", "The containers were kept and can be started again.")
		return nil
	}
	screenHeading("Delete containers  /  " + p.Name)
	for _, c := range matched {
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
