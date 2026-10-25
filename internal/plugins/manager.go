package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"dch/internal/common"
)

// Status aggregates the registered plugins' enabled and pending-build state.
type Status struct {
	Plugins []PluginStatus
	// Enabled is the count of enabled plugins in this project.
	Enabled int
	// PendingBuild is true when any plugin needs a container rebuild.
	PendingBuild bool
}

// Manager registers plugins and coordinates their project-scoped lifecycle.
type Manager struct {
	plugins map[string]Plugin
	aliases map[string]string
}

type operationContextKey struct{}

type operationToken struct {
	manager *Manager
	path    string
}

// NewManager returns an empty plugin manager.
func NewManager() *Manager {
	return &Manager{plugins: map[string]Plugin{}, aliases: map[string]string{}}
}

// Register adds a plugin and rejects duplicate IDs or service aliases.
func (m *Manager) Register(plugin Plugin) error {
	if plugin == nil {
		return errors.New("plugin cannot be nil")
	}
	definition := plugin.Definition()
	id := strings.TrimSpace(definition.ID)
	if id == "" || id != definition.ID {
		return errors.New("plugin ID must be non-empty and trimmed")
	}
	if _, exists := m.plugins[id]; exists {
		return fmt.Errorf("plugin ID %q is already registered", id)
	}
	if err := m.validateAliases(id, plugin, definition.ServiceAliases); err != nil {
		return err
	}
	m.plugins[id] = plugin
	for _, alias := range definition.ServiceAliases {
		m.aliases[alias] = id
	}
	return nil
}

// SupportsInteractive reports whether a plugin accepts interactive configuration.
func (m *Manager) SupportsInteractive(id string) bool {
	plugin, ok := m.plugins[id]
	if !ok {
		return false
	}
	_, ok = plugin.(InteractiveConfigurer)
	return ok
}

// SharedConfig returns a copy of one plugin's opaque shared settings.
func (m *Manager) SharedConfig(ctx context.Context, id string) (json.RawMessage, error) {
	if _, err := m.plugin(id); err != nil {
		return nil, err
	}
	cfg, err := common.ReadConfig()
	if err != nil {
		return nil, err
	}
	return cloneRaw(cfg.PluginSettings[id]), nil
}

// SetSharedConfig validates and stores one plugin's opaque shared settings.
func (m *Manager) SetSharedConfig(ctx context.Context, id string, raw json.RawMessage) error {
	plugin, err := m.plugin(id)
	if err != nil {
		return err
	}
	if validator, ok := plugin.(SharedSettingsValidator); ok {
		if err := validator.ValidateSharedSettings(raw); err != nil {
			return fmt.Errorf("plugin %q shared settings: %w", id, err)
		}
	}
	return common.UpdateConfig(func(config *common.Config) error {
		if config.PluginSettings == nil {
			config.PluginSettings = map[string]json.RawMessage{}
		}
		if raw == nil {
			delete(config.PluginSettings, id)
		} else {
			config.PluginSettings[id] = cloneRaw(raw)
		}
		return nil
	})
}

