package commands

import (
	"fmt"
)

// Run dispatches a CLI command. The console is also the default command.
func Run(args []string, openConsole func() error) error {
	if len(args) == 0 {
		if openConsole == nil {
			return fmt.Errorf("console is unavailable")
		}
		return openConsole()
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return fmt.Errorf("help does not accept arguments")
		}
		usage()
		return nil
	case "console":
		if len(args) != 1 {
			return fmt.Errorf("console does not accept arguments")
		}
		if openConsole == nil {
			return fmt.Errorf("console is unavailable")
		}
		return openConsole()
	case "register":
		if len(args) != 3 || args[1] != "--path" || args[2] == "" {
			return fmt.Errorf("usage: dch register --path PROJECT_PATH")
		}
		return registerProject(args[2])
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("list does not accept arguments")
		}
		return list()
	case "update":
		if len(args) != 1 {
			return fmt.Errorf("update does not accept arguments")
		}
		return update()
	case "version":
		if len(args) != 1 {
			return fmt.Errorf("version does not accept arguments")
		}
		fmt.Printf("dch %s\n", version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Print(`dch — devcontainer helper

Usage:
  dch                       Open the Projects console
  dch console               Open the Projects console
  dch register --path PATH  Register a project (relative paths accepted)
  dch list                  Show registered projects and Docker status
  dch update                Update dch to the latest release
  dch version               Show the installed version
  dch help                  Show this help

Container actions: devcontainer CLI and Docker daemon required
Config file:   ~/.dch/config (JSON; stores only workspace paths)
Navigation:    ↑ / ↓ + Enter, q/Esc to go back (or numbered selection if not a TTY)
`)
}
