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
)

var input = bufio.NewReader(os.Stdin)
var errCancelled = errors.New("cancelled")

// choose shows a native arrow-key menu for Linux/macOS terminals (via stty).
// For redirected stdin, or when stty is unavailable, it falls back to numbered choices.
func choose(title string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, errors.New("no options")
	}
	if index, ok, err := arrowChoose(title, options); ok {
		return index, err
	}
	fmt.Println("\n" + title)
	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt)
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
		if _, e := fmt.Sscanf(line, "%d", &n); e == nil && n >= 1 && n <= len(options) {
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

func arrowChoose(title string, opts []string) (index int, used bool, err error) {
	st, statErr := os.Stdin.Stat()
	if statErr != nil || st.Mode()&os.ModeCharDevice == 0 {
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
	fmt.Printf("\n%s  (↑/↓ Enter, q/Esc cancel)\n", title)
	index = 0
	render := func() {
		for i, opt := range opts {
			prefix := "  "
			if i == index {
				prefix = "> "
			}
			fmt.Printf("\r\033[2K%s%s\n", prefix, opt)
		}
		fmt.Printf("\033[%dA", len(opts))
	}
	render()
	for {
		b, e := readTTYByte(os.Stdin, 0, sig)
		if e != nil {
			fmt.Printf("\033[%dB", len(opts))
			return 0, true, e
		}
		switch b {
		case 3, 'q', 'Q':
			fmt.Printf("\033[%dB", len(opts))
			return -1, true, errCancelled
		case '\r', '\n':
			fmt.Printf("\033[%dB", len(opts))
			fmt.Printf("Selected: %s\n", opts[index])
			return index, true, nil
		case 'j':
			index = (index + 1) % len(opts)
		case 'k':
			index = (index + len(opts) - 1) % len(opts)
		case 27:
			next, e := readTTYByte(os.Stdin, 200*time.Millisecond, sig)
			if e != nil {
				fmt.Printf("\033[%dB", len(opts))
				return -1, true, errCancelled
			}
			if next != '[' && next != 'O' {
				fmt.Printf("\033[%dB", len(opts))
				return -1, true, errCancelled
			}
			dir, e := readTTYByte(os.Stdin, 200*time.Millisecond, sig)
			if e != nil {
				fmt.Printf("\033[%dB", len(opts))
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
