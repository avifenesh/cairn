package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
)

// MemoryService is the interface tools use to interact with the memory system.
type MemoryService interface {
	Create(ctx context.Context, m *MemoryItem) error
	Search(ctx context.Context, query string, limit int) ([]MemorySearchResult, error)
	Get(ctx context.Context, id string) (*MemoryItem, error)
	Accept(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// MemoryItem is a tool-level representation of a memory (avoids import cycle).
type MemoryItem struct {
	ID         string
	Content    string
	Category   string
	Scope      string
	Status     string
	Confidence float64
	Source     string
}

// MemorySearchResult pairs a memory with its relevance score.
type MemorySearchResult struct {
	Memory *MemoryItem
	Score  float64
}

// EventService is the interface tools use to interact with the signal plane.
type EventService interface {
	List(ctx context.Context, f EventFilter) ([]*StoredEvent, error)
	Count(ctx context.Context, f EventFilter) (int, error)
	CountBySource(ctx context.Context) (map[string]int, error)
	CountArchivedBySource(ctx context.Context) (map[string]int, error)
	MarkRead(ctx context.Context, id string) error
	MarkAllRead(ctx context.Context) (int, error)
	Archive(ctx context.Context, id string) error
	DeleteByID(ctx context.Context, id string) error
	Ingest(ctx context.Context, events []*IngestEvent) ([]*IngestEvent, error)
}

// IngestEvent is a tool-level event to insert into the signal plane.
type IngestEvent struct {
	Source     string
	SourceID   string
	Kind       string
	Title      string
	Body       string
	Actor      string
	OccurredAt time.Time
	Metadata   map[string]any
}

// EventFilter controls which events to list.
type EventFilter struct {
	Source          string
	Kind            string
	UnreadOnly      bool
	ExcludeArchived bool
	Limit           int
	Before          string // cursor: events before this ID (for pagination)
}

// StoredEvent is a tool-level representation of a signal event.
type StoredEvent struct {
	ID         string
	Source     string
	Kind       string
	Title      string
	Body       string
	URL        string
	Actor      string
	GroupKey   string
	Metadata   map[string]any
	CreatedAt  time.Time
	ReadAt     *time.Time
	ArchivedAt *time.Time
}

// DigestService generates feed digests.
type DigestService interface {
	Generate(ctx context.Context) (*DigestResult, error)
}

// DigestResult holds a generated summary.
type DigestResult struct {
	Summary    string
	Highlights []string
	EventCount int
}

// JournalService queries the session journal.
type JournalService interface {
	Recent(ctx context.Context, dur time.Duration) ([]*JournalEntry, error)
}

// JournalEntry is a tool-level representation of a journal entry.
type JournalEntry struct {
	ID        string
	Summary   string
	Decisions []string
	Errors    []string
	Learnings []string
	Mode      string
	CreatedAt time.Time
}

// TaskService manages tasks.
type TaskService interface {
	Submit(ctx context.Context, req *TaskSubmitRequest) (*TaskItem, error)
	List(ctx context.Context, status, taskType string, limit int) ([]*TaskItem, error)
	Complete(ctx context.Context, id string, output string) error
}

// TaskSubmitRequest holds parameters for creating a task.
type TaskSubmitRequest struct {
	Description string
	Type        string
	Priority    int
}

// TaskItem is a tool-level representation of a task.
type TaskItem struct {
	ID          string
	Type        string
	Status      string
	Description string
	Priority    int
	Error       string
	CreatedAt   time.Time
}

// StatusService aggregates system status info.
type StatusService interface {
	GetStatus(ctx context.Context) (*SystemStatus, error)
}

// SystemStatus holds aggregated system state.
type SystemStatus struct {
	Uptime       string
	ActiveTasks  int
	UnreadEvents int
	MemoryCount  int
	PollerStatus []PollerInfo
}

// PollerInfo describes a signal poller's state.
type PollerInfo struct {
	Source string
	Active bool
}

// NotifyService sends priority-routed notifications to channels.
type NotifyService interface {
	Notify(ctx context.Context, text string, priority int)
	FlushDigest(ctx context.Context) int
	DigestLen() int
}

// CronService manages scheduled recurring tasks.
type CronService interface {
	Create(ctx context.Context, name, schedule, instruction string, priority int) (string, error)
	List(ctx context.Context) ([]CronJobInfo, error)
	Delete(ctx context.Context, idOrName string) error
}

// CronJobInfo is a tool-level representation of a cron job.
type CronJobInfo struct {
	ID          string
	Name        string
	Schedule    string
	Instruction string
	Timezone    string
	Enabled     bool
	Priority    int
	LastRun     *time.Time
	NextRun     *time.Time
}

// ConfigService provides agent access to runtime config (behind approval gate).
type ConfigService interface {
	// PatchConfig applies partial config updates. Returns the updated config summary.
	PatchConfig(ctx context.Context, changes map[string]any) (map[string]any, error)
	// GetConfig returns the current patchable config as a map.
	GetConfig(ctx context.Context) (map[string]any, error)
}

// SkillService provides access to the skill system.
type SkillService interface {
	Get(name string) *SkillItem
	List() []*SkillItem
	Create(name, description, content, inclusion string, allowedTools []string) error
	Update(name, description, content, inclusion string, allowedTools []string) error
	Delete(name string) error
	InstallDir() string // Returns the user-override directory for marketplace installs.
	Refresh() error     // Re-discovers skills after install or external change.
}

// SkillItem is a tool-level representation of a skill.
type SkillItem struct {
	Name         string
	Description  string
	Inclusion    string // "always" or "on-demand"
	Content      string
	AllowedTools []string // Tool scoping from frontmatter (nil or empty = no restriction)
	Location     string   // Directory path on disk
	DisableModel bool     // Requires approval before activation
}

// Mode represents the agent interaction mode that determines which tools are available.
type Mode string

const (
	ModeTalk   Mode = "talk"
	ModeWork   Mode = "work"
	ModeCoding Mode = "coding"
)

// Tool is the interface every tool must implement.
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Modes() []Mode
	Execute(ctx *ToolContext, args json.RawMessage) (*ToolResult, error)
}

// ToolContext carries session state and dependencies into tool execution.
type ToolContext struct {
	SessionID   string
	TaskID      string
	AgentMode   Mode
	WorkDir     string // Worktree path for coding tasks
	Permissions *PermissionSet
	Bus         *eventbus.Bus
	Cancel      context.Context

	// Service dependencies for non-filesystem tools.
	Memories MemoryService
	Events   EventService
	Digest   DigestService
	Journal  JournalService
	Tasks    TaskService
	Status   StatusService
	Skills   SkillService
	Notifier NotifyService
	Crons    CronService

	Config ConfigService

	// ActivateSkill is called by cairn.loadSkill to register a skill in the session.
	// Set by the ReAct loop before tool execution. Nil = activation not supported.
	ActivateSkill func(name, content string, allowedTools []string)
}

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	Output      string         `json:"output"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Attachments []Attachment   `json:"attachments,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// Attachment is a named binary blob returned by a tool.
type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Data        []byte `json:"data"`
}