// ConfigureAndEnable runs interactive preparation outside the project lock,
// then rechecks the snapshot and commits its candidate while holding that lock.
func (m *Manager) ConfigureAndEnable(ctx context.Context, projectPath, id string, interaction Interaction) error {
	plugin, err := m.plugin(id)
	if err != nil {
		return err
	}
	configurer, ok := plugin.(InteractiveConfigurer)
	if !ok {
		return errors.New("plugin does not support interactive configuration")
	}
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	before, err := m.loadProject(path)
	if err != nil {
		return err
	}
	if err := requireRegisteredProject(path); err != nil {
		return err
	}
	oldConfig, wasEnabled := before.Enabled[id]
	oldState := before.States[id]
	shared, err := m.SharedConfig(ctx, id)
	if err != nil {
		return err
	}
	host, err := m.hostContext(path, id, oldConfig, oldState.Data, nil, nil)
	if err != nil {
		return err
	}
	host.SharedConfig = cloneRaw(shared)
	prepared, err := configurer.Configure(ctx, ConfigureContext{Host: host, SharedConfig: cloneRaw(shared)}, interaction)
	if err != nil {
		return pluginError(id, EventEnable, err)
	}
	if err := plugin.ValidateConfig(prepared.Config); err != nil {
		return pluginError(id, EventEnable, err)
	}
	if prepared.Commit == nil {
		return errors.New("interactive configuration has no commit hook")
	}
	lockedPath, unlock, err := m.lockProject(ctx, path)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := m.loadProject(lockedPath)
	if err != nil {
		return err
	}
	if err := m.validateInteractiveSnapshot(ctx, lockedPath, id, current, wasEnabled, oldConfig, oldState.Data, shared); err != nil {
		return err
	}
	commitHost, err := m.hostContext(lockedPath, id, prepared.Config, oldState.Data, oldConfig, oldState.Data)
	if err != nil {
		return err
	}
	result, err := prepared.Commit(ctx, commitHost)
	if err != nil {
		return pluginError(id, EventEnable, err)
	}
	return m.commitEnable(lockedPath, id, prepared.Config, oldConfig, wasEnabled, result)
}

func (m *Manager) validateInteractiveSnapshot(ctx context.Context, path, id string, current common.ProjectPlugins, wasEnabled bool, oldConfig, oldState, shared json.RawMessage) error {
	if err := requireRegisteredProject(path); err != nil {
		return err
	}
	currentConfig, enabled := current.Enabled[id]
	if enabled != wasEnabled || !bytes.Equal(currentConfig, oldConfig) || !bytes.Equal(current.States[id].Data, oldState) {
		return errors.New("project plugin configuration changed during interactive setup; retry")
	}
	latestShared, err := m.SharedConfig(ctx, id)
	if err != nil {
		return err
	}
	if !bytes.Equal(latestShared, shared) {
		return errors.New("plugin shared settings changed during interactive setup; retry")
	}
	return nil
}

func requireRegisteredProject(path string) error {
	registered, err := projectRegistered(path)
	if err != nil {
		return err
	}
	if !registered {
		return fmt.Errorf("project %s is no longer registered", path)
	}
	return nil
}

func (m *Manager) validateAliases(id string, plugin Plugin, aliases []string) error {
	if len(aliases) > 0 {
		if _, ok := plugin.(HostService); !ok {
			return fmt.Errorf("plugin %q has service aliases but no host service", id)
		}
	}
	seenAliases := map[string]struct{}{}
	for _, alias := range aliases {
		if len(alias) < 2 || strings.TrimSpace(alias) != alias || !strings.HasPrefix(alias, "_") {
			return fmt.Errorf("plugin %q service alias %q must start with _", id, alias)
		}
		if _, exists := seenAliases[alias]; exists {
			return fmt.Errorf("plugin %q repeats service alias %q", id, alias)
		}
		seenAliases[alias] = struct{}{}
		if _, exists := m.aliases[alias]; exists {
			return fmt.Errorf("service alias %q is already registered", alias)
		}
	}
	return nil
}

// Definitions returns registered plugin metadata sorted by stable ID.
func (m *Manager) Definitions() []Definition {
	ids := m.pluginIDs()
	definitions := make([]Definition, 0, len(ids))
	for _, id := range ids {
		definitions = append(definitions, m.plugins[id].Definition())
	}
	return definitions
}

// ServiceAlias maps a registered internal command alias to its plugin ID.
func (m *Manager) ServiceAlias(command string) (string, bool) {
	id, ok := m.aliases[command]
	return id, ok
}

// WithProjectOperation serializes a project operation across processes. Manager
// methods called with the callback context reuse the lock; the context is canceled
// on return so it cannot bypass a later lock acquisition.
func (m *Manager) WithProjectOperation(ctx context.Context, projectPath string, operation func(context.Context) error) error {
	if operation == nil {
		return errors.New("project operation cannot be nil")
	}
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if token, ok := ctx.Value(operationContextKey{}).(operationToken); ok && token.manager == m && token.path == path && ctx.Err() == nil {
		return operation(ctx)
	}
	operationCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	operationCtx = context.WithValue(operationCtx, operationContextKey{}, operationToken{manager: m, path: path})
	return operation(operationCtx)
}

