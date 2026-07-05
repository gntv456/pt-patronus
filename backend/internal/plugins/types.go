package plugins

import (
	"context"
	"time"
)

const RuntimeBuiltin = "builtin"
const RuntimeExternalHTTP = "external-http"

type RuntimeEntry struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	WorkDir string            `json:"work_dir,omitempty"`
	BaseURL string            `json:"base_url,omitempty"`
	Health  string            `json:"health,omitempty"`
}

type ConfigOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

type ConfigField struct {
	Key      string         `json:"key"`
	Label    string         `json:"label"`
	Type     string         `json:"type"`
	Required bool           `json:"required,omitempty"`
	Default  any            `json:"default,omitempty"`
	Options  []ConfigOption `json:"options,omitempty"`
	Help     string         `json:"help,omitempty"`
}

type EventSpec struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type ActionSpec struct {
	Name        string        `json:"name"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	InputSchema []ConfigField `json:"input_schema,omitempty"`
}

type Manifest struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Author       string        `json:"author"`
	Description  string        `json:"description"`
	Runtime      string        `json:"runtime"`
	Builtin      bool          `json:"built_in"`
	Entry        *RuntimeEntry `json:"entry,omitempty"`
	Permissions  []string      `json:"permissions"`
	Events       []EventSpec   `json:"events"`
	Actions      []ActionSpec  `json:"actions"`
	ConfigSchema []ConfigField `json:"config_schema"`
}

type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

type NoticeInput struct {
	Title string
	Body  string
	Level string
}

type Runtime interface {
	Config() map[string]any
	Log(level, eventName, message string, data map[string]any)
	Notice(ctx context.Context, in NoticeInput) error
	Publish(ctx context.Context, eventType string, data map[string]any)
}

type Plugin interface {
	Manifest() Manifest
	DefaultConfig() map[string]any
	HandleEvent(ctx context.Context, rt Runtime, e Event) error
	ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error)
}

type View struct {
	Manifest
	Enabled     bool           `json:"enabled"`
	Installed   bool           `json:"installed"`
	Config      map[string]any `json:"config"`
	LastError   string         `json:"last_error,omitempty"`
	LastEventAt *time.Time     `json:"last_event_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type LogView struct {
	ID        uint           `json:"id"`
	PluginID  string         `json:"plugin_id"`
	Level     string         `json:"level"`
	Event     string         `json:"event"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type MarketIndex struct {
	Version   int            `json:"version"`
	Name      string         `json:"name,omitempty"`
	PublicKey string         `json:"public_key,omitempty"`
	Plugins   []MarketPlugin `json:"plugins"`
}

type MarketPlugin struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Author       string        `json:"author,omitempty"`
	Description  string        `json:"description,omitempty"`
	Runtime      string        `json:"runtime"`
	Archive      string        `json:"archive"`
	SHA256       string        `json:"sha256"`
	Signature    string        `json:"signature,omitempty"`
	Permissions  []string      `json:"permissions,omitempty"`
	Events       []EventSpec   `json:"events,omitempty"`
	Actions      []ActionSpec  `json:"actions,omitempty"`
	ConfigSchema []ConfigField `json:"config_schema,omitempty"`
	Installed    bool          `json:"installed"`
}

type InstallResult struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	InstalledDir  string `json:"installed_dir"`
	RestartNeeded bool   `json:"restart_needed"`
}
