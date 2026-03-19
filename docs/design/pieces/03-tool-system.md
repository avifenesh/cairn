# Piece 3: Tool System

> Type-safe tools with registry, mode-based filtering, permissions, and MCP integration.

## Interface

```go
// Gollem-inspired typed tool definition
type Tool interface {
    Name() string
    Description() string
    Schema() json.RawMessage // JSON Schema for parameters
    Modes() []Mode           // Which agent modes can use this tool
    Execute(ctx *ToolContext, args json.RawMessage) (*ToolResult, error)
}

// Compile-time typed tool helper
func Define[P any](name, desc string, modes []Mode, fn func(ctx *ToolContext, params P) (*ToolResult, error)) Tool

type ToolContext struct {
    SessionID   string
    TaskID      string
    AgentMode   Mode
    WorkDir     string          // Worktree path for coding tasks
    Permissions *PermissionSet
    Bus         *eventbus.Bus
    Cancel      context.Context
}

type ToolResult struct {
    Output      string
    Metadata    map[string]any
    Attachments []Attachment
    Error       string
}

type Mode string
const (
    ModeTalk   Mode = "talk"
    ModeWork   Mode = "work"
    ModeCoding Mode = "coding"
)
```

## Registry

```go
type Registry struct {
    tools    map[string]Tool
    mcpTools map[string]*MCPTool // Discovered from MCP servers
}

func (r *Registry) Register(tool Tool)
func (r *Registry) ForMode(mode Mode) []Tool              // Filter by mode
func (r *Registry) ForLLM(mode Mode) []llm.ToolDef        // Convert to LLM format
func (r *Registry) Execute(ctx *ToolContext, name string, args json.RawMessage) (*ToolResult, error)
```

## Permission Gating

```go
// OpenCode-inspired wildcard permission system
type PermissionRule struct {
    Tool    string // tool name or "*"
    Pattern string // file glob or "*"
    Action  PermissionAction // allow, ask, deny
}

type PermissionAction string
const (
    Allow PermissionAction = "allow"
    Ask   PermissionAction = "ask"
    Deny  PermissionAction = "deny"
)

func EvaluatePermission(tool, pattern string, rules []PermissionRule) PermissionAction
```

## Built-in Tools (Phase 1)

| Tool | Modes | Permission | Description |
|------|-------|------------|-------------|
| `cairn.readFile` | all | allow | Read file contents |
| `cairn.writeFile` | work, coding | ask(*.env,*.key) | Write file |
| `cairn.editFile` | work, coding | ask(*.env,*.key) | Patch file with search/replace |
| `cairn.deleteFile` | coding | ask | Delete file |
| `cairn.listFiles` | all | allow | List directory |
| `cairn.searchFiles` | all | allow | Grep/ripgrep search |
| `cairn.shell` | work, coding | ask | Execute shell command (deny patterns, env filter, output cap) |
| `cairn.gitRun` | work, coding | allow | Git operations (protected branch check) |
| `cairn.webSearch` | all | allow | Search via SearXNG/Z.ai |
| `cairn.webFetch` | all | allow | Fetch URL content |
| `cairn.createMemory` | all | allow | Store a memory |
| `cairn.searchMemory` | all | allow | RAG memory search |
| `cairn.compose` | work, coding | allow | Create feed event |
| `system.status` | all | allow | System health check |

## MCP Integration

```go
// Uses mcp-go for client-side MCP
type MCPToolset struct {
    client  mcpclient.Client
    tools   []Tool
    session string
}

func NewMCPToolset(transport mcptransport.Interface) (*MCPToolset, error)
func (m *MCPToolset) Tools() []Tool   // Discovered MCP tools as native Tool interface
func (m *MCPToolset) Close() error
```

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 3.1 | Tool interface + Define[P] helper | Nothing |
| 3.2 | Registry with mode filtering | 3.1 |
| 3.3 | Permission engine (wildcard rules) | 3.1 |
| 3.4 | Built-in tools (file, shell, git) | 3.1, 3.2, 3.3 |
| 3.5 | MCP toolset adapter (via mcp-go) | 3.1, 3.2 |
| 3.6 | Tool result formatting + truncation | 3.1 |
| 3.7 | Tests | All |
