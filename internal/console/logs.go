package console

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// showLogs keeps the application header fixed and scrolls only the log viewport.
func showLogs(title string, output []byte) {
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
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

	width := projectMenuWidth() + 4
	if width < 24 {
		width = 24
	}
	rows := 24
	if size, err := runStty("size"); err == nil {
		var columns int
		if _, err := fmt.Sscan(string(size), &rows, &columns); err != nil || rows < 5 {
			rows = 24
		}
	}
	pageSize := rows - 4
	if pageSize < 1 {
		pageSize = 1
	}
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		runes := []rune(line)
		if len(runes) == 0 {
			wrapped = append(wrapped, "")
		}
		for len(runes) > 0 {
			n := width
			if len(runes) < n {
				n = len(runes)
			}
			wrapped = append(wrapped, string(runes[:n]))
			runes = runes[n:]
		}
	}
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	start := len(wrapped) - pageSize
	if start < 0 {
		start = 0
	}
	color := menuColorEnabled()
	render := func() {
		clearScreen()
		printDCHHeader(title)
		end := start + pageSize
		if end > len(wrapped) {
			end = len(wrapped)
		}
		for i := start; i < end; i++ {
			fmt.Println(wrapped[i])
		}
		for i := end - start; i < pageSize; i++ {
			fmt.Println()
		}
		if color {
			fmt.Print("\x1b[2m↑/↓ scroll  PgUp/PgDn page  Home/End jump  Enter return\x1b[0m")
		} else {
			fmt.Print("↑/↓ scroll  PgUp/PgDn page  Home/End jump  Enter return")
		}
	}
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
			if start+pageSize < len(wrapped) {
				start++
			} else {
				changed = false
			}
		case 'g':
			start = 0
		case 'G':
			start = len(wrapped) - pageSize
			if start < 0 {
				start = 0
			}
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
				if start+pageSize < len(wrapped) {
					start++
				} else {
					changed = false
				}
			case '5':
				_, _ = readTTYByte(inputTTYReader{}, 200*time.Millisecond, nil)
				start -= pageSize
				if start < 0 {
					start = 0
				}
			case '6':
				_, _ = readTTYByte(inputTTYReader{}, 200*time.Millisecond, nil)
				start += pageSize
				if start+pageSize > len(wrapped) {
					start = len(wrapped) - pageSize
				}
				if start < 0 {
					start = 0
				}
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

type inputTTYReader struct{}

func (inputTTYReader) Read(p []byte) (int, error) { return os.Stdin.Read(p) }
