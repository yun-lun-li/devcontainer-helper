package console

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
		if view.project != nil {
			fmt.Printf("  %d) %s\n", i+1, projectOption(opt, false, color, projectMenuWidth()-3))
		} else if color {
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
	projectWidth := projectMenuWidth()
	footerLines := 2
	render := func() {
		for i, opt := range opts {
			if view.project != nil {
				fmt.Printf("\r\033[2K%s\n", projectOption(opt, i == index, color, projectWidth))
			} else if color {
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
	printProjectHeader(os.Stdout, *p, color, projectMenuWidth())
}

const (
	cardPurple = "\x1b[38;2;177;139;230m"
	cardBlue   = "\x1b[38;2;111;170;255m"
	cardCyan   = "\x1b[38;2;106;216;232m"
	cardAmber  = "\x1b[38;2;255;192;119m"
	cardPink   = "\x1b[38;2;255;127;145m"
	cardMuted  = "\x1b[38;2;142;162;188m"
	cardFill   = "\x1b[48;2;24;35;58m"
	cardSelect = "\x1b[48;2;47;153;177m\x1b[38;2;11;31;46m"
	cardReset  = "\x1b[0m"
)

func projectMenuWidth() int {
	width := 80
	if size, err := runStty("size"); err == nil {
		var rows, cols int
		if _, err := fmt.Sscan(string(size), &rows, &cols); err == nil && cols > 0 {
			width = cols
		}
	}
	if width > 100 {
		width = 100
	}
	if width < 24 {
		width = 24
	}
	return width - 4
}

func truncateMenuText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func printProjectHeader(w io.Writer, p Project, color bool, width int) {
	if width < 20 {
		width = 20
	}
	style := func(code, s string) string {
		if !color {
			return s
		}
		return code + s + cardReset
	}
	name := truncateMenuText(p.Name, width-8)
	breadcrumbName := truncateMenuText(p.Name, width-21)
	borderCount := width - 7 - utf8.RuneCountInString(name)
	fmt.Fprintf(w, "  %s   %s  /  %s\n\n", style(cardPurple, "dch"), style(cardMuted, "Projects"), style(cardMuted, breadcrumbName))
	fmt.Fprintf(w, "  %s%s%s\n", style(cardPurple, "╭─ ◆ "), style(cardPurple, name), style(cardPurple, " "+strings.Repeat("─", borderCount)+"╮"))
	line := func(content string, contentColor string) {
		content = truncateMenuText(content, width-4)
		padding := strings.Repeat(" ", width-4-utf8.RuneCountInString(content))
		if color {
			fmt.Fprintf(w, "  %s%s│  %s%s%s%s│%s\n", cardFill, cardPurple, contentColor, content, padding, cardPurple, cardReset)
		} else {
			fmt.Fprintf(w, "  │  %s%s│\n", content, padding)
		}
	}
	line("", "")
	stateColor := cardAmber
	if strings.HasPrefix(p.State, "RUNNING") {
		stateColor = cardBlue
	}
	line("● "+p.State, stateColor)
	line("", "")
	line("🗁 "+p.Path, cardCyan)
	line("", "")
	fmt.Fprintf(w, "  %s\n\n", style(cardPurple, "╰"+strings.Repeat("─", width-2)+"╯"))
	fmt.Fprintf(w, "  %s\n  %s\n", style(cardPurple, "ACTIONS"), style(cardMuted, strings.Repeat("─", width)))
}

func projectOption(option string, selected, color bool, width int) string {
	label, description, _ := strings.Cut(option, "  ")
	label = truncateMenuText(label, width-5)
	if width < 42 {
		description = ""
	}
	labelWidth := 25
	if labelWidth > width-3 {
		labelWidth = width - 3
	}
	plain := "  "
	if selected {
		plain = " ›"
	}
	plain += "  " + label
	pad := labelWidth - utf8.RuneCountInString(label)
	if pad > 0 && description != "" {
		plain += strings.Repeat(" ", pad)
	}
	available := width - utf8.RuneCountInString(plain)
	description = truncateMenuText(description, available)
	plain += description
	if !color {
		return plain
	}
	if selected {
		return cardSelect + plain + strings.Repeat(" ", width-utf8.RuneCountInString(plain)) + cardReset
	}
	optionColor := cardBlue
	switch {
	case strings.Contains(label, "Connect"):
		optionColor = cardCyan
	case strings.Contains(label, "Stop"), strings.Contains(label, "Start"):
		optionColor = cardAmber
	case strings.Contains(label, "Build"):
		optionColor = cardPurple
	case strings.Contains(label, "Remove"):
		optionColor = cardPink
	}
	spacing := ""
	if description != "" && pad > 0 {
		spacing = strings.Repeat(" ", pad)
	}
	return optionColor + "     " + label + cardReset + spacing + cardMuted + description + cardReset
}

func styledMenuOption(option string) string {
	label, description, found := strings.Cut(option, "  ")
	if found {
		description = strings.TrimLeft(description, " ")
		if strings.HasPrefix(description, "[RUNNING") {
			if end := strings.IndexByte(description, ']'); end >= 0 {
				return "\x1b[1;36m" + label + "\x1b[0m  \x1b[94m" + description[:end+1] + "\x1b[0m  \x1b[2m" + strings.TrimLeft(description[end+1:], " ") + "\x1b[0m"
			}
		}
		return "\x1b[1;36m" + label + "\x1b[0m  \x1b[2m" + description + "\x1b[0m"
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
