package commands

import (
	"errors"

	"dch/internal/common"
)

func projectsForCommand() ([]common.Project, error) {
	projects, err := common.LoadProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, errors.New("no projects registered; run `dch register --path PROJECT_PATH` first")
	}
	return projects, nil
}
