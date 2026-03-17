package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/avifenesh/cairn/internal/eventbus"
)

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
func (t *definedTool[P]) Description() string      { return t.desc }
func (t *definedTool[P]) Schema() json.RawMessage  { return t.schema }
func (t *definedTool[P]) Modes() []Mode            { return t.modes }

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

	raw, _ := json.Marshal(schema)
	return raw
}
