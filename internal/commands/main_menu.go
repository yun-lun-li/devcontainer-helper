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
		"Connect          Open a shell in a running container",
		"Start            Start a project's container",
		"Build            Rebuild a container or image",
		"Manage projects  View, add, or remove projects",
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
			err = connect()
		case 1:
			err = up()
		case 2:
			err = build()
		case 3:
			err = configure()
		case 4:
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
