package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"dch/internal/common"
	"dch/internal/githubapp"
)

const githubAppID = "github-app"

type Status struct {
	Enabled       bool
	PendingBuild  bool
	ServiceActive bool
	Message       string
}

type Manager struct{}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Enable(ctx context.Context, projectPath string, cfg githubapp.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	unitFile, err := unitPath(path)
	if err != nil {
		return err
	}
	unlock, err := lockConfig()
	if err != nil {
		return err
	}
	defer unlock()

	config, err := common.LoadConfig()
	if err != nil {
		return err
	}
	old, hadOld := config.ProjectPlugins[path]
	for id := range old.Enabled {
		if id != githubAppID {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	_, wasEnabled := old.Enabled[githubAppID]
	wasActive, _ := m.serviceActive(ctx, path)
	fragment, runtimeDir, err := renderFragment(path)
	if err != nil {
		return err
	}
	fragmentPath := managedFragment(path)
	oldBytes, oldFileExists, err := readOwnedFile(fragmentPath, old.States[githubAppID].FragmentDigest)
	if err != nil {
		return err
	}
	if !oldFileExists {
		if _, err := os.Lstat(fragmentPath); err == nil {
			return fmt.Errorf("plugin fragment already exists and is not managed by dch")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		return err
	}
	if err := writePrivateFile(fragmentPath, fragment); err != nil {
		return err
	}
	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
		return errors.Join(err, restoreFile(fragmentPath, oldBytes, oldFileExists))
	}
	unitCreated, err := m.writeService(path)
	if err != nil {
		return errors.Join(err, restoreFile(fragmentPath, oldBytes, oldFileExists))
	}

	newDigest := digest(fragment)
	if config.ProjectPlugins == nil {
		config.ProjectPlugins = map[string]common.ProjectPlugins{}
	}
	state := old.States[githubAppID]
	state.FragmentDigest = newDigest
	state.BuildFingerprint = buildFingerprint(cfg, newDigest)
	state.BuildPending = true
	project := cloneProjectPlugins(old)
	if project.Enabled == nil {
		project.Enabled = map[string]githubapp.Config{}
	}
	if project.States == nil {
		project.States = map[string]common.PluginState{}
	}
	project.Enabled[githubAppID] = cfg
	project.States[githubAppID] = state
	config.ProjectPlugins[path] = project
	if err := common.SaveConfig(config); err != nil {
		cleanupErr := restoreFile(fragmentPath, oldBytes, oldFileExists)
		if unitCreated {
			if removeErr := os.Remove(unitFile); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, removeErr)
			}
		}
		return errors.Join(err, cleanupErr)
	}

	serviceTouched := false
	rollback := func(cause error) error {
		var cleanup []error
		if serviceTouched {
			for _, action := range [][]string{{"stop", unitName(path)}, {"disable", unitName(path)}, {"daemon-reload"}} {
				if err := runSystemctl(context.Background(), action...); err != nil {
					cleanup = append(cleanup, err)
				}
			}
		}
		if unitCreated && len(cleanup) == 0 {
			if err := os.Remove(unitFile); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanup = append(cleanup, err)
			}
		}
		if err := os.Remove(filepath.Join(runtimeDir, "credential.sock")); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanup = append(cleanup, err)
		}
		if len(cleanup) > 0 {
			return fmt.Errorf("%w; cleanup failed, settings remain enabled for retry: %w", cause, errors.Join(cleanup...))
		}
		if err := restoreFile(fragmentPath, oldBytes, oldFileExists); err != nil {
			cleanup = append(cleanup, err)
		}
		current, loadErr := common.LoadConfig()
		if loadErr != nil {
			cleanup = append(cleanup, loadErr)
		} else {
			if hadOld {
				current.ProjectPlugins[path] = old
			} else {
				delete(current.ProjectPlugins, path)
			}
			if err := common.SaveConfig(current); err != nil {
				cleanup = append(cleanup, err)
			}
		}
		if wasActive && wasEnabled {
			if err := m.startAndCheck(context.Background(), path); err != nil {
				cleanup = append(cleanup, err)
			}
		}
		if len(cleanup) > 0 {
			return fmt.Errorf("%w; rollback incomplete: %w", cause, errors.Join(cleanup...))
		}
		return cause
	}

	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return rollback(err)
	}
	serviceTouched = true
	if wasActive && wasEnabled {
		if err := runSystemctl(ctx, "restart", unitName(path)); err != nil {
			return rollback(err)
		}
	} else if err := runSystemctl(ctx, "enable", "--now", unitName(path)); err != nil {
		return rollback(err)
	}
	if err := m.checkReady(ctx, runtimeDir); err != nil {
		return rollback(err)
	}
	return nil
}

