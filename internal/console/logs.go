package console

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var logANSISequence = regexp.MustCompile("\\x1b\\[[0-?]*[ -/]*[@-~]")

type liveLogWriter struct {
	mu    sync.Mutex
	title string
	text  strings.Builder
	rows  int
	width int
}

func (w *liveLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.text.Write(p)
	renderLogBox(w.title, w.text.String(), 0, true, w.rows, w.width)
	return len(p), nil
}

func (w *liveLogWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.String()
}

// runDevcontainerWithLogs streams build output into a bounded, bordered viewport.
func runDevcontainerWithLogs(cmd *exec.Cmd, title, command string, onExit func()) error {
	rows, width, _ := terminalSize()
	writer := &liveLogWriter{title: title, rows: rows, width: width}
	_, _ = writer.Write([]byte(command))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		if onExit != nil {
			onExit()
		}
		return err
	}
	err := cmd.Wait()
	if onExit != nil {
		onExit()
	}
	showLogs(title, []byte(writer.String()))
	return err
}

// showLogs keeps the application bar fixed and scrolls only the bordered log box.
func showLogs(title string, output []byte) {
	if !screenActive || !terminalInput() || !terminalOutput() {
		fmt.Print(string(output))
		return
	}
	original, err := runStty("-g")
	if err != nil {
		fmt.Print(string(output))
		return
	}
	if _, err := runStty("-echo", "-icanon", "min", "1", "time", "0"); err != nil {
		fmt.Print(string(output))
		return
	}
	defer runStty(strings.TrimSpace(string(original)))

	rows, width, _ := terminalSize()
	pageSize := rows - 6
	if pageSize < 1 {
		pageSize = 1
	}
	lines := wrappedLogLines(string(output), width)
	start := len(lines) - pageSize
	if start < 0 {
		start = 0
	}
	render := func() { renderLogBox(title, string(output), start, false, rows, width) }
	render()
	for {
		b, err := readTTYByte(inputTTYReader{}, 0, nil)
		if err != nil {
			return
		}
		changed := true
		switch b {
		case '\r', '\n', 'q', 'Q':
			return
		case 'k':
			if start > 0 {
				start--
			} else {
				changed = false
			}
		case 'j':
			if start+pageSize < len(lines) {
				start++
			} else {
				changed = false
			}
		case 'g':
			start = 0
		case 'G':
			start = max(0, len(lines)-pageSize)
		case 27:
			next, err := readTTYByte(inputTTYReader{}, 200*time.Millisecond, nil)
			if err != nil || next != '[' {
				changed = false
				break
			}
			dir, err := readTTYByte(inputTTYReader{}, 200*time.Millisecond, nil)
			if err != nil {
				changed = false
				break
			}
			switch dir {
			case 'A':
				if start > 0 {
					start--
				} else {
					changed = false
				}
			case 'B':
				if start+pageSize < len(lines) {
					start++
				} else {
					changed = false
				}
			case '5':
				_, _ = readTTYByte(inputTTYReader{}, 200*time.Millisecond, nil)
				start = max(0, start-pageSize)
			case '6':
				_, _ = readTTYByte(inputTTYReader{}, 200*time.Millisecond, nil)
				start = min(max(0, len(lines)-pageSize), start+pageSize)
			case 'H':
				start = 0
			case 'F':
				start = max(0, len(lines)-pageSize)
			default:
				changed = false
			}
		default:
			changed = false
		}
		if changed {
			render()
		}
	}
}

func renderLogBox(title, output string, start int, running bool, rows, width int) {
	if rows < 8 {
		rows = 8
	}
	if width < 30 {
		width = 30
	}
	innerWidth := width - 6
	pageSize := rows - 6
	if pageSize < 1 {
		pageSize = 1
	}
	lines := wrappedLogLines(output, width)
	if start+pageSize > len(lines) {
		start = max(0, len(lines)-pageSize)
	}
	clearScreen()
	printDCHHeaderWidth(title, width)
	fmt.Printf("  ╭%s╮\n", strings.Repeat("─", width-4))
	for i := 0; i < pageSize; i++ {
		line := ""
		if start+i < len(lines) {
			line = lines[start+i]
		}
		fmt.Printf("  │ %-*s │\n", innerWidth, line)
	}
	fmt.Printf("  ╰%s╯\n", strings.Repeat("─", width-4))
	if running {
		fmt.Print("  Build running…")
	} else {
		fmt.Print("  ↑/↓ scroll  PgUp/PgDn page  Home/End jump  Enter return")
	}
}

func wrappedLogLines(output string, width int) []string {
	plain := logANSISequence.ReplaceAllString(output, "")
	plain = strings.ReplaceAll(plain, "\r\n", "\n")
	plain = strings.ReplaceAll(plain, "\r", "\n")
	lines := strings.Split(plain, "\n")
	inner := width - 6
	if inner < 20 {
		inner = 20
	}
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		runes := []rune(line)
		if len(runes) == 0 {
			wrapped = append(wrapped, "")
		}
		for len(runes) > 0 {
			n := min(inner, len(runes))
			wrapped = append(wrapped, string(runes[:n]))
			runes = runes[n:]
		}
	}
	return wrapped
}

func terminalSize() (rows, columns int, err error) {
	rows, columns = 24, 80
	if size, e := runStty("size"); e == nil {
		if _, e = fmt.Sscan(string(size), &rows, &columns); e == nil && rows > 0 && columns > 0 {
			return rows, columns, nil
		}
	}
	return rows, columns, fmt.Errorf("terminal size unavailable")
}

type inputTTYReader struct{}

func (inputTTYReader) Read(p []byte) (int, error) { return os.Stdin.Read(p) }
