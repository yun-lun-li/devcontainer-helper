package common

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
	"unsafe"
)

// RunHostCommand gives a command its own process group, transfers terminal input
// when present, and kills the whole group on context cancellation.
// cmd must have been created with exec.CommandContext.
func RunHostCommand(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	restore := foregroundTerminal(cmd)
	defer restore()
	return cmd.Run()
}

func foregroundTerminal(cmd *exec.Cmd) func() {
	input, ok := cmd.Stdin.(*os.File)
	if !ok {
		return func() {}
	}
	var group int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, input.Fd(), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&group)))
	if errno != 0 {
		return func() {}
	}
	cmd.SysProcAttr.Foreground = true
	cmd.SysProcAttr.Ctty = int(input.Fd())
	return func() {
		// The parent is temporarily a background group when reclaiming its terminal.
		signal.Ignore(syscall.SIGTTOU)
		defer signal.Reset(syscall.SIGTTOU)
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, input.Fd(), syscall.TIOCSPGRP, uintptr(unsafe.Pointer(&group)))
	}
}
