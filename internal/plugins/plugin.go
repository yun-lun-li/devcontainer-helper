package plugins

import (
	"context"
	"encoding/json"
)

type Event string

const (
	EventEnable  Event = "enable"
	EventDisable Event = "disable"
	EventStart   Event = "start"
	EventBuild   Event = "build"
	EventConnect Event = "connect"
)

type FieldKind string

const (
	FieldText   FieldKind = "text"
	FieldChoice FieldKind = "choice"
)

type Choice struct {
	Value string
	Label string
}

type Field struct {
	Name        string
	Label       string
	Description string
	Placeholder string
	Required    bool
	Kind        FieldKind
	Options     []Choice
	Path        []string
}

type Definition struct {
	ID             string
	Name           string
	Description    string
	Fields         []Field
	Notices        []string
	ServiceAliases []string
}

type Result struct {
	NeedsBuild bool
	Config     json.RawMessage
	State      json.RawMessage
}

type PluginStatus struct {
	ID           string
	Name         string
	Enabled      bool
	PendingBuild bool
	Message      string
}

type HostContext struct {
	ProjectPath    string
	Config         json.RawMessage
	State          json.RawMessage
	PreviousConfig json.RawMessage
	PreviousState  json.RawMessage
	Store          ScopedStore
}

type ContainerContext struct {
	ProjectPath string
	Config      json.RawMessage
	State       json.RawMessage
	Executor    Executor
}

type ScopedStore interface {
	// SaveConfig and SaveState delete the scoped value when passed nil.
	SaveConfig(json.RawMessage) error
	SaveState(json.RawMessage) error
}

type Executor interface {
	Exec(context.Context, ...string) ([]byte, error)
}

type Plugin interface {
	Definition() Definition
	ValidateConfig(json.RawMessage) error
	HandleHost(context.Context, HostContext, Event) (Result, error)
	HandleContainer(context.Context, ContainerContext, Event) (Result, error)
	ValidateContainerConfig(context.Context, ContainerContext, map[string]json.RawMessage) error
	Status(context.Context, HostContext) (PluginStatus, error)
}

type HostService interface {
	ServeHost(context.Context, HostContext) error
}
