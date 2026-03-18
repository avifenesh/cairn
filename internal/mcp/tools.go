package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/avifenesh/cairn/internal/tool"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// registerTools converts all Cairn tools to MCP tools and registers them.
func registerTools(srv *mcpserver.MCPServer, reg *tool.Registry, toolCtx *tool.ToolContext) {
	for _, t := range reg.All() {
		mcpTool := cairnToolToMCP(t)
		handler := makeToolHandler(t, toolCtx)
		srv.AddTool(mcpTool, handler)
	}
}

// cairnToolToMCP converts a Cairn tool to an MCP tool definition.
// Uses NewToolWithRawSchema since Cairn's Schema() already returns valid JSON Schema.
func cairnToolToMCP(t tool.Tool) mcp.Tool {
	return mcp.NewToolWithRawSchema(t.Name(), t.Description(), t.Schema())
}

// makeToolHandler creates an MCP tool handler that delegates to a Cairn tool.
func makeToolHandler(t tool.Tool, toolCtx *tool.ToolContext) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Convert MCP arguments to JSON for Cairn tool.
		args, err := json.Marshal(request.GetArguments())
		if err != nil {
			return nil, fmt.Errorf("marshal arguments: %w", err)
		}

		result, err := t.Execute(toolCtx, args)
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", t.Name(), err)
		}

		return cairnResultToMCP(result), nil
	}
}

// cairnResultToMCP converts a Cairn ToolResult to an MCP CallToolResult.
func cairnResultToMCP(tr *tool.ToolResult) *mcp.CallToolResult {
	if tr.Error != "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewTextContent(tr.Error)},
			IsError: true,
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(tr.Output)},
	}
}