// Define creates a Tool from a typed function, generating JSON Schema from the
// parameter struct's tags at definition time. The struct P must have exported
// fields with `json` tags. Use `desc` tags for descriptions. Non-pointer fields
// are treated as required.
func Define[P any](name, desc string, modes []Mode, fn func(ctx *ToolContext, params P) (*ToolResult, error)) Tool {
	schema := generateSchema[P]()
	return &definedTool[P]{
		name:   name,
		desc:   desc,
		modes:  modes,
		schema: schema,
		fn:     fn,
	}
}

type definedTool[P any] struct {
	name   string
	desc   string
	modes  []Mode
	schema json.RawMessage
	fn     func(ctx *ToolContext, params P) (*ToolResult, error)
}

func (t *definedTool[P]) Name() string            { return t.name }
func (t *definedTool[P]) Description() string     { return t.desc }
func (t *definedTool[P]) Schema() json.RawMessage { return t.schema }
func (t *definedTool[P]) Modes() []Mode           { return t.modes }

func (t *definedTool[P]) Execute(ctx *ToolContext, args json.RawMessage) (*ToolResult, error) {
	var params P
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid parameters for %s: %w", t.name, err)
		}
	}
	return t.fn(ctx, params)
}

// generateSchema builds a JSON Schema object from the struct tags on P.
// Supports: string, int, float64, bool, []string. Uses `json` for field names,
// `desc` for descriptions. Non-pointer fields are required.
func generateSchema[P any]() json.RawMessage {
	var zero P
	t := reflect.TypeOf(zero)
	if t == nil {
		// P is an interface type with nil zero value — return empty schema.
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		// Non-struct params get an empty object schema.
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	properties := map[string]any{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		fieldName := strings.Split(jsonTag, ",")[0]
		if fieldName == "" {
			fieldName = field.Name
		}

		prop := map[string]any{}

		// Determine the underlying type (unwrap pointer).
		ft := field.Type
		isPtr := ft.Kind() == reflect.Ptr
		if isPtr {
			ft = ft.Elem()
		}

		switch ft.Kind() {
		case reflect.String:
			prop["type"] = "string"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			prop["type"] = "integer"
		case reflect.Float32, reflect.Float64:
			prop["type"] = "number"
		case reflect.Bool:
			prop["type"] = "boolean"
		case reflect.Slice:
			if ft.Elem().Kind() == reflect.String {
				prop["type"] = "array"
				prop["items"] = map[string]any{"type": "string"}
			} else {
				prop["type"] = "array"
			}
		default:
			prop["type"] = "string" // fallback
		}

		if desc := field.Tag.Get("desc"); desc != "" {
			prop["description"] = desc
		}

		properties[fieldName] = prop

		// Non-pointer fields are required.
		if !isPtr {
			required = append(required, fieldName)
		}
	}

	// Sort required for deterministic output.
	sort.Strings(required)

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		// Fallback to empty object schema if marshal fails.
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return raw
}
