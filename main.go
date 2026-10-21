package main

import (
	"fmt"
	"os"

	"dch/internal/commands"
	"dch/internal/console"
)

func main() {
	if err := commands.Run(os.Args[1:], console.Run); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
