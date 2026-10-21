package commands

import (
	"errors"
	"fmt"
	"os"
)

func mainMenu() error {
	if startScreen() {
		defer stopScreen()
	}
	options := []string{
		"Projects              Open a project and choose an action",
		"Manage registrations  Add or remove project paths",
		"Quit",
	}
	for {
		choice, err := choose("dch  /  DEV CONTAINER HELPER", options)
		if errors.Is(err, errCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		switch choice {
		case 0:
			err = projectsMenu()
		case 1:
			err = configure()
		case 2:
			return nil
		}
		if err != nil && !errors.Is(err, errCancelled) {
			if screenActive {
				screenError(err)
			} else {
				fmt.Fprintln(os.Stderr, "Error:", err)
			}
		}
	}
}