// Enable validates configuration, runs the plugin's host hook, then commits its
// enabled record and lifecycle result. The hook owns rollback of early scoped saves.
func (m *Manager) Enable(ctx context.Context, projectPath, id string, raw json.RawMessage) error {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	plugin, err := m.plugin(id)
	if err != nil {
		return err
	}
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	registered, err := projectRegistered(path)
	if err != nil {
		return err
	}
	if !registered {
		return fmt.Errorf("project %s is no longer registered", path)
	}
	if err := plugin.ValidateConfig(raw); err != nil {
		return pluginError(id, EventEnable, err)
	}
	oldConfig, wasEnabled := project.Enabled[id]
	oldState, hadState := project.States[id]
	host, err := m.hostContext(path, id, raw, oldState.Data, oldConfig, oldState.Data)
	if err != nil {
		return err
	}
	if !wasEnabled {
		host.PreviousConfig = nil
	}
	if !hadState {
		host.PreviousState = nil
	}
	result, err := plugin.HandleHost(ctx, host, EventEnable)
	if err != nil {
		return pluginError(id, EventEnable, err)
	}
	return m.commitEnable(path, id, raw, oldConfig, wasEnabled, result)
}

func (m *Manager) commitEnable(path, id string, raw, previousConfig json.RawMessage, wasEnabled bool, result Result) error {
	return common.UpdateConfig(func(config *common.Config) error {
		project := projectAt(config, path)
		if project.Enabled == nil {
			project.Enabled = map[string]json.RawMessage{}
		}
		if project.States == nil {
			project.States = map[string]common.PluginState{}
		}
		currentConfig, alreadyEnabled := project.Enabled[id]
		if result.Config != nil {
			raw = result.Config
		} else if alreadyEnabled && (!wasEnabled || !bytes.Equal(currentConfig, previousConfig)) {
			raw = currentConfig
		}
		project.Enabled[id] = raw
		state := project.States[id]
		if result.State != nil {
			state.Data = result.State
		}
		state.BuildPending = state.BuildPending || result.NeedsBuild
		project.States[id] = state
		config.ProjectPlugins[path] = project
		return nil
	})
}

// Disable runs cleanup before removing the enabled record, preserving it when
// cleanup fails so the plugin can retry.
func (m *Manager) Disable(ctx context.Context, projectPath, id string) error {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	plugin, err := m.plugin(id)
	if err != nil {
		return err
	}
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	config := project.Enabled[id]
	state := project.States[id]
	if !has(project.Enabled, id) && len(state.Data) == 0 {
		return nil
	}
	host, err := m.hostContext(path, id, config, state.Data, nil, nil)
	if err != nil {
		return err
	}
	result, err := plugin.HandleHost(ctx, host, EventDisable)
	if err != nil {
		return pluginError(id, EventDisable, err)
	}
	return m.commitDisable(path, id, result)
}

func (m *Manager) commitDisable(path, id string, result Result) error {
	return common.UpdateConfig(func(config *common.Config) error {
		project := projectAt(config, path)
		delete(project.Enabled, id)
		state := project.States[id]
		if result.State != nil {
			state.Data = result.State
		}
		state.BuildPending = state.BuildPending || result.NeedsBuild
		if len(state.Data) == 0 && !state.BuildPending {
			delete(project.States, id)
		} else {
			setState(&project, id, state)
		}
		config.ProjectPlugins[path] = project
		return nil
	})
}

