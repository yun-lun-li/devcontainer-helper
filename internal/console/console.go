package console

import (
	"dch/internal/common"
	"dch/internal/plugins"
)

type Project = common.Project
type DockerContainer = common.DockerContainer

func Run(manager *plugins.Manager) error {
	if startScreen() {
		defer stopScreen()
	}
	return projectsMenu(manager)
}
