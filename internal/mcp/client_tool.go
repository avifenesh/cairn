package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
	"github.com/mark3labs/mcp-go/mcp"
)

// mcpCaller abstracts the mcp-go client's CallTool method for testing.
type mcpCaller interface {
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// mcpClientTool wraps an external MCP server's tool as a Cairn tool.Tool.
type mcpClientTool struct {
	name       string          // "mcp.<server>.<tool>"
	desc       string          // description from MCP server
	schema     json.RawMessage // inputSchema from MCP ListTools
	modes      []tool.Mode     // all modes by default
	toolName   string          // original tool name on the MCP server
	serverName string
	caller     mcpCaller
}

func (t *mcpClientTool) Name() string            { return t.name }
func (t *mcpClientTool) Description() string     { return t.desc }
func (t *mcpClientTool) Schema() json.RawMessage { return t.schema }
func (t *mcpClientTool) Modes() []tool.Mode      { return t.modes }

func (t *mcpClientTool) Execute(ctx *tool.ToolContext, args json.RawMessage) (*tool.ToolResult, error) {
	// Permission gate: external MCP tools are evaluated through the same
	// permission engine as built-in tools. The tool name (mcp.<server>.<tool>)
	// can be matched by wildcard permission rules (e.g., deny "mcp.untrusted.*").
	if ctx.Permissions != nil {
		action := ctx.Permissions.Evaluate(t.name, "")
		if action == tool.Deny {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied for external tool %s", t.name)}, nil
		}
	}

	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return nil, fmt.Errorf("invalid arguments for mcp tool %s: %w", t.name, err)
		}
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = t.toolName
	req.Params.Arguments = arguments

	result, err := t.caller.CallTool(ctx.Cancel, req)
	if err != nil {
		return nil, fmt.Errorf("mcp tool %s: %w", t.name, err)
	}

	return mcpResultToCairn(result), nil
}

// mcpResultToCairn converts an MCP CallToolResult to a Cairn ToolResult.
func mcpResultToCairn(result *mcp.CallToolResult) *tool.ToolResult {
	if result == nil {
		return &tool.ToolResult{Output: ""}
	}

	var parts []string
	for _, c := range result.Content {
		text := mcp.GetTextFromContent(c)
		if text != "" {
			parts = append(parts, text)
		}
	}
	output := strings.Join(parts, "\n")

	if result.IsError {
		return &tool.ToolResult{Error: output}
	}
	return &tool.ToolResult{Output: output}
}

// wrapMCPTools converts MCP tool definitions into Cairn tool.Tool instances.
func wrapMCPTools(serverName string, mcpTools []mcp.Tool, caller mcpCaller) []tool.Tool {
	allModes := []tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding}
	tools := make([]tool.Tool, 0, len(mcpTools))

	for _, mt := range mcpTools {
		schema := mcpInputSchemaToJSON(mt)
		name := fmt.Sprintf("mcp.%s.%s", serverName, mt.Name)

		tools = append(tools, &mcpClientTool{
			name:       name,
			desc:       fmt.Sprintf("[MCP:%s] %s", serverName, mt.Description),
			schema:     schema,
			modes:      allModes,
			toolName:   mt.Name,
			serverName: serverName,
			caller:     caller,
		})
	}
	return tools
}

// mcpInputSchemaToJSON converts an MCP tool's InputSchema to json.RawMessage.
func mcpInputSchemaToJSON(mt mcp.Tool) json.RawMessage {
	// Prefer RawInputSchema if set (allows arbitrary JSON Schema).
	if len(mt.RawInputSchema) > 0 {
		return mt.RawInputSchema
	}
	// Marshal the structured InputSchema.
	raw, err := json.Marshal(mt.InputSchema)
	if err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return raw
}
