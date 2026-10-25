package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var ErrSecretMissing = fmt.Errorf("credential entry is missing")

type SecretStore struct {
	PassPath string
	Timeout  time.Duration
	Entry    string
	LockDir  string
	Now      func() time.Time
}

func (s *SecretStore) Read(ctx context.Context, cfg Config) (TokenSet, error) {
	var set TokenSet
	data, err := s.run(ctx, nil, "show", s.Entry)
	if err != nil {
		return set, fmt.Errorf("credential unavailable")
	}
	decoded, decodeErr := decodeTokenSet(data)
	if decodeErr != nil {
		return TokenSet{}, fmt.Errorf("authorization required")
	}
	set = decoded
	if err := validateTokenSet(set, cfg, s.now()); err != nil {
		return TokenSet{}, err
	}
	return set, nil
}

func (s *SecretStore) Snapshot(ctx context.Context) ([]byte, error) {
	data, stderr, err := s.runCaptured(ctx, nil, "show", s.Entry)
	if err == nil {
		return data, nil
	}
	if strings.Contains(string(stderr), "not in the password store") {
		return nil, ErrSecretMissing
	}
	return nil, fmt.Errorf("credential unavailable")
}

func (s *SecretStore) RestoreRaw(ctx context.Context, data []byte) error {
	if len(data) == 0 || len(data) > maxResponseBytes {
		return fmt.Errorf("invalid credential backup")
	}
	if _, err := decodeTokenSet(data); err != nil {
		return fmt.Errorf("invalid credential backup")
	}
	if _, err := s.run(ctx, data, "insert", "--multiline", "--force", s.Entry); err != nil {
		return fmt.Errorf("could not restore credential")
	}
	readback, err := s.run(ctx, nil, "show", s.Entry)
	if err != nil || !bytes.Equal(bytes.TrimSpace(readback), bytes.TrimSpace(data)) {
		return fmt.Errorf("credential restore verification failed")
	}
	return nil
}

func (s *SecretStore) Delete(ctx context.Context) error {
	if _, err := s.Snapshot(ctx); errors.Is(err, ErrSecretMissing) {
		return nil
	} else if err != nil {
		return fmt.Errorf("credential unavailable")
	}
	if _, err := s.run(ctx, nil, "rm", "--force", s.Entry); err != nil {
		if _, snapshotErr := s.Snapshot(ctx); errors.Is(snapshotErr, ErrSecretMissing) {
			return nil
		}
		return fmt.Errorf("could not remove credential")
	}
	if _, err := s.Snapshot(ctx); !errors.Is(err, ErrSecretMissing) {
		return fmt.Errorf("credential removal verification failed")
	}
	return nil
}

func decodeTokenSet(data []byte) (TokenSet, error) {
	var set TokenSet
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&set) != nil || decoder.Decode(new(any)) != io.EOF {
		return TokenSet{}, fmt.Errorf("invalid credential")
	}
	return set, nil
}

func (s *SecretStore) Write(ctx context.Context, set TokenSet) error {
	data, err := json.Marshal(set)
	if err != nil {
		return err
	}
	if err := validateCandidate(set, s.now()); err != nil {
		return err
	}
	_, err = s.run(ctx, data, "insert", "--multiline", "--force", s.Entry)
	if err != nil {
		return fmt.Errorf("could not save credential")
	}
	readback, err := s.run(ctx, nil, "show", s.Entry)
	if err != nil || !bytes.Equal(bytes.TrimSpace(readback), data) {
		return fmt.Errorf("credential save verification failed")
	}
	return nil
}

func (s *SecretStore) WithLock(ctx context.Context, fn func() error) error {
	dir := s.LockDir
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(base, "dch", "github-app-locks")
	}
	if err := ensureLockDirectory(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, pathHash(s.Entry)+".lock")
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	if err := validateLockFile(f); err != nil {
		f.Close()
		return err
	}
	defer f.Close()
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN && err != syscall.EINTR {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func (s *SecretStore) run(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	stdout, _, err := s.runCaptured(ctx, input, args...)
	return stdout, err
}

func (s *SecretStore) runCaptured(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
	path := s.PassPath
	if path == "" {
		path = "pass"
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(callCtx, path, args...)
	cmd.Env = securePassEnvironment(os.Environ())
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil || stdout.overflow || stderr.overflow {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("pass operation failed")
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (s *SecretStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func validateCandidate(set TokenSet, now time.Time) error {
	if set.Schema != 1 || set.ClientID == "" || set.ProjectPath == "" || set.UserID <= 0 || set.RepositoryID <= 0 || !validRepositoryName(set.Repository) || set.AccessToken == "" || set.RefreshToken == "" || strings.ContainsAny(set.AccessToken+set.RefreshToken, "\r\n") || !set.AccessTokenExpiresAt.After(now) || !set.RefreshTokenExpiresAt.After(now) {
		return fmt.Errorf("invalid GitHub credential")
	}
	return nil
}

func validateTokenSet(set TokenSet, cfg Config, now time.Time) error {
	if set.Schema != 1 || set.ClientID != cfg.ClientID || filepath.Clean(set.ProjectPath) != filepath.Clean(cfg.ProjectPath) || set.UserID != cfg.UserID || set.RepositoryID != cfg.RepositoryID || set.Repository != cfg.Repository || set.AccessToken == "" || set.RefreshToken == "" || strings.ContainsAny(set.AccessToken+set.RefreshToken, "\r\n") || set.AccessTokenExpiresAt.IsZero() || !set.RefreshTokenExpiresAt.After(now) {
		return fmt.Errorf("authorization required")
	}
	return nil
}

func ensureLockDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil || abs == string(filepath.Separator) {
		return fmt.Errorf("credential lock directory is unsafe")
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		if err := ensureLockPathComponent(current, current == abs); err != nil {
			return err
		}
	}
	return nil
}

func ensureLockPathComponent(path string, final bool) error {
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("credential lock directory is unsafe")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credential lock directory is unsafe")
	}
	if final {
		return validatePrivateLockDir(info)
	}
	if info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("credential lock parent is unsafe")
	}
	return nil
}

func validatePrivateLockDir(info os.FileInfo) error {
	if info.Mode().Perm() != 0700 || !ownedByCurrentUser(info) {
		return fmt.Errorf("credential lock directory is unsafe")
	}
	return nil
}

func validateLockFile(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ownedByCurrentUser(info) {
		return fmt.Errorf("credential lock file is unsafe")
	}
	return nil
}
