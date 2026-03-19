package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
	"github.com/avifenesh/cairn/internal/tool/builtin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestCairnToolToMCP(t *testing.T) {
	cairnTool := tool.Define("test.echo", "Echo input back", []tool.Mode{tool.ModeTalk},
		func(ctx *tool.ToolContext, p struct {
			Message string `json:"message" desc:"Message to echo"`
		}) (*tool.ToolResult, error) {
			return &tool.ToolResult{Output: p.Message}, nil
		},
	)

	mcpTool := cairnToolToMCP(cairnTool)
	if mcpTool.Name != "test.echo" {
		t.Fatalf("expected name test.echo, got %s", mcpTool.Name)
	}
	if mcpTool.Description != "Echo input back" {
		t.Fatalf("expected description, got %s", mcpTool.Description)
	}
}

func TestCairnResultToMCP_Success(t *testing.T) {
	tr := &tool.ToolResult{Output: "hello world"}
	result := cairnResultToMCP(tr)
	if result.IsError {
		t.Fatal("expected non-error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "hello world" {
		t.Fatalf("expected 'hello world', got %q", tc.Text)
	}
}

func TestCairnResultToMCP_Error(t *testing.T) {
	tr := &tool.ToolResult{Error: "something failed"}
	result := cairnResultToMCP(tr)
	if !result.IsError {
		t.Fatal("expected error result")
	}
	tc := result.Content[0].(mcp.TextContent)
	if tc.Text != "something failed" {
		t.Fatalf("expected error message, got %q", tc.Text)
	}
}

func TestRegisterAllTools(t *testing.T) {
	srv := mcpserver.NewMCPServer("test", "1.0.0")
	reg := tool.NewRegistry()
	reg.Register(builtin.All()...)

	toolCtx := &tool.ToolContext{Cancel: context.Background()}
	registerTools(srv, reg, toolCtx)

	tools := srv.ListTools()
	if len(tools) != 27 {
		t.Fatalf("expected 27 registered MCP tools, got %d", len(tools))
	}

	// Verify a specific tool exists.
	if _, ok := tools["cairn.webSearch"]; !ok {
		t.Fatal("expected cairn.webSearch to be registered")
	}
	if _, ok := tools["cairn.readFile"]; !ok {
		t.Fatal("expected cairn.readFile to be registered")
	}
}

func TestMakeToolHandler(t *testing.T) {
	cairnTool := tool.Define("test.add", "Add numbers", []tool.Mode{tool.ModeTalk},
		func(ctx *tool.ToolContext, p struct {
			A int `json:"a"`
			B int `json:"b"`
		}) (*tool.ToolResult, error) {
			return &tool.ToolResult{
				Output:   "result",
				Metadata: map[string]any{"sum": p.A + p.B},
			}, nil
		},
	)

	toolCtx := &tool.ToolContext{Cancel: context.Background()}
	handler := makeToolHandler(cairnTool, toolCtx)

	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "test.add",
			Arguments: json.RawMessage(`{"a":2,"b":3}`),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}
