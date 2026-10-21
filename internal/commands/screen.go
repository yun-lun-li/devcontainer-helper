package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var screenActive bool

func startScreen() bool {
	if !terminalInput() || !terminalOutput() {
		return false
	}
	if _, err := exec.LookPath("stty"); err != nil {
		return false
	}
	if _, err := runStty("-g"); err != nil {
		return false
	}
	screenActive = true
	fmt.Print("\x1b[?1049h\x1b[?25l")
	return true
}

func stopScreen() {
	if screenActive {
		fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
		screenActive = false
	}
}

func clearScreen() {
	if screenActive {
		fmt.Print("\x1b[H\x1b[2J")
	}
}

func screenHeading(title string) {
	clearScreen()
	if screenActive {
		fmt.Print("\x1b[?25h")
		if menuColorEnabled() {
			fmt.Printf("\x1b[1;35m%s\x1b[0m\n\n", title)
		} else {
			fmt.Printf("%s\n\n", title)
		}
	}
}

func screenNotice(title string, message string) {
	if !screenActive {
		fmt.Println(message)
		return
	}
	screenHeading(title)
	fmt.Println(message)
	screenWait()
}

func screenWait() {
	if !screenActive {
		return
	}
	fmt.Print("\nPress Enter to continue… ")
	_, _ = input.ReadString('\n')
}

func screenError(err error) {
	if !screenActive || err == nil || errors.Is(err, errCancelled) {
		return
	}
	screenNotice("Error", err.Error())
}

func terminalInput() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func terminalOutput() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
