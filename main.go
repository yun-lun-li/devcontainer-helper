package main

import (
	"fmt"
	"os"

	"dch/internal/commands"
	"dch/internal/console"
	"dch/internal/plugins"
	"dch/internal/plugins/githubapp"
)

func main() {
	manager := plugins.NewManager()
	if err := manager.Register(githubapp.New()); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if err := commands.Run(os.Args[1:], manager, func() error { return console.Run(manager) }); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
