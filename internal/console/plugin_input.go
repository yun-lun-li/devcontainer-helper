package console

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"unicode/utf8"

	"dch/internal/plugins"
	"golang.org/x/sys/unix"
)

const maxPluginInputBytes = 4096

func (consoleInteraction) Input(ctx context.Context, title, defaultValue string, secret bool) (string, error) {
	wasScreenActive := screenActive
	stopScreen()
	if wasScreenActive {
		defer startScreen()
	}
	if secret && !terminalInput() {
		return "", fmt.Errorf("secure terminal input is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !terminalInput() {
		fmt.Print(title + ": ")
		return readStreamInput(ctx, defaultValue)
	}
	return readWithTerminal(ctx, title, defaultValue, secret)
}

func readWithTerminal(ctx context.Context, title, defaultValue string, secret bool) (string, error) {
	fd := int(os.Stdin.Fd())
	state, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return "", terminalSetupError(secret, err)
	}
	updated := *state
	updated.Lflag &^= unix.ICANON
	if secret {
		updated.Lflag &^= unix.ECHO
	}
	updated.Cc[unix.VMIN], updated.Cc[unix.VTIME] = 1, 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &updated); err != nil {
		return "", terminalSetupError(secret, err)
	}
	fmt.Print(title + ": ")
	return readTerminalInput(ctx, defaultValue, state, secret)
}

func terminalSetupError(secret bool, err error) error {
	if secret {
		return fmt.Errorf("secure terminal input is unavailable")
	}
	return err
}

func readStreamInput(ctx context.Context, defaultValue string) (string, error) {
	var input []byte
	for {
		character, err := readTerminalByte(ctx)
		if err != nil && err != io.EOF {
			return "", err
		}
		if err == io.EOF {
			if len(input) == 0 {
				return "", io.EOF
			}
			break
		}
		done, err := appendTerminalInput(&input, character, 0x7f)
		if err != nil {
			return "", err
		}
		if done {
			break
		}
	}
	if len(input) == 0 {
		return defaultValue, nil
	}
	return string(input), nil
}

func readTerminalInput(ctx context.Context, defaultValue string, original *unix.Termios, secret bool) (value string, resultErr error) {
	defer func() {
		fd := int(os.Stdin.Fd())
		if err := unix.IoctlSetTermios(fd, unix.TCSETS, original); err != nil && resultErr == nil {
			value, resultErr = "", err
		}
		if secret {
			fmt.Println()
		}
	}()
	var input []byte
	for {
		character, err := readTerminalByte(ctx)
		if err != nil {
			return "", err
		}
		done, err := appendTerminalInput(&input, character, original.Cc[unix.VERASE])
		if err != nil {
			return "", err
		}
		if done {
			break
		}
	}
	if len(input) == 0 {
		return defaultValue, nil
	}
	return string(input), nil
}

func appendTerminalInput(input *[]byte, character, erase byte) (bool, error) {
	if character == '\n' {
		return true, nil
	}
	if character == '\r' {
		return false, nil
	}
	if character == erase || character == 8 || character == 127 {
		if len(*input) > 0 {
			_, size := utf8.DecodeLastRune(*input)
			*input = (*input)[:len(*input)-size]
		}
		return false, nil
	}
	if len(*input) >= maxPluginInputBytes {
		return false, fmt.Errorf("input exceeds %d bytes", maxPluginInputBytes)
	}
	*input = append(*input, character)
	return false, nil
}

func readTerminalByte(ctx context.Context) (byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		fds := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
		if _, err := unix.Poll(fds, 100); err != nil {
			return 0, err
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return 0, io.EOF
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			continue
		}
		buffer := []byte{0}
		n, err := os.Stdin.Read(buffer)
		if err == nil && n == 0 {
			return 0, io.EOF
		}
		return buffer[0], err
	}
}

func (consoleInteraction) RunHostCommand(ctx context.Context, command string, args []string) error {
	wasScreenActive := screenActive
	stopScreen()
	if wasScreenActive {
		defer startScreen()
	}
	process := exec.CommandContext(ctx, command, args...)
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	return process.Run()
}

var _ plugins.InputInteraction = consoleInteraction{}
var _ plugins.HostCommandInteraction = consoleInteraction{}
