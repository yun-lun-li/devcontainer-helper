package console

import "dch/internal/common"

type Project = common.Project
type DockerContainer = common.DockerContainer

func Run() error {
	if startScreen() {
		defer stopScreen()
	}
	return projectsMenu()
}
