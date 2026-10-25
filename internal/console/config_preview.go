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
	pager := configPager{width: width, pageSize: rows - 6, lines: wrappedLogLines(content, width)}
	pager.maxOffset = len(pager.lines) - pager.pageSize
	if pager.maxOffset < 0 {
		pager.maxOffset = 0
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	pager.render()
	for {
		key, err := readTTYByte(os.Stdin, 0, interrupt)
		if err != nil {
			return err
		}
		done, changed, err := pager.handleKey(key, interrupt)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if changed {
			pager.render()
		}
	}
}

type configPager struct {
	width, pageSize, maxOffset, offset int
	lines                              []string
}

func (pager *configPager) render() {
	clearScreen()
	printDCHHeaderWidth("Show config", pager.width)
	fmt.Printf("  ╭%s╮\n", strings.Repeat("─", pager.width-4))
	end := pager.offset + pager.pageSize
	if end > len(pager.lines) {
		end = len(pager.lines)
	}
	for i := 0; i < pager.pageSize; i++ {
		line := ""
		if pager.offset+i < end {
			line = pager.lines[pager.offset+i]
		}
		fmt.Printf("  │ %-*s │\n", pager.width-6, line)
	}
	fmt.Printf("  ╰%s╯\n", strings.Repeat("─", pager.width-4))
	fmt.Printf("  Lines %d-%d of %d  ·  ↑/↓ or j/k scroll  ·  q/Esc back\n", pager.offset+1, end, len(pager.lines))
	fmt.Println("  Keep .devcontainer/devcontainer.local.d/ out of version control.")
}

func (pager *configPager) handleKey(key byte, interrupt chan os.Signal) (bool, bool, error) {
	switch key {
	case 'q', 'Q', 3:
		return true, false, nil
	case 'j', ' ':
		if pager.offset < pager.maxOffset {
			pager.offset++
			return false, true, nil
		}
	case 'k':
		if pager.offset > 0 {
			pager.offset--
			return false, true, nil
		}
	case '\r', '\n':
		pager.offset += pager.pageSize
		if pager.offset > pager.maxOffset {
			pager.offset = pager.maxOffset
		}
		return false, true, nil
	case 27:
		return pager.handleEscape(interrupt)
	}
	return false, false, nil
}

func (pager *configPager) handleEscape(interrupt chan os.Signal) (bool, bool, error) {
	next, err := readTTYByte(os.Stdin, 200*time.Millisecond, interrupt)
	if err != nil || (next != '[' && next != 'O') {
		return true, false, nil
	}
	direction, err := readTTYByte(os.Stdin, 200*time.Millisecond, interrupt)
	if err != nil {
		return true, false, nil
	}
	if direction == 'A' && pager.offset > 0 {
		pager.offset--
		return false, true, nil
	}
	if direction == 'B' && pager.offset < pager.maxOffset {
		pager.offset++
		return false, true, nil
	}
	return false, false, nil
}
