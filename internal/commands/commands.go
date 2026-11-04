package commands

import (
	"fmt"
)

// Run dispatches a CLI command. The console is also the default command.
func Run(args []string, openConsole func() error) error {
	if len(args) > 0 && args[0] == "host-pass" {
		return runHostPass(args[1:])
	}
	if len(args) == 0 {
		return launchConsole(openConsole)
	}
	return runCommand(args, openConsole)
}

func runCommand(args []string, openConsole func() error) error {
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
		return launchConsole(openConsole)
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

func launchConsole(openConsole func() error) error {
	if openConsole == nil {
		return fmt.Errorf("console is unavailable")
	}
	checkForStartupUpdate()
	return openConsole()
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
  dch host-pass prepare --project PATH [--entry NAME]
                            Prepare a project's host pass PAT service
  dch host-pass stop --project PATH
                            Stop the service, retaining the PAT in host pass

Container actions: devcontainer CLI and Docker daemon required
Config file:   ~/.dch/config (JSON; stores only workspace paths)
Navigation:    ↑ / ↓ + Enter, q/Esc to go back (or numbered selection if not a TTY)
`)
}
