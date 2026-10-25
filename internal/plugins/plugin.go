package plugins

import (
	"context"
	"encoding/json"
)

// Event identifies a plugin lifecycle action.
type Event string

const (
	// EventEnable validates and prepares the plugin for an enabled project.
	EventEnable Event = "enable"
	// EventDisable asks the plugin to clean up its host resources.
	EventDisable Event = "disable"
	// EventStart runs before starting a container and after it is up.
	EventStart Event = "start"
	// EventBuild runs before and after rebuilding a container.
	EventBuild Event = "build"
	// EventConnect runs before entering an existing container.
	EventConnect Event = "connect"
)

// FieldKind selects the renderer used for a configuration field.
type FieldKind string

const (
	// FieldText renders a free-form text input.
	FieldText FieldKind = "text"
	// FieldChoice renders a choice from the field's Options.
	FieldChoice FieldKind = "choice"
)

// Choice is a selectable value and its display label.
type Choice struct {
	Value string
	Label string
}

// Field describes one plugin-owned setting for a generic configuration form.
// Path identifies its nested JSON value; an empty path uses Name.
type Field struct {
	Name        string
	Label       string
	Description string
	Placeholder string
	Required    bool
	// Confirm includes this non-secret field in the enable confirmation summary.
	Confirm bool
	Kind    FieldKind
	Options []Choice
	Path    []string
}

// Definition contains the stable identity and generic UI metadata for a plugin.
type Definition struct {
	ID          string
	Name        string
	Description string
	Fields      []Field
	// SharedFields describes plugin-owned settings used across projects.
	SharedFields []Field
	Notices      []string
	// ServiceAliases are internal command names and require HostService support.
	ServiceAliases []string
}

// Result returns plugin-owned values and lifecycle effects from a hook.
// Nil Config keeps the candidate or currently stored config, including scoped
// writes made during the hook; nil State keeps the current private state.
// NeedsBuild marks the plugin pending; a successful unrelated hook does not clear
// it. The caller clears pending state with Manager.MarkBuilt after container hooks.
type Result struct {
	NeedsBuild bool
	Config     json.RawMessage
	State      json.RawMessage
}

// PluginStatus contains display-safe status details for one plugin.
type PluginStatus struct {
	ID           string
	Name         string
	Enabled      bool
	PendingBuild bool
	Message      string
}

// HostContext gives a plugin its project-scoped data and persistence capability.
// Config and State are this plugin's candidate/current values; PreviousConfig and
// PreviousState let Enable restore earlier values if its rollback succeeds.
type HostContext struct {
	ProjectPath    string
	SharedConfig   json.RawMessage
	Config         json.RawMessage
	State          json.RawMessage
	PreviousConfig json.RawMessage
	PreviousState  json.RawMessage
	Store          ScopedStore
}

// SharedSettingsValidator validates one plugin's opaque global settings.
type SharedSettingsValidator interface {
	ValidateSharedSettings(json.RawMessage) error
}

// ConfigureContext contains the project and opaque shared settings for an
// interactive authorization or configuration flow.
type ConfigureContext struct {
	Host         HostContext
	SharedConfig json.RawMessage
}

// Interaction provides generic host UI capabilities to an interactive plugin.
type Interaction interface {
	Notify(context.Context, string, string) error
	Select(context.Context, string, []Choice, int) (string, error)
	Confirm(context.Context, string, string) (bool, error)
	OpenBrowser(context.Context, string) error
}

// PreparedConfiguration holds a candidate project config and a commit hook.
// The hook runs under the project lock after Manager revalidates its snapshot.
type PreparedConfiguration struct {
	Config json.RawMessage
	Commit func(context.Context, HostContext) (Result, error)
}

// InteractiveConfigurer prepares a configuration without holding the project lock.
type InteractiveConfigurer interface {
	Configure(context.Context, ConfigureContext, Interaction) (PreparedConfiguration, error)
}

// ContainerContext gives a plugin its own project data and a host-side executor
// for commands that must run inside the container.
type ContainerContext struct {
	ProjectPath string
	Config      json.RawMessage
	State       json.RawMessage
	Executor    Executor
}

// ScopedStore reads and writes only one plugin's config and private state.
type ScopedStore interface {
	// SaveConfig persists only this plugin's config; nil deletes it. Plugins may
	// save before starting host resources that read the config.
	SaveConfig(json.RawMessage) error
	// SaveState persists only this plugin's opaque state; nil clears that data
	// without changing manager-owned pending-build state.
	SaveState(json.RawMessage) error
}

// Executor runs an argument vector inside a project's container from the host.
type Executor interface {
	Exec(context.Context, ...string) ([]byte, error)
}

// Plugin implements configuration validation, host and container lifecycle hooks,
// merged-container validation, and display-safe status. Host hooks run on the host;
// container hooks also run on the host and must use ContainerContext.Executor for
// commands inside the container. Manager dispatches plugins in ID order and stops
// after the first hook error.
type Plugin interface {
	// Definition describes this plugin's stable identity and generic UI fields.
	Definition() Definition
	// ValidateConfig validates only this plugin's candidate settings.
	ValidateConfig(json.RawMessage) error
	// HandleHost runs host-side work. Enable runs before manager commit, Disable
	// runs before the enabled record is removed, and lifecycle events run before
	// their associated container operation.
	HandleHost(context.Context, HostContext, Event) (Result, error)
	// HandleContainer runs after up or before shell access using only the executor
	// for commands inside the container.
	HandleContainer(context.Context, ContainerContext, Event) (Result, error)
	// ValidateContainerConfig checks the fully merged settings used by devcontainer.
	ValidateContainerConfig(context.Context, ContainerContext, map[string]json.RawMessage) error
	// Status returns display-safe details and must not expose secrets.
	Status(context.Context, HostContext) (PluginStatus, error)
}

// HostService is an optional long-running host service. Manager reads the enabled
// plugin context, then calls ServeHost without holding the project operation lock.
type HostService interface {
	// ServeHost serves this project's credentials until the context is canceled.
	ServeHost(context.Context, HostContext) error
}
