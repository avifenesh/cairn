package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type addParams struct {
	A     int     `json:"a" desc:"First number"`
	B     int     `json:"b" desc:"Second number"`
	Label *string `json:"label,omitempty" desc:"Optional label"`
}

func TestDefine(t *testing.T) {
	add := Define("math.add", "Add two numbers", []Mode{ModeTalk}, func(ctx *ToolContext, p addParams) (*ToolResult, error) {
		return &ToolResult{
			Output: "result",
			Metadata: map[string]any{
				"sum": p.A + p.B,
			},
		}, nil
	})

	if add.Name() != "math.add" {
		t.Fatalf("expected name %q, got %q", "math.add", add.Name())
	}
	if add.Description() != "Add two numbers" {
		t.Fatalf("expected description %q, got %q", "Add two numbers", add.Description())
	}
	if len(add.Modes()) != 1 || add.Modes()[0] != ModeTalk {
		t.Fatalf("expected modes [talk], got %v", add.Modes())
	}

	args := json.RawMessage(`{"a": 3, "b": 5}`)
	ctx := &ToolContext{Cancel: context.Background()}
	result, err := add.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "result" {
		t.Fatalf("expected output %q, got %q", "result", result.Output)
	}
	sum, ok := result.Metadata["sum"]
	if !ok {
		t.Fatal("expected metadata key 'sum'")
	}
	if sum != 8 {
		t.Fatalf("expected sum 8, got %v", sum)
	}
}

type schemaTestParams struct {
	Name     string   `json:"name" desc:"A name"`
	Count    int      `json:"count" desc:"A count"`
	Score    float64  `json:"score" desc:"A score"`
	Enabled  bool     `json:"enabled" desc:"Is enabled"`
	Tags     []string `json:"tags" desc:"Tag list"`
	Optional *string  `json:"optional,omitempty" desc:"Optional field"`
}

func TestDefine_SchemaGeneration(t *testing.T) {
	tool := Define("test.schema", "test", []Mode{ModeTalk}, func(ctx *ToolContext, p schemaTestParams) (*ToolResult, error) {
		return &ToolResult{Output: "ok"}, nil
	})

	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	if schema["type"] != "object" {
		t.Fatalf("expected type 'object', got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be an object")
	}

	// Check field types.
	checks := map[string]string{
		"name":    "string",
		"count":   "integer",
		"score":   "number",
		"enabled": "boolean",
		"tags":    "array",
	}
	for field, expectedType := range checks {
		prop, ok := props[field].(map[string]any)
		if !ok {
			t.Fatalf("expected property %q to be an object", field)
		}
		if prop["type"] != expectedType {
			t.Fatalf("expected %q type %q, got %q", field, expectedType, prop["type"])
		}
	}

	// Check descriptions.
	nameProp := props["name"].(map[string]any)
	if nameProp["description"] != "A name" {
		t.Fatalf("expected description %q, got %q", "A name", nameProp["description"])
	}

	// Check required fields — should NOT include "optional" (pointer field).
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("expected required to be an array")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	for _, field := range []string{"name", "count", "score", "enabled", "tags"} {
		if !requiredSet[field] {
			t.Fatalf("expected %q to be required", field)
		}
	}
	if requiredSet["optional"] {
		t.Fatal("pointer field 'optional' should not be required")
	}
}

func TestRegistry_ForMode(t *testing.T) {
	reg := NewRegistry()

	talkOnly := Define("t1", "talk only", []Mode{ModeTalk}, func(ctx *ToolContext, p struct{}) (*ToolResult, error) {
		return &ToolResult{Output: "t1"}, nil
	})
	workCoding := Define("t2", "work+coding", []Mode{ModeWork, ModeCoding}, func(ctx *ToolContext, p struct{}) (*ToolResult, error) {
		return &ToolResult{Output: "t2"}, nil
	})
	allModes := Define("t3", "all modes", []Mode{ModeTalk, ModeWork, ModeCoding}, func(ctx *ToolContext, p struct{}) (*ToolResult, error) {
		return &ToolResult{Output: "t3"}, nil
	})

	reg.Register(talkOnly, workCoding, allModes)

	talkTools := reg.ForMode(ModeTalk)
	if len(talkTools) != 2 {
		t.Fatalf("expected 2 talk tools, got %d", len(talkTools))
	}

	workTools := reg.ForMode(ModeWork)
	if len(workTools) != 2 {
		t.Fatalf("expected 2 work tools, got %d", len(workTools))
	}

	codingTools := reg.ForMode(ModeCoding)
	if len(codingTools) != 2 {
		t.Fatalf("expected 2 coding tools, got %d", len(codingTools))
	}
}

func TestRegistry_ForLLM(t *testing.T) {
	reg := NewRegistry()

	tool := Define("test.tool", "a test tool", []Mode{ModeTalk}, func(ctx *ToolContext, p addParams) (*ToolResult, error) {
		return &ToolResult{Output: "ok"}, nil
	})
	reg.Register(tool)

	defs := reg.ForLLM(ModeTalk)
	if len(defs) != 1 {
		t.Fatalf("expected 1 LLM tool def, got %d", len(defs))
	}

	def := defs[0]
	if def.Name != "test.tool" {
		t.Fatalf("expected name %q, got %q", "test.tool", def.Name)
	}
	if def.Description != "a test tool" {
		t.Fatalf("expected description %q, got %q", "a test tool", def.Description)
	}
	if len(def.Parameters) == 0 {
		t.Fatal("expected non-empty parameters schema")
	}

	// Verify it's valid JSON.
	var schema map[string]any
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("parameters is not valid JSON: %v", err)
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()

	tool := Define("test.echo", "echo", []Mode{ModeTalk}, func(ctx *ToolContext, p struct {
		Msg string `json:"msg"`
	}) (*ToolResult, error) {
		return &ToolResult{Output: p.Msg}, nil
	})
	reg.Register(tool)

	ctx := &ToolContext{Cancel: context.Background()}
	result, err := reg.Execute(ctx, "test.echo", json.RawMessage(`{"msg":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "hello" {
		t.Fatalf("expected output %q, got %q", "hello", result.Output)
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	reg := NewRegistry()

	ctx := &ToolContext{Cancel: context.Background()}
	_, err := reg.Execute(ctx, "nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if err.Error() != "unknown tool: nonexistent" {
		t.Fatalf("unexpected error message: %v", err)
	}
}
