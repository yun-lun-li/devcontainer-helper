package console

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPluginSecretInputFailsClosedWithoutTerminal(t *testing.T) {
	previous := os.Stdin
	file, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = file
	defer func() { os.Stdin = previous; _ = file.Close() }()
	if _, err := (consoleInteraction{}).Input(context.Background(), "Token", "", true); err == nil || !strings.Contains(err.Error(), "secure terminal input") {
		t.Fatalf("Input() error = %v, want secure terminal error", err)
	}
}

func TestPluginSecretInputDisablesEchoAndRestoresTerminal(t *testing.T) {
	master, slave := openPluginPTY(t)
	previous := os.Stdin
	os.Stdin = slave
	defer func() { os.Stdin = previous; _ = slave.Close() }()
	original, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		value string
		err   error
	}, 1)
	go func() {
		value, err := (consoleInteraction{}).Input(context.Background(), "Token", "", true)
		result <- struct {
			value string
			err   error
		}{value, err}
	}()
	waitForEcho(t, int(slave.Fd()), false)
	if _, err := master.Write([]byte("secret\n")); err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.err != nil || got.value != "secret" {
		t.Fatalf("Input() = %q, %v; want secret", got.value, got.err)
	}
	actual, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil || actual.Lflag != original.Lflag {
		t.Fatalf("terminal flags = %#x, %v; want %#x", actual.Lflag, err, original.Lflag)
	}
}

func TestPluginSecretInputDisablesEchoBeforeWritingPrompt(t *testing.T) {
	master, slave := usePluginPTY(t)
	defer master.Close()
	reader, writer := filledOutputPipe(t)
	defer reader.Close()
	previousOutput := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previousOutput; _ = writer.Close() }()
	previousScreenState := screenActive
	screenActive = false
	defer func() { screenActive = previousScreenState }()
	result := startPluginInput(context.Background(), true)
	time.Sleep(20 * time.Millisecond)
	echoDuringPrompt := terminalFlags(t, slave)&unix.ECHO != 0
	go func() { _, _ = io.Copy(io.Discard, reader) }()
	waitForEcho(t, int(slave.Fd()), false)
	if _, err := master.Write([]byte("secret\n")); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.err != nil || got.value != "secret" {
		t.Fatalf("Input() = %q, %v; want secret", got.value, got.err)
	}
	if echoDuringPrompt {
		t.Fatal("terminal echo remained enabled while the secret prompt was being written")
	}
}

func filledOutputPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	fds := []int{0, 0}
	if err := unix.Pipe2(fds, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(fds[0]), "output-reader")
	writer := os.NewFile(uintptr(fds[1]), "output-writer")
	if reader == nil || writer == nil {
		_ = unix.Close(fds[0])
		_ = unix.Close(fds[1])
		t.Fatal("could not open output pipe")
	}
	if err := unix.SetNonblock(int(writer.Fd()), true); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatal(err)
	}
	fillPipe(t, int(writer.Fd()))
	if err := unix.SetNonblock(int(writer.Fd()), false); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatal(err)
	}
	return reader, writer
}

func fillPipe(t *testing.T, fd int) {
	t.Helper()
	buffer := strings.Repeat("x", 4096)
	for {
		_, err := unix.Write(fd, []byte(buffer))
		if errors.Is(err, unix.EAGAIN) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPluginSecretInputRestoresTerminalAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	master, slave := usePluginPTY(t)
	defer master.Close()
	original := terminalFlags(t, slave)
	result := startPluginInput(ctx, true)
	waitForEcho(t, int(slave.Fd()), false)
	cancel()
	if got := <-result; got.err != context.Canceled {
		t.Fatalf("Input() error = %v; want context cancellation", got.err)
	}
	assertTerminalFlags(t, slave, original)
}

func TestPluginInputCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	master, _ := usePluginPTY(t)
	defer master.Close()
	result := startPluginInput(ctx, false)
	cancel()
	if got := <-result; got.err != context.Canceled {
		t.Fatalf("Input() error = %v; want context cancellation", got.err)
	}
}

func TestPluginSecretInputRestoresTerminalAfterHangup(t *testing.T) {
	master, slave := usePluginPTY(t)
	result := startPluginInput(context.Background(), true)
	waitForEcho(t, int(slave.Fd()), false)
	_ = master.Close()
	got := <-result
	if got.err == nil {
		t.Fatal("Input() error = nil; want terminal hangup")
	}
}

