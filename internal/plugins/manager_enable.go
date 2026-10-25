package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"dch/internal/common"
	"dch/internal/githubapp"
)

func validateEnabledPlugins(project common.ProjectPlugins) error {
	for id := range project.Enabled {
		if id != githubAppID {
			return fmt.Errorf("unknown enabled plugin %q", id)
		}
	}
	return nil
}

func saveEnabledPlugin(config common.Config, path string, old common.ProjectPlugins, cfg githubapp.Config, fragment []byte, unit serviceUnitChange) error {
	if config.ProjectPlugins == nil {
		config.ProjectPlugins = map[string]common.ProjectPlugins{}
	}
	state := old.States[githubAppID]
	state.FragmentDigest = digest(fragment)
	state.UnitDigest = unit.digest
	state.UnitPath = unit.path
	state.BuildFingerprint = buildFingerprint(cfg, state.FragmentDigest)
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
	return common.SaveConfig(config)
}

func replaceManagedFragment(path string, fragment []byte, previousDigest string) (string, []byte, bool, error) {
	fragmentPath := managedFragment(path)
	oldBytes, oldExists, err := readOwnedFile(fragmentPath, previousDigest)
	if err != nil {
		return "", nil, false, err
	}
	if err := ensureFragmentAvailable(fragmentPath, oldExists); err != nil {
		return "", nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(fragmentPath), 0700); err != nil {
		return "", nil, false, err
	}
	if err := writePrivateFile(fragmentPath, fragment); err != nil {
		return "", nil, false, err
	}
	return fragmentPath, oldBytes, oldExists, nil
}

func ensureFragmentAvailable(path string, alreadyManaged bool) error {
	if alreadyManaged {
		return nil
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("plugin fragment already exists and is not managed by dch")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

type enableRollback struct {
	manager        *Manager
	path           string
	old            common.ProjectPlugins
	hadOld         bool
	wasEnabled     bool
	wasActive      bool
	fragmentPath   string
	oldBytes       []byte
	oldFileExists  bool
	runtimeDir     string
	unit           serviceUnitChange
	serviceTouched bool
}

func (rollback *enableRollback) run(cause error, serviceTouched bool) error {
	rollback.serviceTouched = serviceTouched
	cleanup := rollback.stopService()
	if len(cleanup) == 0 {
		cleanup = appendError(cleanup, rollback.unit.restore())
	}
	cleanup = appendError(cleanup, removeCredentialSocket(rollback.runtimeDir))
	if len(cleanup) > 0 {
		return fmt.Errorf("%w; cleanup failed, settings remain enabled for retry: %w", cause, errors.Join(cleanup...))
	}
	cleanup = rollback.restoreSettings()
	if rollback.wasActive && rollback.wasEnabled {
		cleanup = appendError(cleanup, rollback.manager.startAndCheck(context.Background(), rollback.path))
	}
	if len(cleanup) > 0 {
		return fmt.Errorf("%w; rollback incomplete: %w", cause, errors.Join(cleanup...))
	}
	return cause
}

type ensureRollback struct {
	unit            serviceUnitChange
	metadataChanged bool
	project         common.ProjectPlugins
	config          common.Config
	path            string
	oldState        common.PluginState
}

func (rollback ensureRollback) run(cause error) error {
	if !rollback.metadataChanged {
		return cause
	}
	var restoreErr error
	restoreErr = errors.Join(restoreErr, rollback.unit.restore())
	rollback.project.States[githubAppID] = rollback.oldState
	rollback.config.ProjectPlugins[rollback.path] = rollback.project
	restoreErr = errors.Join(restoreErr, common.SaveConfig(rollback.config))
	restoreErr = errors.Join(restoreErr, runSystemctl(context.Background(), "daemon-reload"))
	if restoreErr != nil {
		return fmt.Errorf("%w; previous unit restore failed: %w", cause, restoreErr)
	}
	return cause
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

func (rollback *enableRollback) stopService() []error {
	if !rollback.serviceTouched {
		return nil
	}
	var cleanup []error
	for _, action := range [][]string{{"stop", unitName(rollback.path)}, {"disable", unitName(rollback.path)}, {"daemon-reload"}} {
		cleanup = appendError(cleanup, runSystemctl(context.Background(), action...))
	}
	return cleanup
}

func (rollback *enableRollback) restoreSettings() []error {
	var cleanup []error
	cleanup = appendError(cleanup, restoreFile(rollback.fragmentPath, rollback.oldBytes, rollback.oldFileExists))
	current, err := common.LoadConfig()
	if err != nil {
		return appendError(cleanup, err)
	}
	if rollback.hadOld {
		current.ProjectPlugins[rollback.path] = rollback.old
	} else {
		delete(current.ProjectPlugins, rollback.path)
	}
	return appendError(cleanup, common.SaveConfig(current))
}

func removeCredentialSocket(runtimeDir string) error {
	err := os.Remove(filepath.Join(runtimeDir, "credential.sock"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func appendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}