func (m *Manager) Ensure(ctx context.Context, projectPath string) error {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	unlock, err := lockConfig()
	if err != nil {
		return err
	}
	defer unlock()
	config, err := common.LoadConfig()
	if err != nil {
		return err
	}
	project, ok := config.ProjectPlugins[path]
	if !ok {
		return nil
	}
	for id := range project.Enabled {
		if id != githubAppID {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	_, enabled := project.Enabled[githubAppID]
	if !enabled {
		return nil
	}
	if _, exists, err := readOwnedFile(managedFragment(path), project.States[githubAppID].FragmentDigest); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("managed plugin fragment is missing")
	}
	_, runtimeDir, err := renderFragment(path)
	if err != nil {
		return err
	}
	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
		return err
	}
	if _, err := m.writeService(path); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "enable", "--now", unitName(path)); err != nil {
		return err
	}
	return m.checkReady(ctx, runtimeDir)
}

func (m *Manager) Status(ctx context.Context, projectPath string) (Status, error) {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return Status{}, err
	}
	unlock, err := lockConfig()
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	config, err := common.LoadConfig()
	if err != nil {
		return Status{}, err
	}
	project, ok := config.ProjectPlugins[path]
	if !ok {
		return Status{}, nil
	}
	for id := range project.Enabled {
		if id != githubAppID {
			return Status{}, fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	_, enabled := project.Enabled[githubAppID]
	active, activeErr := m.serviceActive(ctx, path)
	status := Status{Enabled: enabled, PendingBuild: project.States[githubAppID].BuildPending, ServiceActive: active}
	if activeErr != nil {
		status.Message = activeErr.Error()
	}
	return status, nil
}

func (m *Manager) MarkBuilt(projectPath string) error {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	unlock, err := lockConfig()
	if err != nil {
		return err
	}
	defer unlock()
	config, err := common.LoadConfig()
	if err != nil {
		return err
	}
	project, ok := config.ProjectPlugins[path]
	if !ok {
		return nil
	}
	for id := range project.Enabled {
		if id != githubAppID {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	state := project.States[githubAppID]
	state.BuildPending = false
	project.States[githubAppID] = state
	config.ProjectPlugins[path] = project
	return common.SaveConfig(config)
}

func (m *Manager) Disable(ctx context.Context, projectPath string) error {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	unlock, err := lockConfig()
	if err != nil {
		return err
	}
	defer unlock()
	config, err := common.LoadConfig()
	if err != nil {
		return err
	}
	project, ok := config.ProjectPlugins[path]
	if !ok {
		return nil
	}
	for id := range project.Enabled {
		if id != githubAppID {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	if _, enabled := project.Enabled[githubAppID]; !enabled {
		return nil
	}
	unitFile, err := unitPath(path)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(unitFile); err == nil {
		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
			return fmt.Errorf("systemd unit is unsafe")
		}
		if created, err := m.writeService(path); err != nil || created {
			if err != nil {
				return err
			}
			return fmt.Errorf("systemd unit ownership changed during cleanup")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, _, err := readOwnedFile(managedFragment(path), project.States[githubAppID].FragmentDigest); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "stop", unitName(path)); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "disable", unitName(path)); err != nil {
		return err
	}
	if err := os.Remove(unitFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	fragmentPath := managedFragment(path)
	if _, exists, err := readOwnedFile(fragmentPath, project.States[githubAppID].FragmentDigest); err != nil {
		return err
	} else if exists {
		if err := os.Remove(fragmentPath); err != nil {
			return err
		}
	}
	runtimeDir, err := githubapp.RuntimeDir(path)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(runtimeDir); err != nil {
		return err
	}
	delete(project.Enabled, githubAppID)
	state := project.States[githubAppID]
	state.FragmentDigest = ""
	state.BuildFingerprint = ""
	state.BuildPending = true
	project.States[githubAppID] = state
	config.ProjectPlugins[path] = project
	return common.SaveConfig(config)
}

func (m *Manager) ServeProject(ctx context.Context, projectPath string) error {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	config, err := common.LoadConfig()
	if err != nil {
		return err
	}
	project, ok := config.ProjectPlugins[path]
	if !ok {
		return fmt.Errorf("github-app is not enabled for this project")
	}
	for id := range project.Enabled {
		if id != githubAppID {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	cfg, ok := project.Enabled[githubAppID]
	if !ok {
		return fmt.Errorf("github-app is not enabled for this project")
	}
	runtimeDir, err := githubapp.RuntimeDir(path)
	if err != nil {
		return err
	}
	return githubapp.Serve(ctx, filepath.Join(runtimeDir, "credential.sock"), cfg)
}

func renderFragment(path string) ([]byte, string, error) {
	runtimeDir, err := githubapp.RuntimeDir(path)
	if err != nil {
		return nil, "", err
	}
	fragment, err := githubapp.AdapterFragment(runtimeDir)
	return fragment, runtimeDir, err
}

func (m *Manager) checkReady(ctx context.Context, runtimeDir string) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	socketPath := filepath.Join(runtimeDir, "credential.sock")
	for {
		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("credential service socket did not become ready")
		case <-tick.C:
		}
	}
	if err := githubapp.CheckCredential(ctx, socketPath); err != nil {
		return fmt.Errorf("credential service validation failed: %w", err)
	}
	return nil
}

func (m *Manager) serviceActive(ctx context.Context, path string) (bool, error) {
	err := runSystemctl(ctx, "is-active", "--quiet", unitName(path))
	if err == nil {
		return true, nil
	}
	return false, err
}

func (m *Manager) startAndCheck(ctx context.Context, path string) error {
	_, runtimeDir, err := renderFragment(path)
	if err != nil {
		return err
	}
	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
		return err
	}
	if _, err := m.writeService(path); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "enable", "--now", unitName(path)); err != nil {
		return err
	}
	return m.checkReady(ctx, runtimeDir)
}

func (m *Manager) writeService(projectPath string) (bool, error) {
	configPath, err := common.ConfigPath()
	if err != nil {
		return false, err
	}
	bin, err := os.Executable()
	if err != nil {
		return false, err
	}
	unit, err := unitPath(projectPath)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
		return false, err
	}
	content := "[Unit]\nDescription=dch GitHub App credential service\n\n[Service]\nType=simple\nExecStart=" + systemdArg(bin) + " _github-app-serve --project " + systemdArg(projectPath) + "\nRestart=on-failure\n"
	for _, key := range []string{"DCH_CONFIG", "XDG_RUNTIME_DIR", "PATH", "GNUPGHOME", "PASSWORD_STORE_DIR"} {
		value := os.Getenv(key)
		if key == "DCH_CONFIG" {
			value = configPath
		}
		if value != "" {
			content += "Environment=" + systemdEnv(key, value) + "\n"
		}
	}
	content += "\n[Install]\nWantedBy=default.target\n"
	if info, err := os.Lstat(unit); err == nil {
		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
			return false, fmt.Errorf("systemd unit is unsafe")
		}
		existing, err := os.ReadFile(unit)
		if err != nil {
			return false, err
		}
		if string(existing) != content {
			return false, fmt.Errorf("systemd unit exists and is not managed by dch")
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := writePrivateFile(unit, []byte(content)); err != nil {
		return false, err
	}
	return true, nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

func runSystemctl(ctx context.Context, args ...string) error {
	args = append([]string{"--user"}, args...)
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s failed: %s", args[1], strings.TrimSpace(string(output)))
	}
	return nil
}

func systemdArg(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "%", "%%").Replace(s) + "\""
}
func systemdEnv(key, value string) string { return systemdArg(key + "=" + value) }

func unitName(path string) string { return "dch-github-app-" + digest([]byte(path))[:16] + ".service" }
func unitPath(path string) (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(config) {
		return "", fmt.Errorf("user config directory must be absolute")
	}
	return filepath.Join(config, "systemd", "user", unitName(path)), nil
}
func managedFragment(path string) string {
	return filepath.Join(path, ".devcontainer", "devcontainer.local.d", "90-dch-github-app.json")
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func buildFingerprint(config githubapp.Config, fragmentDigest string) string {
	data, _ := json.Marshal(struct {
		Config         githubapp.Config `json:"config"`
		FragmentDigest string           `json:"fragment_digest"`
	}{config, fragmentDigest})
	return digest(data)
}

func cloneProjectPlugins(project common.ProjectPlugins) common.ProjectPlugins {
	clone := common.ProjectPlugins{Enabled: make(map[string]githubapp.Config, len(project.Enabled)), States: make(map[string]common.PluginState, len(project.States))}
	for id, cfg := range project.Enabled {
		permissions := make(map[string]string, len(cfg.Permissions))
		for key, value := range cfg.Permissions {
			permissions[key] = value
		}
		cfg.Permissions = permissions
		clone.Enabled[id] = cfg
	}
	for id, state := range project.States {
		clone.States[id] = state
	}
	return clone
}

func readOwnedFile(path, expected string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("plugin fragment is not a regular file")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if expected == "" || digest(data) != expected {
		return nil, false, fmt.Errorf("plugin fragment was modified; refusing to overwrite or remove it")
	}
	return data, true, nil
}

func writePrivateFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dch-plugin-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func restoreFile(path string, data []byte, existed bool) error {
	if existed {
		return writePrivateFile(path, data)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func lockConfig() (func(), error) {
	path, err := common.ConfigPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path+".plugins.lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path+".plugins.lock")
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
		_ = file.Close()
		return nil, fmt.Errorf("plugin config lock is unsafe")
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = file.Close() }, nil
}
