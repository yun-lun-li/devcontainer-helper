// Package hostpass exposes one host password-store PAT through a private Unix socket.
package hostpass

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"dch/internal/common"
)

const requestTimeout = 120 * time.Second

var entryPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]*$`)
var tokenPattern = regexp.MustCompile(`^(github_pat_|ghp_)[A-Za-z0-9_]+$`)

// ValidateEntry checks a pass entry name without reading its secret.
func ValidateEntry(entry string) error { return validateEntry(entry) }

func validateEntry(entry string) error {
	if !entryPattern.MatchString(entry) || filepath.Clean(entry) != entry || strings.Contains(entry, "..") {
		return fmt.Errorf("pass entry must be a relative name without parent traversal")
	}
	return nil
}

func passCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "pass", args...)
	cmd.Env = passEnvironment()
	return cmd
}

func passEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PASSWORD_STORE_GPG_OPTS=") {
			env = append(env, value)
		}
	}
	if os.Getenv("GPG_TTY") == "" {
		if tty, err := os.Readlink("/proc/self/fd/0"); err == nil && (strings.HasPrefix(tty, "/dev/pts/") || strings.HasPrefix(tty, "/dev/tty")) {
			env = append(env, "GPG_TTY="+tty)
		}
	}
	return append(env, "PASSWORD_STORE_GPG_OPTS=--pinentry-mode=ask")
}

type tokenBuffer struct{ bytes.Buffer }

func (buffer *tokenBuffer) Write(data []byte) (int, error) {
	if buffer.Len()+len(data) > 16<<10 {
		return 0, fmt.Errorf("pass output too large")
	}
	return buffer.Buffer.Write(data)
}

func readToken(ctx context.Context, entry string) (string, error) {
	return readTokenUsing(ctx, entry, "pass", passEnvironment(), "")
}

func readTokenUsing(ctx context.Context, entry, executable string, env []string, directory string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "show", "--", entry)
	cmd.Env = env
	cmd.Dir = directory
	var output tokenBuffer
	cmd.Stdout = &output
	if err := common.RunHostCommand(cmd); err != nil {
		return "", fmt.Errorf("could not read PAT from host pass; unlock GPG on the host and retry")
	}
	token := strings.SplitN(output.String(), "\n", 2)[0]
	if !tokenPattern.MatchString(token) {
		return "", fmt.Errorf("host pass entry must contain a GitHub PAT on its first line")
	}
	return token, nil
}

func ensureToken(ctx context.Context, entry string, stdin io.Reader, stdout, stderr io.Writer) error {
	root, err := passwordStoreDirectory()
	if err != nil {
		return err
	}
	_, err = os.Stat(filepath.Join(root, filepath.FromSlash(entry)+".gpg"))
	if os.IsNotExist(err) {
		fmt.Fprintln(stdout, "Store the GitHub PAT in host pass (input is hidden).")
		cmd := passCommand(ctx, "insert", "--", entry)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
		if err := common.RunHostCommand(cmd); err != nil {
			return fmt.Errorf("could not store PAT; run pass insert on the host and retry")
		}
	} else if err != nil {
		return fmt.Errorf("inspect pass entry: %w", err)
	}
	_, err = readToken(ctx, entry)
	return err
}
