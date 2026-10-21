package commands

func console() error {
	if startScreen() {
		defer stopScreen()
	}
	return projectsMenu()
}
