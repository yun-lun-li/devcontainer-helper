package githubapp

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
	"time"

	"dch/internal/common"
	"dch/internal/plugins"
)

type ownershipState struct {
	FragmentDigest string `json:"fragment_digest,omitempty"`
	UnitDigest     string `json:"unit_digest,omitempty"`
	UnitPath       string `json:"unit_path,omitempty"`
}

func decodeState(raw json.RawMessage) (ownershipState, error) {
	var state ownershipState
	if len(raw) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("invalid GitHub App state")
	}
	return state, nil
}

func encodeState(state ownershipState) json.RawMessage {
	raw, _ := json.Marshal(state)
	return raw
}

func (p *Plugin) enable(ctx context.Context, host plugins.HostContext) (plugins.Result, error) {
	resources, err := prepareEnableResources(ctx, host)
	if err != nil {
		return plugins.Result{}, err
	}
	if err := host.Store.SaveConfig(host.Config); err != nil {
		return plugins.Result{}, errors.Join(err, resources.restoreFiles())
	}
	if err := host.Store.SaveState(encodeState(resources.state)); err != nil {
		return plugins.Result{}, p.rollbackEnable(host, resources, err, false)
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return plugins.Result{}, p.rollbackEnable(host, resources, err, false)
	}
	if err := runSystemctl(ctx, resources.activationArgs()...); err != nil {
		return plugins.Result{}, p.rollbackEnable(host, resources, err, true)
	}
	if err := checkReady(ctx, resources.runtimeDir); err != nil {
		return plugins.Result{}, p.rollbackEnable(host, resources, err, true)
	}
	return plugins.Result{NeedsBuild: true, State: encodeState(resources.state)}, nil
}

type enableResources struct {
	projectPath  string
	runtimeDir   string
	fragmentPath string
	oldFragment  []byte
	hadFragment  bool
	unit         serviceUnitChange
	state        ownershipState
	wasActive    bool
	wasEnabled   bool
}

func prepareEnableResources(ctx context.Context, host plugins.HostContext) (enableResources, error) {
	resources := enableResources{projectPath: host.ProjectPath}
	cfg, err := decodeConfig(host.Config)
	if err != nil {
		return resources, err
	}
	if err := cfg.Validate(); err != nil {
		return resources, err
	}
	if err := requireLocalDocker(); err != nil {
		return resources, err
	}
	oldState, err := decodeState(host.State)
	if err != nil {
		return resources, err
	}
	_, resources.wasEnabled = previousConfig(host.PreviousConfig)
	resources.wasActive = runSystemctl(ctx, "is-active", "--quiet", unitName(host.ProjectPath)) == nil
	fragment, runtimeDir, err := renderFragment(host.ProjectPath)
	if err != nil {
		return resources, err
	}
	resources.runtimeDir = runtimeDir
	resources.fragmentPath, resources.oldFragment, resources.hadFragment, err = replaceManagedFragment(host.ProjectPath, fragment, oldState.FragmentDigest)
	if err != nil {
		return resources, err
	}
	if err = WriteAdapterScripts(runtimeDir); err != nil {
		return resources, errors.Join(err, resources.restoreFragment())
	}
	resources.unit, err = prepareServiceUnit(host.ProjectPath, oldState)
	if err != nil {
		return resources, errors.Join(err, resources.restoreFragment())
	}
	resources.state = ownershipState{FragmentDigest: digest(fragment), UnitDigest: resources.unit.digest, UnitPath: resources.unit.path}
	return resources, nil
}

func (resources enableResources) activationArgs() []string {
	if resources.wasActive && resources.wasEnabled {
		return []string{"restart", unitName(resources.projectPath)}
	}
	return []string{"enable", "--now", unitName(resources.projectPath)}
}

func (resources enableResources) restoreFragment() error {
	return restoreFile(resources.fragmentPath, resources.oldFragment, resources.hadFragment)
}

func (resources enableResources) restoreFiles() error {
	return errors.Join(resources.restoreFragment(), resources.unit.restore())
}

func previousConfig(raw json.RawMessage) (Config, bool) {
	if len(raw) == 0 {
		return Config{}, false
	}
	cfg, err := decodeConfig(raw)
	return cfg, err == nil
}

func (p *Plugin) rollbackEnable(host plugins.HostContext, resources enableResources, cause error, serviceTouched bool) error {
	var cleanup []error
	if serviceTouched {
		cleanup = appendError(cleanup, runSystemctl(context.Background(), "stop", unitName(host.ProjectPath)))
		cleanup = appendError(cleanup, runSystemctl(context.Background(), "disable", unitName(host.ProjectPath)))
		cleanup = appendError(cleanup, runSystemctl(context.Background(), "daemon-reload"))
	}
	if serviceTouched {
		cleanup = appendError(cleanup, removeCredentialSocket(resources.runtimeDir))
	}
	if len(cleanup) == 0 {
		cleanup = appendError(cleanup, resources.unit.restore())
	}
	if len(cleanup) == 0 {
		cleanup = appendError(cleanup, resources.restoreFragment())
	}
	if len(cleanup) == 0 {
		cleanup = appendError(cleanup, restoreScoped(host))
	}
	if len(cleanup) == 0 && serviceTouched && resources.wasActive {
		cleanup = appendError(cleanup, p.startAndCheck(context.Background(), host.ProjectPath))
	}
	if len(cleanup) > 0 {
		return fmt.Errorf("%w; cleanup failed, GitHub App settings remain for retry: %w", cause, errors.Join(cleanup...))
	}
	return cause
}

