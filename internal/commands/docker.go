package commands

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type DockerContainer struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
	} `json:"State"`
	Created string `json:"Created"`
}

func inspectContainers() ([]DockerContainer, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker executable not found")
	}
	cmd := exec.Command("docker", "ps", "-a", "-q", "--filter", "label=devcontainer.local_folder")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %s: %w", strings.TrimSpace(string(out)), err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return []DockerContainer{}, nil
	}
	args := append([]string{"inspect", "--type", "container"}, ids...)
	out, err = exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %s: %w", strings.TrimSpace(string(out)), err)
	}
	var cs []DockerContainer
	if err := json.Unmarshal(out, &cs); err != nil {
		return nil, fmt.Errorf("invalid docker inspect output: %w", err)
	}
	return cs, nil
}

func matchContainer(path string, cs []DockerContainer) (DockerContainer, bool, int) {
	matched := matchingContainers(path, cs)
	if len(matched) == 0 {
		return DockerContainer{}, false, 0
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].State.Running != matched[j].State.Running {
			return matched[i].State.Running
		}
		return matched[i].Created > matched[j].Created
	})
	return matched[0], true, len(matched)
}

func matchingContainers(path string, cs []DockerContainer) []DockerContainer {
	confFile := filepath.Join(path, ".devcontainer", "devcontainer.json")
	var matched []DockerContainer
	for _, c := range cs {
		labels := c.Config.Labels
		if labels["devcontainer.local_folder"] != path {
			continue
		}
		// Some older setups only set local_folder. If config_file is present it must match.
		if p := labels["devcontainer.config_file"]; p != "" && p != confFile {
			continue
		}
		matched = append(matched, c)
	}
	return matched
}

func updateStatuses(projects []Project) error {
	cs, err := inspectContainers()
	if err != nil {
		for i := range projects {
			if projects[i].Valid {
				projects[i].State = "UNKNOWN"
			}
		}
		return err
	}
	for i := range projects {
		if !projects[i].Valid {
			continue
		}
		c, found, n := matchContainer(projects[i].Path, cs)
		if !found {
			continue
		}
		projects[i].ID = c.ID
		projects[i].ContainerName = strings.TrimPrefix(c.Name, "/")
		switch {
		case c.State.Running:
			projects[i].State = "RUNNING"
		case c.State.Status == "exited":
			projects[i].State = "STOPPED"
		case c.State.Status != "":
			projects[i].State = strings.ToUpper(c.State.Status)
		default:
			projects[i].State = "UNKNOWN"
		}
		if n > 1 {
			projects[i].State = fmt.Sprintf("%s (%d)", projects[i].State, n)
		}
	}
	return nil
}
