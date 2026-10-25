package plugins

import (
	"bytes"
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
	if err := validateEnabledPlugins(old); err != nil {
		return err
	}
	_, wasEnabled := old.Enabled[githubAppID]
	wasActive, _ := m.serviceActive(ctx, path)
	fragment, runtimeDir, err := renderFragment(path)
	if err != nil {
		return err
	}
	fragmentPath, oldBytes, oldFileExists, err := replaceManagedFragment(path, fragment, old.States[githubAppID].FragmentDigest)
	if err != nil {
		return err
	}
	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
		return errors.Join(err, restoreFile(fragmentPath, oldBytes, oldFileExists))
	}
	unit, err := m.prepareServiceUnit(path, old.States[githubAppID])
	if err != nil {
		return errors.Join(err, restoreFile(fragmentPath, oldBytes, oldFileExists))
	}

	if err := saveEnabledPlugin(config, path, old, cfg, fragment, unit); err != nil {
		cleanupErr := restoreFile(fragmentPath, oldBytes, oldFileExists)
		cleanupErr = errors.Join(cleanupErr, unit.restore())
		return errors.Join(err, cleanupErr)
	}

	rollback := enableRollback{manager: m, path: path, old: old, hadOld: hadOld, wasEnabled: wasEnabled, wasActive: wasActive, fragmentPath: fragmentPath, oldBytes: oldBytes, oldFileExists: oldFileExists, runtimeDir: runtimeDir, unit: unit}

	return m.activateEnabledPlugin(ctx, path, runtimeDir, wasActive && wasEnabled, rollback)
}

func (m *Manager) activateEnabledPlugin(ctx context.Context, path, runtimeDir string, restart bool, rollback enableRollback) error {
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return rollback.run(err, false)
	}
	if restart {
		if err := runSystemctl(ctx, "restart", unitName(path)); err != nil {
			return rollback.run(err, true)
		}
	} else if err := runSystemctl(ctx, "enable", "--now", unitName(path)); err != nil {
		return rollback.run(err, true)
	}
	if err := m.checkReady(ctx, runtimeDir); err != nil {
		return rollback.run(err, true)
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
	if err := validateEnabledPlugins(project); err != nil {
		return err
	}
	_, enabled := project.Enabled[githubAppID]
	if !enabled {
		return nil
	}
	if err := validateManagedFragment(path, project.States[githubAppID].FragmentDigest); err != nil {
		return err
	}
	runtimeDir, unit, err := m.prepareEnsureService(path, project)
	if err != nil {
		return err
	}
	oldState := project.States[githubAppID]
	state := oldState
	state.UnitDigest = unit.digest
	state.UnitPath = unit.path
	metadataChanged := state != oldState
	if metadataChanged {
		project.States[githubAppID] = state
		config.ProjectPlugins[path] = project
		if err := common.SaveConfig(config); err != nil {
			return errors.Join(err, unit.restore())
		}
	}
	active, _ := m.serviceActive(ctx, path)
	rollback := ensureRollback{unit: unit, metadataChanged: metadataChanged, project: project, config: config, path: path, oldState: oldState}
	if err := activateEnsuredPlugin(ctx, path, active, unit, rollback); err != nil {
		return err
	}
	return m.checkReady(ctx, runtimeDir)
}

func activateEnsuredPlugin(ctx context.Context, path string, active bool, unit serviceUnitChange, rollback ensureRollback) error {
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return rollback.run(err)
	}
	if active && unit.changed {
		if err := runSystemctl(ctx, "restart", unitName(path)); err != nil {
			return rollback.run(err)
		}
	} else if err := runSystemctl(ctx, "enable", "--now", unitName(path)); err != nil {
		return rollback.run(err)
	}
	return nil
}

func validateManagedFragment(path, expectedDigest string) error {
	_, exists, err := readOwnedFile(managedFragment(path), expectedDigest)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("managed plugin fragment is missing")
	}
	return nil
}