// Prepare dispatches a Start, Build, or Connect host hook before container work.
func (m *Manager) Prepare(ctx context.Context, projectPath string, event Event) error {
	if !hostLifecycleEvent(event) {
		return fmt.Errorf("unsupported host event %q", event)
	}
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	for _, id := range sortedEnabled(project) {
		plugin := m.plugins[id]
		state := project.States[id]
		host, err := m.hostContext(path, id, project.Enabled[id], state.Data, nil, nil)
		if err != nil {
			return err
		}
		result, err := plugin.HandleHost(ctx, host, event)
		if err != nil {
			return pluginError(id, event, err)
		}
		if err := m.commitHookResult(path, id, result); err != nil {
			return pluginError(id, event, err)
		}
		project, err = m.loadProject(path)
		if err != nil {
			return err
		}
	}
	return nil
}

// HandleContainer dispatches enabled plugins' container hooks with the supplied
// executor. Call it after up succeeds or before opening a container shell.
func (m *Manager) HandleContainer(ctx context.Context, projectPath string, event Event, executor Executor) error {
	if !hostLifecycleEvent(event) {
		return fmt.Errorf("unsupported container event %q", event)
	}
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	for _, id := range sortedEnabled(project) {
		state := project.States[id]
		container := ContainerContext{ProjectPath: path, Config: project.Enabled[id], State: state.Data, Executor: executor}
		result, err := m.plugins[id].HandleContainer(ctx, container, event)
		if err != nil {
			return pluginError(id, event, err)
		}
		if err := m.commitHookResult(path, id, result); err != nil {
			return pluginError(id, event, err)
		}
		project, err = m.loadProject(path)
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateMergedConfig asks each enabled plugin to validate the final merged
// container configuration before the container operation.
func (m *Manager) ValidateMergedConfig(ctx context.Context, projectPath string, merged map[string]json.RawMessage) error {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	for _, id := range sortedEnabled(project) {
		state := project.States[id]
		container := ContainerContext{ProjectPath: path, Config: project.Enabled[id], State: state.Data}
		if err := m.plugins[id].ValidateContainerConfig(ctx, container, merged); err != nil {
			return pluginError(id, EventBuild, err)
		}
	}
	return nil
}

// Statuses returns one display-safe status per registered plugin, sorted by ID.
func (m *Manager) Statuses(ctx context.Context, projectPath string) ([]PluginStatus, error) {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return nil, err
	}
	defer unlock()
	project, err := m.loadProject(path)
	if err != nil {
		return nil, err
	}
	statuses := make([]PluginStatus, 0, len(m.plugins))
	for _, id := range m.pluginIDs() {
		state := project.States[id]
		host, err := m.hostContext(path, id, project.Enabled[id], state.Data, nil, nil)
		if err != nil {
			return nil, err
		}
		status, err := m.plugins[id].Status(ctx, host)
		if err != nil {
			return nil, pluginError(id, "status", err)
		}
		status.ID = id
		status.Name = m.plugins[id].Definition().Name
		status.Enabled = has(project.Enabled, id)
		status.PendingBuild = state.BuildPending
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// Status aggregates enabled count and pending-build state across the project.
func (m *Manager) Status(ctx context.Context, projectPath string) (Status, error) {
	statuses, err := m.Statuses(ctx, projectPath)
	if err != nil {
		return Status{}, err
	}
	result := Status{Plugins: statuses}
	for _, status := range statuses {
		if status.Enabled {
			result.Enabled++
		}
		result.PendingBuild = result.PendingBuild || status.PendingBuild
	}
	return result, nil
}

// Config returns the enabled plugin's config, or nil when that plugin is disabled.
func (m *Manager) Config(ctx context.Context, projectPath, id string) (json.RawMessage, error) {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if _, err := m.plugin(id); err != nil {
		return nil, err
	}
	project, err := m.loadProject(path)
	if err != nil {
		return nil, err
	}
	return cloneRaw(project.Enabled[id]), nil
}

// MarkBuilt clears pending-build state after up and all required container hooks
// have succeeded.
func (m *Manager) MarkBuilt(ctx context.Context, projectPath string) error {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = m.loadProject(path)
	if err != nil {
		return err
	}
	return common.UpdateConfig(func(config *common.Config) error {
		project := projectAt(config, path)
		for id, state := range project.States {
			state.BuildPending = false
			if len(state.Data) == 0 {
				delete(project.States, id)
			} else {
				project.States[id] = state
			}
		}
		config.ProjectPlugins[path] = project
		return nil
	})
}

// RemoveProject cleans enabled or residual plugin state before unregistering the
// project. A failed cleanup leaves the registration and retry data in place.
func (m *Manager) RemoveProject(ctx context.Context, projectPath string) error {
	path, unlock, err := m.lockProject(ctx, projectPath)
	if err != nil {
		return err
	}
	defer unlock()
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	registered, err := projectRegistered(path)
	if err != nil {
		return err
	}
	if !registered {
		return fmt.Errorf("project %s is no longer registered", path)
	}
	ids := unionIDs(project)
	for _, id := range ids {
		state := project.States[id]
		if !has(project.Enabled, id) && len(state.Data) == 0 {
			continue
		}
		if err := m.cleanupProjectPlugin(ctx, path, id, project.Enabled[id], state.Data); err != nil {
			return err
		}
		project, err = m.loadProject(path)
		if err != nil {
			return err
		}
	}
	return common.RemoveProjectRegistration(path)
}

func (m *Manager) cleanupProjectPlugin(ctx context.Context, path, id string, config, state json.RawMessage) error {
	host, err := m.hostContext(path, id, config, state, nil, nil)
	if err != nil {
		return err
	}
	result, err := m.plugins[id].HandleHost(ctx, host, EventDisable)
	if err != nil {
		return pluginError(id, EventDisable, err)
	}
	return m.commitDisable(path, id, result)
}

func projectRegistered(path string) (bool, error) {
	config, err := common.ReadConfig()
	if err != nil {
		return false, err
	}
	for _, registered := range config.Projects {
		if registered == path {
			return true, nil
		}
	}
	return false, nil
}

// Serve runs an enabled plugin's optional host service. It holds only the short
// global config lock while loading plugin data, never the project operation lock.
func (m *Manager) Serve(ctx context.Context, projectPath, id string) error {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return err
	}
	plugin, err := m.plugin(id)
	if err != nil {
		return err
	}
	service, ok := plugin.(HostService)
	if !ok {
		return fmt.Errorf("plugin %q has no host service", id)
	}
	project, err := m.loadProject(path)
	if err != nil {
		return err
	}
	if !has(project.Enabled, id) {
		return fmt.Errorf("plugin %q is not enabled", id)
	}
	state := project.States[id]
	host, err := m.hostContext(path, id, project.Enabled[id], state.Data, nil, nil)
	if err != nil {
		return err
	}
	return service.ServeHost(ctx, host)
}

func (m *Manager) commitHookResult(path, id string, result Result) error {
	return common.UpdateConfig(func(config *common.Config) error {
		project := projectAt(config, path)
		state := project.States[id]
		if result.Config != nil && has(project.Enabled, id) {
			project.Enabled[id] = result.Config
		}
		if result.State != nil {
			state.Data = result.State
		}
		state.BuildPending = state.BuildPending || result.NeedsBuild
		if len(state.Data) > 0 || state.BuildPending {
			setState(&project, id, state)
		}
		config.ProjectPlugins[path] = project
		return nil
	})
}

func (m *Manager) hostContext(path, id string, config, state, previousConfig, previousState json.RawMessage) (HostContext, error) {
	shared, err := m.SharedConfig(context.Background(), id)
	if err != nil {
		return HostContext{}, err
	}
	return HostContext{ProjectPath: path, SharedConfig: shared, Config: cloneRaw(config), State: cloneRaw(state), PreviousConfig: cloneRaw(previousConfig), PreviousState: cloneRaw(previousState), Store: scopedStore{path: path, id: id}}, nil
}

func (m *Manager) loadProject(path string) (common.ProjectPlugins, error) {
	config, err := common.ReadConfig()
	if err != nil {
		return common.ProjectPlugins{}, err
	}
	project := config.ProjectPlugins[path]
	if err := validateIDs(project, m.plugins); err != nil {
		return common.ProjectPlugins{}, err
	}
	return project, nil
}

func (m *Manager) lockProject(ctx context.Context, projectPath string) (string, func(), error) {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return "", nil, err
	}
	if token, ok := ctx.Value(operationContextKey{}).(operationToken); ok && token.manager == m && token.path == path && ctx.Err() == nil {
		return path, func() {}, nil
	}
	return lockProject(path)
}

func (m *Manager) plugin(id string) (Plugin, error) {
	plugin, ok := m.plugins[id]
	if !ok {
		return nil, fmt.Errorf("unknown plugin %q", id)
	}
	return plugin, nil
}

func (m *Manager) pluginIDs() []string {
	ids := make([]string, 0, len(m.plugins))
	for id := range m.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

type scopedStore struct {
	path string
	id   string
}

func (s scopedStore) SaveConfig(raw json.RawMessage) error {
	return common.UpdateConfig(func(config *common.Config) error {
		project := projectAt(config, s.path)
		if raw == nil {
			delete(project.Enabled, s.id)
		} else {
			if project.Enabled == nil {
				project.Enabled = map[string]json.RawMessage{}
			}
			project.Enabled[s.id] = cloneRaw(raw)
		}
		config.ProjectPlugins[s.path] = project
		return nil
	})
}

func (s scopedStore) SaveState(raw json.RawMessage) error {
	return common.UpdateConfig(func(config *common.Config) error {
		project := projectAt(config, s.path)
		state := project.States[s.id]
		state.Data = cloneRaw(raw)
		if len(state.Data) == 0 && !state.BuildPending {
			delete(project.States, s.id)
		} else {
			setState(&project, s.id, state)
		}
		config.ProjectPlugins[s.path] = project
		return nil
	})
}

func projectAt(config *common.Config, path string) common.ProjectPlugins {
	if config.ProjectPlugins == nil {
		config.ProjectPlugins = map[string]common.ProjectPlugins{}
	}
	return config.ProjectPlugins[path]
}

func setState(project *common.ProjectPlugins, id string, state common.PluginState) {
	if project.States == nil {
		project.States = map[string]common.PluginState{}
	}
	project.States[id] = state
}

func validateIDs(project common.ProjectPlugins, registered map[string]Plugin) error {
	known := make(map[string]struct{}, len(registered))
	for id := range registered {
		known[id] = struct{}{}
	}
	return common.ValidateProjectPlugins(project, known)
}

func sortedEnabled(project common.ProjectPlugins) []string {
	ids := make([]string, 0, len(project.Enabled))
	for id := range project.Enabled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func unionIDs(project common.ProjectPlugins) []string {
	set := map[string]struct{}{}
	for id := range project.Enabled {
		set[id] = struct{}{}
	}
	for id := range project.States {
		set[id] = struct{}{}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func has(values map[string]json.RawMessage, id string) bool {
	_, ok := values[id]
	return ok
}

func hostLifecycleEvent(event Event) bool {
	return event == EventStart || event == EventBuild || event == EventConnect
}

func pluginError(id string, event Event, err error) error {
	return fmt.Errorf("plugin %q event %q: %w", id, event, err)
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func lockProject(projectPath string) (string, func(), error) {
	path, err := common.NormalizePath(projectPath)
	if err != nil {
		return "", nil, err
	}
	configPath, err := common.ConfigPath()
	if err != nil {
		return "", nil, err
	}
	hash := sha256.Sum256([]byte(path))
	lockPath := filepath.Join(filepath.Dir(configPath), ".dch-plugin-"+hex.EncodeToString(hash[:]))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return "", nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return "", nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return "", nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || stat.Uid != uint32(os.Geteuid()) {
		file.Close()
		return "", nil, fmt.Errorf("unsafe project operation lock file %s", lockPath)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return "", nil, err
	}
	return path, func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
