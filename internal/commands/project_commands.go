package commands

import (
	"errors"
	"fmt"
	"os"
)

func projectsForCommand() ([]Project, error) {
	projects, err := loadProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, errors.New("no projects registered; run `dch config` first")
	}
	return projects, nil
}

func selectRunnableProject(command string) (Project, error) {
	projects, err := projectsForCommand()
	if err != nil {
		return Project{}, err
	}
	statusErr := updateStatuses(projects)
	if statusErr != nil {
		fmt.Fprintln(os.Stderr, "Warning: cannot query Docker status:", statusErr)
	}
	if err := requireExecutable("devcontainer"); err != nil {
		return Project{}, err
	}
	if statusErr != nil {
		return Project{}, fmt.Errorf("Docker is not available: %w", statusErr)
	}
	p, err := selectProject(command, projects)
	if err != nil {
		return Project{}, err
	}
	if !p.Valid {
		return Project{}, fmt.Errorf("missing or invalid .devcontainer/devcontainer.json at %s", p.Path)
	}
	fmt.Printf("\nSelected: %s (%s)\n", p.Name, p.Path)
	return p, nil
}
