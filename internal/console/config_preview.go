package console

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"dch/internal/common"
)

func showProjectConfig(p Project) error {
	data, err := common.MergedDevcontainerConfig(p.Path)
	if err != nil {
		return err
	}
	content := strings.TrimSuffix(string(data), "\n")
	if !screenActive {
		fmt.Println(content)
		fmt.Println("\nKeep .devcontainer/devcontainer.local.d/ out of version control.")
		return nil
	}
	return showConfigPages(content)
}

func showConfigPages(content string) error {
	original, err := runStty("-g")
	if err != nil {
		fmt.Println(content)
		screenWait()
		return nil
	}
	if _, err := runStty("-echo", "-icanon", "min", "0", "time", "1"); err != nil {
		return err
	}
	defer runStty(strings.TrimSpace(string(original)))

	rows, width, _ := terminalSize()
	if rows < 8 {
		rows = 8
	}
	if width < 30 {
		width = 30
	}
	pageSize := rows - 6
	lines := wrappedLogLines(content, width)
	maxOffset := len(lines) - pageSize
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := 0
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	render := func() {
		clearScreen()
		printDCHHeaderWidth("Show config", width)
		fmt.Printf("  ╭%s╮\n", strings.Repeat("─", width-4))
		end := offset + pageSize
		if end > len(lines) {
			end = len(lines)
		}
		innerWidth := width - 6
		for i := 0; i < pageSize; i++ {
			line := ""
			if offset+i < end {
				line = lines[offset+i]
			}
			fmt.Printf("  │ %-*s │\n", innerWidth, line)
		}
		fmt.Printf("  ╰%s╯\n", strings.Repeat("─", width-4))
		fmt.Printf("  Lines %d-%d of %d  ·  ↑/↓ or j/k scroll  ·  q/Esc back\n", offset+1, end, len(lines))
		fmt.Println("  Keep .devcontainer/devcontainer.local.d/ out of version control.")
	}
	render()
	for {
		key, err := readTTYByte(os.Stdin, 0, interrupt)
		if err != nil {
			return err
		}
		switch key {
		case 'q', 'Q', 3:
			return nil
		case 'j', ' ':
			if offset < maxOffset {
				offset++
			}
		case 'k':
			if offset > 0 {
				offset--
			}
		case '\r', '\n':
			offset += pageSize
			if offset > maxOffset {
				offset = maxOffset
			}
		case 27:
			next, err := readTTYByte(os.Stdin, 200*time.Millisecond, interrupt)
			if err != nil || (next != '[' && next != 'O') {
				return nil
			}
			direction, err := readTTYByte(os.Stdin, 200*time.Millisecond, interrupt)
			if err != nil {
				return nil
			}
			if direction == 'A' && offset > 0 {
				offset--
			} else if direction == 'B' && offset < maxOffset {
				offset++
			}
		default:
			continue
		}
		render()
	}
}
