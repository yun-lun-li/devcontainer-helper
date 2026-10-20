package main

import (
	"fmt"
	"os"

	"dch/internal/commands"
)

func main() {
	if err := commands.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
