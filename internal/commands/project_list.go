package commands

import (
	"errors"
)

func projectsForCommand() ([]Project, error) {
	projects, err := loadProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, errors.New("no projects registered; run `dch register --path PROJECT_PATH` first")
	}
	return projects, nil
}