func restoreScoped(host plugins.HostContext) error {
	return errors.Join(host.Store.SaveConfig(host.PreviousConfig), host.Store.SaveState(host.PreviousState))
}

func (p *Plugin) disable(ctx context.Context, host plugins.HostContext) (plugins.Result, error) {
	if len(host.Config) == 0 && len(host.State) == 0 {
		return plugins.Result{}, nil
	}
	state, err := decodeState(host.State)
	if err != nil {
		return plugins.Result{}, err
	}
	unitFile, err := ownedUnitPath(host.ProjectPath, state)
	if err != nil {
		return plugins.Result{}, err
	}
	if _, _, err := readOwnedFile(managedFragment(host.ProjectPath), state.FragmentDigest); err != nil {
		return plugins.Result{}, err
	}
	if err := removePluginFiles(ctx, host.ProjectPath, unitFile, state.FragmentDigest); err != nil {
		return plugins.Result{}, err
	}
	if err := host.Store.SaveState(nil); err != nil {
		return plugins.Result{}, err
	}
	return plugins.Result{NeedsBuild: true}, nil
}

func (p *Plugin) ensure(ctx context.Context, host plugins.HostContext) (plugins.Result, error) {
	cfg, err := decodeConfig(host.Config)
	if err != nil {
		return plugins.Result{}, err
	}
	if err := cfg.Validate(); err != nil {
		return plugins.Result{}, err
	}
	if err := requireLocalDocker(); err != nil {
		return plugins.Result{}, err
	}
	state, err := decodeState(host.State)
	if err != nil {
		return plugins.Result{}, err
	}
	if err := validateManagedFragment(host.ProjectPath, state.FragmentDigest); err != nil {
		return plugins.Result{}, err
	}
	runtimeDir, err := RuntimeDir(host.ProjectPath)
	if err != nil {
		return plugins.Result{}, err
	}
	if err := WriteAdapterScripts(runtimeDir); err != nil {
		return plugins.Result{}, err
	}
	unit, err := prepareServiceUnit(host.ProjectPath, state)
	if err != nil {
		return plugins.Result{}, err
	}
	oldState := state
	state.UnitDigest, state.UnitPath = unit.digest, unit.path
	if err := host.Store.SaveState(encodeState(state)); err != nil {
		return plugins.Result{}, errors.Join(err, unit.restore())
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return plugins.Result{}, p.rollbackEnsure(host, oldState, unit, err)
	}
	active := runSystemctl(ctx, "is-active", "--quiet", unitName(host.ProjectPath)) == nil
	if active && unit.changed {
		err = runSystemctl(ctx, "restart", unitName(host.ProjectPath))
	} else {
		err = runSystemctl(ctx, "enable", "--now", unitName(host.ProjectPath))
	}
	if err != nil {
		return plugins.Result{}, p.rollbackEnsure(host, oldState, unit, err)
	}
	if err := checkReady(ctx, runtimeDir); err != nil {
		return plugins.Result{}, p.rollbackEnsure(host, oldState, unit, err)
	}
	return plugins.Result{State: encodeState(state)}, nil
}

func (p *Plugin) rollbackEnsure(host plugins.HostContext, oldState ownershipState, unit serviceUnitChange, cause error) error {
	restoreErr := errors.Join(unit.restore(), host.Store.SaveState(encodeState(oldState)), runSystemctl(context.Background(), "daemon-reload"))
	if restoreErr == nil && unit.changed && runSystemctl(context.Background(), "is-active", "--quiet", unitName(host.ProjectPath)) == nil {
		restoreErr = runSystemctl(context.Background(), "restart", unitName(host.ProjectPath))
	}
	if restoreErr != nil {
		return fmt.Errorf("%w; previous unit restore failed: %w", cause, restoreErr)
	}
	return cause
}

func (p *Plugin) startAndCheck(ctx context.Context, projectPath string) error {
	_, runtimeDir, err := renderFragment(projectPath)
	if err != nil {
		return err
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "enable", "--now", unitName(projectPath)); err != nil {
		return err
	}
	return checkReady(ctx, runtimeDir)
}

func (p *Plugin) ServeHost(ctx context.Context, host plugins.HostContext) error {
	cfg, err := decodeConfig(host.Config)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	runtimeDir, err := RuntimeDir(host.ProjectPath)
	if err != nil {
		return err
	}
	return Serve(ctx, filepath.Join(runtimeDir, "credential.sock"), cfg)
}