func TestPluginInputRejectsOversizedLine(t *testing.T) {
	master, slave := usePluginPTY(t)
	defer master.Close()
	result := startPluginInput(context.Background(), true)
	waitForEcho(t, int(slave.Fd()), false)
	if _, err := master.Write(append([]byte(strings.Repeat("x", 4097)), '\n')); err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.err == nil || !strings.Contains(got.err.Error(), "exceeds") {
		t.Fatalf("Input() error = %v; want input size error", got.err)
	}
}

func TestPluginInputAppliesTerminalErase(t *testing.T) {
	master, slave := usePluginPTY(t)
	defer master.Close()
	result := startPluginInput(context.Background(), true)
	waitForEcho(t, int(slave.Fd()), false)
	if _, err := master.Write([]byte("ab\x7fc\n")); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.err != nil || got.value != "ac" {
		t.Fatalf("Input() = %q, %v; want erased value ac", got.value, got.err)
	}
}

func TestPluginInteractionRestoresActiveScreen(t *testing.T) {
	master, slave := usePluginPTY(t)
	previousOutput := os.Stdout
	os.Stdout = slave
	defer func() { os.Stdout = previousOutput }()
	previousScreenState := screenActive
	screenActive = true
	t.Cleanup(func() { screenActive = previousScreenState })
	result := startPluginInput(context.Background(), false)
	if _, err := master.Write([]byte("value\n")); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.err != nil || got.value != "value" {
		t.Fatalf("Input() = %q, %v; want value", got.value, got.err)
	}
	if !screenActive {
		t.Fatal("Input() did not restore active screen")
	}
	if err := (consoleInteraction{}).RunHostCommand(context.Background(), "true", nil); err != nil {
		t.Fatal(err)
	}
	if !screenActive {
		t.Fatal("RunHostCommand() did not restore active screen")
	}
}

func TestPluginHostCommandCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- (consoleInteraction{}).RunHostCommand(ctx, "sleep", []string{"30"})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-result; err == nil {
		t.Fatal("RunHostCommand() error = nil; want cancellation")
	}
}

func usePluginPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, slave := openPluginPTY(t)
	previous := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = previous; _ = slave.Close() })
	return master, slave
}

func startPluginInput(ctx context.Context, secret bool) <-chan struct {
	value string
	err   error
} {
	result := make(chan struct {
		value string
		err   error
	}, 1)
	go func() {
		value, err := (consoleInteraction{}).Input(ctx, "Value", "", secret)
		result <- struct {
			value string
			err   error
		}{value, err}
	}()
	return result
}

func terminalFlags(t *testing.T, terminal *os.File) uint32 {
	t.Helper()
	state, err := unix.IoctlGetTermios(int(terminal.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	return state.Lflag
}

func assertTerminalFlags(t *testing.T, terminal *os.File, expected uint32) {
	t.Helper()
	if actual := terminalFlags(t, terminal); actual != expected {
		t.Fatalf("terminal flags = %#x; want %#x", actual, expected)
	}
}

func openPluginPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	var unlock int32
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, int(unlock)); err != nil {
		_ = unix.Close(masterFD)
		t.Fatal(err)
	}
	number, err := unix.IoctlGetUint32(masterFD, unix.TIOCGPTN)
	if err != nil {
		_ = unix.Close(masterFD)
		t.Fatal(err)
	}
	slaveFD, err := unix.Open("/dev/pts/"+strconv.Itoa(int(number)), unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = unix.Close(masterFD)
		t.Fatal(err)
	}
	return os.NewFile(uintptr(masterFD), "pty-master"), os.NewFile(uintptr(slaveFD), "pty-slave")
}

func waitForEcho(t *testing.T, fd int, enabled bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, err := unix.IoctlGetTermios(fd, unix.TCGETS)
		if err == nil && (state.Lflag&unix.ECHO != 0) == enabled {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("terminal echo did not become %v", enabled)
}

func TestPluginInputReturnsDefaultOnEmptyLine(t *testing.T) {
	previous := os.Stdin
	file, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	os.Stdin = file
	defer func() { os.Stdin = previous; _ = file.Close() }()
	value, err := (consoleInteraction{}).Input(context.Background(), "Client ID", "default", false)
	if err != nil || value != "default" {
		t.Fatalf("Input() = %q, %v; want default", value, err)
	}
}
