package console

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

type checkboxMenu struct {
	title   string
	labels  []string
	checked []bool
	index   int
}

func chooseCheckboxes(title string, labels []string, checked []bool) ([]bool, error) {
	menu := checkboxMenu{title: title, labels: labels, checked: append([]bool(nil), checked...)}
	if len(labels) == 0 || len(labels) != len(checked) {
		return nil, fmt.Errorf("invalid checkbox options")
	}
	if terminalInput() && terminalOutput() {
		if original, err := runStty("-g"); err == nil {
			return menu.terminal(strings.TrimSpace(string(original)))
		}
	}
	return menu.numbered()
}

func (m *checkboxMenu) render(numbered bool) {
	clearScreen()
	printMenuHeader(menuView{title: m.title}, menuColorEnabled())
	for i, label := range m.labels {
		mark := " "
		if m.checked[i] {
			mark = "x"
		}
		prefix := "  "
		if i == m.index {
			prefix = "› "
		}
		if numbered {
			prefix = strconv.Itoa(i+1) + ") "
		}
		fmt.Printf("  %s[%s] %s\n", prefix, mark, label)
	}
	if numbered {
		fmt.Print("Number to enable/disable · q to go back: ")
		return
	}
	fmt.Println("\n  ↑/↓ move · Space/Enter enable/disable · q/Esc back")
}

func (m *checkboxMenu) numbered() ([]bool, error) {
	for {
		m.render(true)
		line, err := input.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "q" || errors.Is(err, io.EOF) {
			return nil, errCancelled
		}
		number, err := strconv.Atoi(line)
		if err == nil && number > 0 && number <= len(m.labels) {
			m.checked[number-1] = !m.checked[number-1]
			return m.checked, nil
		}
	}
}

func (m *checkboxMenu) terminal(original string) ([]bool, error) {
	if _, err := runStty("-echo", "-icanon", "min", "0", "time", "1"); err != nil {
		return nil, err
	}
	defer runStty(original)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	for {
		m.render(false)
		key, err := checkboxKey(os.Stdin, interrupt)
		if err != nil {
			return nil, err
		}
		apply, err := m.handleKey(key)
		if err != nil {
			return nil, err
		}
		if apply {
			return m.checked, nil
		}
	}
}

func (m *checkboxMenu) handleKey(key byte) (bool, error) {
	switch key {
	case 3, 'q', 'Q':
		return false, errCancelled
	case '\r', '\n', ' ':
		m.checked[m.index] = !m.checked[m.index]
		return true, nil
	case 'j':
		m.index = (m.index + 1) % len(m.labels)
	case 'k':
		m.index = (m.index + len(m.labels) - 1) % len(m.labels)
	}
	return false, nil
}

func checkboxKey(reader io.Reader, interrupt <-chan os.Signal) (byte, error) {
	key, err := readTTYByte(reader, 0, interrupt)
	if err != nil || key != 27 {
		return key, err
	}
	next, err := readTTYByte(reader, 200*time.Millisecond, interrupt)
	if err != nil || (next != '[' && next != 'O') {
		return 'q', nil
	}
	direction, err := readTTYByte(reader, 200*time.Millisecond, interrupt)
	if err != nil {
		return 'q', nil
	}
	switch direction {
	case 'A':
		return 'k', nil
	case 'B':
		return 'j', nil
	}
	return 0, nil
}
