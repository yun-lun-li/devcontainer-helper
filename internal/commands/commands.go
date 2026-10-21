package commands

import (
	"errors"
	"fmt"
)

// Run dispatches one CLI command. Cancelling an interactive menu exits cleanly.
func Run(args []string) error {
	if len(args) == 0 {
		return mainMenu()
	}
	if len(args) != 1 {
		usage()
		return errors.New("expected one command")
	}

	var err error
	switch args[0] {
	case "help", "-h", "--help":
		usage()
	case "config":
		err = configure()
	case "list":
		err = list()
	case "build":
		err = build()
	case "up":
		err = up()
	case "connect":
		err = connect()
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
	if errors.Is(err, errCancelled) {
		return nil
	}
	return err
}

func usage() {
	fmt.Print(`dch — devcontainer helper

Usage:
  dch           Open the interactive menu
  dch config    Add or remove project paths (interactive)
  dch list      Show registered projects and Docker status
  dch build     Build without cache (optionally recreate container)
  dch up        Start a devcontainer
  dch connect   Open a shell inside a running devcontainer
  dch help      Show this help

Prerequisites: devcontainer CLI, Docker (daemon running)
Config file:   ~/.dch/config (JSON; stores only workspace paths)
Navigation:    ↑ / ↓ + Enter, q/Esc to go back (or numbered selection if not a TTY)
`)
}