func renderFragment(path string) ([]byte, string, error) {
	runtimeDir, err := RuntimeDir(path)
	if err != nil {
		return nil, "", err
	}
	fragment, err := AdapterFragment(runtimeDir)
	return fragment, runtimeDir, err
}

func replaceManagedFragment(path string, fragment []byte, previousDigest string) (string, []byte, bool, error) {
	fragmentPath := managedFragment(path)
	oldBytes, oldExists, err := readOwnedFile(fragmentPath, previousDigest)
	if err != nil {
		return "", nil, false, err
	}
	if !oldExists {
		if _, err := os.Lstat(fragmentPath); err == nil {
			return "", nil, false, fmt.Errorf("plugin fragment already exists and is not managed by dch")
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", nil, false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		return "", nil, false, err
	}
	if err := writePrivateFile(fragmentPath, fragment, 0600); err != nil {
		return "", nil, false, err
	}
	return fragmentPath, oldBytes, oldExists, nil
}

func validateManagedFragment(path, expected string) error {
	_, exists, err := readOwnedFile(managedFragment(path), expected)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("managed plugin fragment is missing")
	}
	return nil
}

func checkReady(ctx context.Context, runtimeDir string) error {
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
	if err := CheckCredential(ctx, socketPath); err != nil {
		return fmt.Errorf("credential service validation failed: %w", err)
	}
	return nil
}

func ownedUnitPath(path string, state ownershipState) (string, error) {
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
	runtimeDir, err := RuntimeDir(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(runtimeDir)
}

func removeManagedFragment(path, expectedDigest string) error {
	fragmentPath := managedFragment(path)
	_, exists, err := readOwnedFile(fragmentPath, expectedDigest)
	if err != nil || !exists {
		return err
	}
	return os.Remove(fragmentPath)
}

func removeCredentialSocket(runtimeDir string) error {
	err := os.Remove(filepath.Join(runtimeDir, "credential.sock"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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
		return writePrivateFile(change.path, change.before, 0600)
	}
	return removeIfPresent(change.path)
}

func prepareServiceUnit(projectPath string, previous ownershipState) (serviceUnitChange, error) {
	unit, err := serviceUnitPath(projectPath, previous)
	if err != nil {
		return serviceUnitChange{}, err
	}
	content, err := serviceUnitContent(projectPath)
	if err != nil {
		return serviceUnitChange{}, err
	}
	change := serviceUnitChange{path: unit, digest: digest(content)}
	return prepareUnitFile(change, previous.UnitDigest, content)
}

func serviceUnitPath(projectPath string, previous ownershipState) (string, error) {
	if previous.UnitPath != "" {
		if err := validateUnitPath(previous.UnitPath, projectPath); err != nil {
			return "", err
		}
		return previous.UnitPath, nil
	}
	return unitPath(projectPath)
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
	if err := writePrivateFile(change.path, content, 0600); err != nil {
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
	change.before, change.existed = existing, true
	change.changed = !bytes.Equal(existing, content)
	if change.changed {
		if err := writePrivateFile(change.path, content, 0600); err != nil {
			return change, err
		}
	}
	return change, nil
}

func serviceUnitContent(projectPath string) ([]byte, error) {
	configPath, err := common.ConfigPath()
	if err != nil {
		return nil, err
	}
	bin, err := os.Executable()
	if err != nil {
		return nil, err
	}
	content := "[Unit]\nDescription=dch GitHub App credential service\n\n[Service]\nType=simple\nExecStart=" + systemdExecArg(bin) + " _plugin-serve --plugin github-app --project " + systemdExecArg(projectPath) + "\nRestart=on-failure\n"
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

func validateUnitPath(path, projectPath string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != unitName(projectPath) || filepath.Base(filepath.Dir(path)) != "user" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "systemd" {
		return fmt.Errorf("stored systemd unit path is invalid")
	}
	return nil
}

func verifyOwnedUnit(path, expectedDigest string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
		return fmt.Errorf("systemd unit is unsafe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if expectedDigest == "" || digest(data) != expectedDigest {
		return fmt.Errorf("systemd unit was modified; refusing to remove it")
	}
	return nil
}

func runSystemctl(ctx context.Context, args ...string) error {
	args = append([]string{"--user"}, args...)
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "systemctl", args...)
	var output limitedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
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
func unitName(path string) string         { return "dch-github-app-" + digest([]byte(path))[:16] + ".service" }

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
	if err != nil {
		return nil, false, err
	}
	if expected == "" || digest(data) != expected {
		return nil, false, fmt.Errorf("plugin fragment was modified; refusing to overwrite or remove it")
	}
	return data, true, nil
}

func restoreFile(path string, data []byte, existed bool) error {
	if existed {
		return writePrivateFile(path, data, 0600)
	}
	return removeIfPresent(path)
}

func appendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}
