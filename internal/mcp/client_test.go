package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/tool"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockCaller implements mcpCaller for testing.
type mockCaller struct {
	result *mcp.CallToolResult
	err    error
	called string
	args   map[string]any
}

func (m *mockCaller) CallTool(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m.called = req.Params.Name
	if argMap, ok := req.Params.Arguments.(map[string]any); ok {
		m.args = argMap
	}
	return m.result, m.err
}

func TestWrapMCPTools(t *testing.T) {
	mcpTools := []mcp.Tool{
		{
			Name:        "read_file",
			Description: "Read a file from the filesystem",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{"type": "string", "description": "File path"},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file",
		},
	}

	caller := &mockCaller{}
	tools := wrapMCPTools("filesystem", mcpTools, caller)

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Check naming.
	if tools[0].Name() != "mcp.filesystem.read_file" {
		t.Errorf("unexpected name: %s", tools[0].Name())
	}
	if tools[1].Name() != "mcp.filesystem.write_file" {
		t.Errorf("unexpected name: %s", tools[1].Name())
	}

	// Check description prefix.
	if tools[0].Description() != "[MCP:filesystem] Read a file from the filesystem" {
		t.Errorf("unexpected description: %s", tools[0].Description())
	}

	// Check modes (all modes).
	modes := tools[0].Modes()
	if len(modes) != 3 {
		t.Errorf("expected 3 modes, got %d", len(modes))
	}

	// Check schema is valid JSON.
	var schema map[string]any
	if err := json.Unmarshal(tools[0].Schema(), &schema); err != nil {
		t.Fatalf("invalid schema JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected type=object, got %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not a map")
	}
	if _, ok := props["path"]; !ok {
		t.Error("missing 'path' property")
	}
}

func TestMCPClientToolExecute(t *testing.T) {
	caller := &mockCaller{
		result: &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewTextContent("hello world")},
		},
	}

	wrapped := wrapMCPTools("test", []mcp.Tool{
		{Name: "greet", Description: "Greet someone"},
	}, caller)

	ctx := &tool.ToolContext{Cancel: context.Background()}
	args := json.RawMessage(`{"name":"cairn"}`)

	result, err := wrapped[0].Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Output)
	}
	if caller.called != "greet" {
		t.Errorf("expected tool call 'greet', got %q", caller.called)
	}
	if caller.args["name"] != "cairn" {
		t.Errorf("expected arg name=cairn, got %v", caller.args["name"])
	}
}

func TestMCPResultToCairn_TextContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent("output text")},
	}
	tr := mcpResultToCairn(result)
	if tr.Output != "output text" {
		t.Errorf("expected 'output text', got %q", tr.Output)
	}
	if tr.Error != "" {
		t.Errorf("unexpected error: %q", tr.Error)
	}
}

func TestMCPResultToCairn_MultipleContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent("line 1"),
			mcp.NewTextContent("line 2"),
		},
	}
	tr := mcpResultToCairn(result)
	if tr.Output != "line 1\nline 2" {
		t.Errorf("expected 'line 1\\nline 2', got %q", tr.Output)
	}
}

func TestMCPResultToCairn_Error(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent("something failed")},
		IsError: true,
	}
	tr := mcpResultToCairn(result)
	if tr.Error != "something failed" {
		t.Errorf("expected error 'something failed', got %q", tr.Error)
	}
	if tr.Output != "" {
		t.Errorf("expected empty output, got %q", tr.Output)
	}
}

func TestMCPResultToCairn_Nil(t *testing.T) {
	tr := mcpResultToCairn(nil)
	if tr.Output != "" {
		t.Errorf("expected empty output for nil result, got %q", tr.Output)
	}
}

func TestMCPClientToolExecute_CallerError(t *testing.T) {
	caller := &mockCaller{
		err: fmt.Errorf("connection lost"),
	}

	wrapped := wrapMCPTools("broken", []mcp.Tool{
		{Name: "fail", Description: "Always fails"},
	}, caller)

	ctx := &tool.ToolContext{Cancel: context.Background()}
	_, err := wrapped[0].Execute(ctx, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "mcp tool mcp.broken.fail: connection lost" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClientManagerStatus_Empty(t *testing.T) {
	reg := tool.NewRegistry()
	bus := eventbus.New()
	mgr := NewClientManager(reg, bus, nil)

	statuses := mgr.Status()
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestClientManagerConnect_InvalidTransport(t *testing.T) {
	reg := tool.NewRegistry()
	bus := eventbus.New()
	mgr := NewClientManager(reg, bus, nil)

	err := mgr.Connect(context.Background(), MCPServerConfig{
		Name:      "test",
		Transport: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
}

func TestClientManagerConnect_EmptyName(t *testing.T) {
	reg := tool.NewRegistry()
	bus := eventbus.New()
	mgr := NewClientManager(reg, bus, nil)

	err := mgr.Connect(context.Background(), MCPServerConfig{
		Transport: "stdio",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestClientManagerDisconnect_NotFound(t *testing.T) {
	reg := tool.NewRegistry()
	bus := eventbus.New()
	mgr := NewClientManager(reg, bus, nil)

	err := mgr.Disconnect("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestRegistryDeregister(t *testing.T) {
	reg := tool.NewRegistry()
	dummy := &mcpClientTool{
		name:  "mcp.test.dummy",
		desc:  "test",
		modes: []tool.Mode{tool.ModeTalk},
	}
	reg.Register(dummy)

	if _, ok := reg.Get("mcp.test.dummy"); !ok {
		t.Fatal("tool should be registered")
	}

	removed := reg.Deregister("mcp.test.dummy")
	if !removed {
		t.Error("Deregister should return true for existing tool")
	}

	if _, ok := reg.Get("mcp.test.dummy"); ok {
		t.Error("tool should be deregistered")
	}

	removed = reg.Deregister("mcp.test.dummy")
	if removed {
		t.Error("Deregister should return false for non-existing tool")
	}
}

func TestParseServerConfigs(t *testing.T) {
	raw := json.RawMessage(`[{"name":"fs","transport":"stdio","command":"npx","args":["@mcp/server-fs"],"enabled":true}]`)
	configs, err := ParseServerConfigs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Name != "fs" {
		t.Errorf("expected name=fs, got %q", configs[0].Name)
	}
	if configs[0].Command != "npx" {
		t.Errorf("expected command=npx, got %q", configs[0].Command)
	}
}

func TestParseServerConfigs_Empty(t *testing.T) {
	configs, err := ParseServerConfigs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if configs != nil {
		t.Errorf("expected nil configs, got %v", configs)
	}
}

func TestMCPInputSchemaToJSON(t *testing.T) {
	mt := mcp.Tool{
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}
	raw := mcpInputSchemaToJSON(mt)
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected type=object, got %v", schema["type"])
	}
}
