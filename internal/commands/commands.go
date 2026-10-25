package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"dch/internal/plugins"
)

// Run dispatches a CLI command. The console is also the default command.
func Run(args []string, openConsole func() error) error {
	if len(args) == 0 {
		return runConsole(openConsole)
	}

	return runCommand(args, openConsole)
}

func runConsole(openConsole func() error) error {
	if openConsole == nil {
		return fmt.Errorf("console is unavailable")
	}
	checkForStartupUpdate()
	return openConsole()
}

func runCommand(args []string, openConsole func() error) error {
	switch args[0] {
	case "help", "-h", "--help":
		return runHelp(args)
	case "console":
		return runConsoleCommand(args, openConsole)
	case "register":
		return runRegister(args)
	case "list":
		return runList(args)
	case "update":
		return runUpdate(args)
	case "version":
		return runVersion(args)
	case "_github-app-serve":
		return runGitHubAppServe(args)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runHelp(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("help does not accept arguments")
	}
	usage()
	return nil
}

func runConsoleCommand(args []string, openConsole func() error) error {
	if len(args) != 1 {
		return fmt.Errorf("console does not accept arguments")
	}
	return runConsole(openConsole)
}

func runRegister(args []string) error {
	if len(args) != 3 || args[1] != "--path" || args[2] == "" {
		return fmt.Errorf("usage: dch register --path PROJECT_PATH")
	}
	return registerProject(args[2])
}

func runList(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("list does not accept arguments")
	}
	return list()
}

func runUpdate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("update does not accept arguments")
	}
	return update()
}

func runVersion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("version does not accept arguments")
	}
	fmt.Printf("dch %s\n", version)
	return nil
}

func runGitHubAppServe(args []string) error {
	if len(args) != 3 || args[1] != "--project" || args[2] == "" {
		return fmt.Errorf("usage: dch _github-app-serve --project PATH")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return plugins.NewManager().ServeProject(ctx, args[2])
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
Config file:   ~/.dch/config (JSON; stores project paths and plugin settings)
Navigation:    ↑ / ↓ + Enter, q/Esc to go back (or numbered selection if not a TTY)
`)
}
