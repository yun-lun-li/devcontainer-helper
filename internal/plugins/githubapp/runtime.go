package githubapp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func RuntimeDir(projectPath string) (string, error) {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		return "", fmt.Errorf("XDG_RUNTIME_DIR is required")
	}
	info, err := os.Lstat(runtime)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("XDG_RUNTIME_DIR is not a private directory owned by the current user")
	}
	abs, err := filepath.Abs(projectPath)
	if err != nil || projectPath == "" {
		return "", fmt.Errorf("invalid project path")
	}
	key := sha256.Sum256([]byte(filepath.Clean(abs)))
	dir := filepath.Join(runtime, "dch", hex.EncodeToString(key[:12]))
	parent := filepath.Join(runtime, "dch")
	if err := ensurePrivateDir(parent); err != nil {
		return "", err
	}
	if err := ensurePrivateDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func ensurePrivateDir(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("cannot create runtime directory")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) || info.Mode().Perm() != 0700 {
		return fmt.Errorf("runtime directory is unsafe")
	}
	return nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}
