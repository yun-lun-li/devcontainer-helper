package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
	"unicode/utf8"
)

var input = bufio.NewReader(os.Stdin)
var errCancelled = errors.New("cancelled")

type menuView struct {
	title   string
	project *Project
	options []string
}

// choose shows a native arrow-key menu for Linux/macOS terminals (via stty).
// For redirected stdin, or when stty is unavailable, it falls back to numbered choices.
func choose(title string, options []string) (int, error) {
	return chooseView(menuView{title: title, options: options})
}

func chooseView(view menuView) (int, error) {
	if len(view.options) == 0 {
		return -1, errors.New("no options")
	}
	if index, ok, err := arrowChoose(view); ok {
		return index, err
	}
	clearScreen()
	color := menuColorEnabled()
	printMenuHeader(view, color)
	for i, opt := range view.options {
		if color {
			fmt.Printf("  \x1b[1;36m%d)\x1b[0m %s\n", i+1, styledMenuOption(opt))
		} else {
			fmt.Printf("  %d) %s\n", i+1, opt)
		}
	}
	for {
		fmt.Print("Choose number (q to cancel): ")
		line, err := input.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return -1, err
		}
		line = strings.TrimSpace(line)
		if line == "q" || line == "Q" {
			return -1, errCancelled
		}
		var n int
		if _, e := fmt.Sscanf(line, "%d", &n); e == nil && n >= 1 && n <= len(view.options) {
			return n - 1, nil
		}
		if errors.Is(err, io.EOF) {
			return -1, errCancelled
		}
		fmt.Println("Invalid selection.")
	}
}

func runStty(args ...string) ([]byte, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	return cmd.Output()
}

func arrowChoose(view menuView) (index int, used bool, err error) {
	opts := view.options
	if !terminalInput() {
		return 0, false, nil
	}
	if !terminalOutput() {
		return 0, false, nil
	}
	if _, e := exec.LookPath("stty"); e != nil {
		return 0, false, nil
	}
	orig, e := runStty("-g")
	if e != nil {
		return 0, false, nil
	}
	if _, e = runStty("-echo", "-icanon", "min", "0", "time", "1"); e != nil {
		return 0, false, nil
	}
	defer runStty(strings.TrimSpace(string(orig)))
	// Restore terminal when interrupted, including when running via WSL.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	clearScreen()
	if screenActive {
		fmt.Print("\x1b[?25l")
	}
	color := menuColorEnabled()
	printMenuHeader(view, color)
	index = 0
	footerLines := 2
	render := func() {
		for i, opt := range opts {
			if color {
				if i == index {
					fmt.Printf("\r\033[2K\x1b[1;30;46m › %-*s \x1b[0m\n", menuWidth(opts), opt)
				} else {
					fmt.Printf("\r\033[2K  \x1b[36m·\x1b[0m %s\n", styledMenuOption(opt))
				}
			} else {
				prefix := "  "
				if i == index {
					prefix = "> "
				}
				fmt.Printf("\r\033[2K%s%s\n", prefix, opt)
			}
		}
		fmt.Print("\r\033[2K\n\r\033[2K")
		if color {
			fmt.Print("  \x1b[2m↑/↓ move  ·  Enter select  ·  q/Esc back\x1b[0m\n")
		} else {
			fmt.Print("  ↑/↓ move  ·  Enter select  ·  q/Esc back\n")
		}
		fmt.Printf("\033[%dA", len(opts)+footerLines)
	}
	render()
	for {
		b, e := readTTYByte(os.Stdin, 0, sig)
		if e != nil {
			fmt.Printf("\033[%dB", len(opts)+footerLines)
			return 0, true, e
		}
		switch b {
		case 3, 'q', 'Q':
			fmt.Printf("\033[%dB", len(opts)+footerLines)
			return -1, true, errCancelled
		case '\r', '\n':
			fmt.Printf("\033[%dB", len(opts)+footerLines)
			return index, true, nil
		case 'j':
			index = (index + 1) % len(opts)
		case 'k':
			index = (index + len(opts) - 1) % len(opts)
		case 27:
			next, e := readTTYByte(os.Stdin, 200*time.Millisecond, sig)
			if e != nil {
				fmt.Printf("\033[%dB", len(opts)+footerLines)
				return -1, true, errCancelled
			}
			if next != '[' && next != 'O' {
				fmt.Printf("\033[%dB", len(opts)+footerLines)
				return -1, true, errCancelled
			}
			dir, e := readTTYByte(os.Stdin, 200*time.Millisecond, sig)
			if e != nil {
				fmt.Printf("\033[%dB", len(opts)+footerLines)
				return -1, true, errCancelled
			}
			if dir == 'A' {
				index = (index + len(opts) - 1) % len(opts)
			}
			if dir == 'B' {
				index = (index + 1) % len(opts)
			}
		default:
			continue
		}
		render()
	}
}

func printMenuHeader(view menuView, color bool) {
	if view.project == nil {
		if color {
			fmt.Printf("\n\x1b[1;35m%s\x1b[0m\n\n", view.title)
		} else {
			fmt.Printf("\n%s\n\n", view.title)
		}
		return
	}
	p := view.project
	if color {
		fmt.Printf("\n  \x1b[1;35m◆  PROJECT\x1b[0m\n")
		fmt.Printf("  \x1b[1m%s\x1b[0m\n\n", p.Name)
		fmt.Printf("  %s● %s\x1b[0m\n", statusColor(p.State), p.State)
		fmt.Printf("  \x1b[2m⌂ %s\x1b[0m\n\n", p.Path)
		fmt.Printf("  \x1b[1;35mACTIONS\x1b[0m\n\n")
	} else {
		fmt.Printf("\n  ◆  PROJECT\n  %s\n\n  ● %s\n  ⌂ %s\n\n  ACTIONS\n\n", p.Name, p.State, p.Path)
	}
}

func styledMenuOption(option string) string {
	label, description, found := strings.Cut(option, "  ")
	if found {
		return "\x1b[1;36m" + label + "\x1b[0m  \x1b[2m" + strings.TrimLeft(description, " ") + "\x1b[0m"
	}
	return "\x1b[1;36m" + option + "\x1b[0m"
}

func menuColorEnabled() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func menuWidth(options []string) int {
	width := 0
	for _, option := range options {
		if n := utf8.RuneCountInString(option); n > width {
			width = n
		}
	}
	return width
}

// VMIN=0 and VTIME>0 can make an idle terminal read return io.EOF.
// Keep waiting for a key until the optional escape-sequence timeout expires.
func readTTYByte(r io.Reader, timeout time.Duration, sig <-chan os.Signal) (byte, error) {
	deadline := time.Now().Add(timeout)
	var buf [1]byte
	for {
		select {
		case <-sig:
			return 0, errCancelled
		default:
		}
		n, err := r.Read(buf[:])
		if n > 0 {
			return buf[0], nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if timeout > 0 && time.Now().After(deadline) {
			return 0, io.EOF
		}
	}
}

func confirm(question string, defaultYes bool) (bool, error) {
	screenHeading("Confirm")
	def := "y/N"
	if defaultYes {
		def = "Y/n"
	}
	fmt.Printf("%s [%s]: ", question, def)
	line, err := input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "" {
		return defaultYes, nil
	}
	return answer == "y" || answer == "yes", nil
}