func (m *Manager) prepareEnsureService(path string, project common.ProjectPlugins) (string, serviceUnitChange, error) {
	_, runtimeDir, err := renderFragment(path)
	if err != nil {
		return "", serviceUnitChange{}, err
	}
	if err := githubapp.WriteAdapterScripts(runtimeDir); err != nil {
		return "", serviceUnitChange{}, err
	}
	unit, err := m.prepareServiceUnit(path, project.States[githubAppID])
	return runtimeDir, unit, err
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
	if err := validateEnabledPlugins(project); err != nil {
		return Status{}, err
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
	if err := validateEnabledPlugins(project); err != nil {
		return err
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
	if err := validateEnabledPlugins(project); err != nil {
		return err
	}
	if _, enabled := project.Enabled[githubAppID]; !enabled {
		return nil
	}
	unitFile, err := ownedUnitPath(path, project.States[githubAppID])
	if err != nil {
		return err
	}
	if _, _, err := readOwnedFile(managedFragment(path), project.States[githubAppID].FragmentDigest); err != nil {
		return err
	}
	if err := removePluginFiles(ctx, path, unitFile, project.States[githubAppID].FragmentDigest); err != nil {
		return err
	}
	delete(project.Enabled, githubAppID)
	state := project.States[githubAppID]
	state.FragmentDigest = ""
	state.UnitDigest = ""
	state.UnitPath = ""
	state.BuildFingerprint = ""
	state.BuildPending = true
	project.States[githubAppID] = state
	config.ProjectPlugins[path] = project
	return common.SaveConfig(config)
}

func ownedUnitPath(path string, state common.PluginState) (string, error) {
	unitFile := state.UnitPath
	if unitFile == "" {
		var err error
		unitFile, err = unitPath(path)
		if err != nil {
			return "", err
		}
	} else if err := validateUnitPath(unitFile, path); err != nil {
		return "", err
	}
	if err := verifyOwnedUnit(unitFile, state.UnitDigest); err != nil {
		return "", err
	}
	return unitFile, nil
}

func removePluginFiles(ctx context.Context, path, unitFile, fragmentDigest string) error {
	if err := runSystemctl(ctx, "stop", unitName(path)); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "disable", unitName(path)); err != nil {
		return err
	}
	if err := removeIfPresent(unitFile); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := removeManagedFragment(path, fragmentDigest); err != nil {
		return err
	}
	runtimeDir, err := githubapp.RuntimeDir(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(runtimeDir)
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func removeManagedFragment(path, expectedDigest string) error {
	fragmentPath := managedFragment(path)
	_, exists, err := readOwnedFile(fragmentPath, expectedDigest)
	if err != nil || !exists {
		return err
	}
	return os.Remove(fragmentPath)
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
	if err := validateEnabledPlugins(project); err != nil {
		return err
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
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "enable", "--now", unitName(path)); err != nil {
		return err
	}
	return m.checkReady(ctx, runtimeDir)
}

type serviceUnitChange struct {
	path    string
	digest  string
	changed bool
	before  []byte
	existed bool
}

func (change serviceUnitChange) restore() error {
	if change.existed {
		return writePrivateFile(change.path, change.before)
	}
	if err := os.Remove(change.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Manager) prepareServiceUnit(projectPath string, previous common.PluginState) (serviceUnitChange, error) {
	var change serviceUnitChange
	unit, err := unitPath(projectPath)
	if previous.UnitPath != "" {
		unit = previous.UnitPath
		if err := validateUnitPath(unit, projectPath); err != nil {
			return change, err
		}
	} else if err != nil {
		return change, err
	}
	content, err := m.serviceUnitContent(projectPath)
	if err != nil {
		return change, err
	}
	change = serviceUnitChange{path: unit, digest: digest(content)}
	return prepareUnitFile(change, previous.UnitDigest, content)
}

func prepareUnitFile(change serviceUnitChange, previousDigest string, content []byte) (serviceUnitChange, error) {
	info, err := os.Lstat(change.path)
	if errors.Is(err, os.ErrNotExist) {
		return createUnitFile(change, content)
	}
	if err != nil {
		return change, err
	}
	return replaceUnitFile(change, previousDigest, content, info)
}

func createUnitFile(change serviceUnitChange, content []byte) (serviceUnitChange, error) {
	if err := os.MkdirAll(filepath.Dir(change.path), 0700); err != nil {
		return change, err
	}
	if err := writePrivateFile(change.path, content); err != nil {
		return change, err
	}
	change.changed = true
	return change, nil
}

func replaceUnitFile(change serviceUnitChange, previousDigest string, content []byte, info os.FileInfo) (serviceUnitChange, error) {
	if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
		return change, fmt.Errorf("systemd unit is unsafe")
	}
	existing, err := os.ReadFile(change.path)
	if err != nil {
		return change, err
	}
	if previousDigest == "" || digest(existing) != previousDigest {
		return change, fmt.Errorf("systemd unit was modified; refusing to replace it")
	}
	change.before = existing
	change.existed = true
	change.changed = !bytes.Equal(existing, content)
	if change.changed {
		if err := writePrivateFile(change.path, content); err != nil {
			return change, err
		}
	}
	return change, nil
}

func validateUnitPath(path, projectPath string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != unitName(projectPath) || filepath.Base(filepath.Dir(path)) != "user" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "systemd" {
		return fmt.Errorf("stored systemd unit path is invalid")
	}
	return nil
}

func (m *Manager) serviceUnitContent(projectPath string) ([]byte, error) {
	configPath, err := common.ConfigPath()
	if err != nil {
		return nil, err
	}
	bin, err := os.Executable()
	if err != nil {
		return nil, err
	}
	content := "[Unit]\nDescription=dch GitHub App credential service\n\n[Service]\nType=simple\nExecStart=" + systemdExecArg(bin) + " _github-app-serve --project " + systemdExecArg(projectPath) + "\nRestart=on-failure\n"
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
	return []byte(content), nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

func runSystemctl(ctx context.Context, args ...string) error {
	args = append([]string{"--user"}, args...)
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "systemctl", args...)
	var output limitedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s failed: %s", args[1], strings.TrimSpace(output.String()))
	}
	return nil
}

type limitedOutput struct{ buffer bytes.Buffer }

func (output *limitedOutput) Write(data []byte) (int, error) {
	const limit = 4096
	originalLength := len(data)
	remaining := limit - output.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = output.buffer.Write(data)
	}
	return originalLength, nil
}
func (output *limitedOutput) String() string { return output.buffer.String() }

func systemdArg(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "%", "%%").Replace(s) + "\""
}
func systemdExecArg(s string) string      { return systemdArg(strings.ReplaceAll(s, "$", "$$")) }
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
