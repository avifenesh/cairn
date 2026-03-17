# Piece 8: Plugin & Skill System

> Lifecycle hooks, skill discovery, ClawHub-compatible, extensible.

## Skill System (OpenClaw-compatible)

```go
type Skill struct {
    Name          string
    Description   string
    Inclusion     Inclusion       // always, on-demand
    AllowedTools  []string        // scoped tool access
    Content       string          // SKILL.md body (loaded on demand)
    Location      string          // file path
    DisableModel  bool            // side-effect skills require approval
    Metadata      map[string]any  // OpenClaw metadata (requires, install, etc.)
}

type Inclusion string
const (
    Always   Inclusion = "always"    // always in system prompt
    OnDemand Inclusion = "on-demand" // loaded when triggered by description match
)

type SkillService struct {
    skills  map[string]*Skill
    dirs    []string           // discovery directories
    watcher *fsnotify.Watcher  // hot-reload on change
}

func (s *SkillService) Discover() error  // scan directories, parse SKILL.md frontmatter
func (s *SkillService) Get(name string) *Skill
func (s *SkillService) ForAgent(mode tool.Mode) []*Skill
func (s *SkillService) Watch() error     // re-discover on file changes
```

## Discovery Directories (multi-source, OpenClaw + Claude Code compatible)

```
Priority (high to low):
1. {workspace}/.pub/skills/       — project-specific
2. {workspace}/.claude/skills/    — Claude Code compatibility
3. {workspace}/.agents/skills/    — OpenCode/agent compatibility
4. ~/.pub/skills/                 — user global
5. {binary}/skills/               — bundled with binary
```

## Skill Marketplace Integration

```go
// ClawHub-compatible skill installation
type SkillRegistry interface {
    Search(query string) ([]*SkillListing, error)
    Install(slug string, version string) error
    Update(slug string) error
    List() ([]*InstalledSkill, error)
}

// Phase 2: implement ClawHub client
// Phase 3: Pub's own registry (compatible format)
```

## Plugin System (ADK-Go + OpenCode inspired hooks)

```go
type Plugin interface {
    Name() string
    Init(ctx *PluginContext) error
    Hooks() *Hooks
    Close() error
}

type PluginContext struct {
    Bus    *eventbus.Bus
    Config map[string]any
    Logger *slog.Logger
}

type Hooks struct {
    // Agent lifecycle
    BeforeAgentRun  func(ctx *HookContext) error
    AfterAgentRun   func(ctx *HookContext) error

    // LLM
    BeforeModelCall func(ctx *HookContext, req *llm.Request) (*llm.Request, error)
    AfterModelCall  func(ctx *HookContext, resp *llm.Event) error
    OnModelError    func(ctx *HookContext, err error) error

    // Tools
    BeforeToolExec  func(ctx *HookContext, tool string, args json.RawMessage) (json.RawMessage, error)
    AfterToolExec   func(ctx *HookContext, tool string, result *tool.ToolResult) error
    OnToolError     func(ctx *HookContext, tool string, err error) error

    // Permissions
    OnPermissionAsk func(ctx *HookContext, req *PermissionRequest) (*PermissionAction, error)

    // Events
    OnEvent         func(event *agent.Event) error

    // Custom tools
    Tools           map[string]tool.Tool  // plugin-provided tools
}
```

## Built-in Plugins

| Plugin | Purpose |
|--------|---------|
| `logging` | Log all agent events at debug level |
| `budget` | Enforce daily/weekly LLM spend limits |
| `permissions` | Default permission rules |
| `metrics` | Prometheus-compatible metrics |
| `journal` | Session journaling after completion |
| `reflection` | Periodic pattern detection |
| `push-notify` | Telegram/Gotify notifications |

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 8.1 | Skill types + SKILL.md parser (YAML frontmatter) | Nothing |
| 8.2 | Skill discovery (multi-directory scan) | 8.1 |
| 8.3 | Skill hot-reload (fsnotify) | 8.1, 8.2 |
| 8.4 | Skill injection into system prompt | 8.1, 4 (agent) |
| 8.5 | Plugin interface + hook system | 1 (event bus) |
| 8.6 | Built-in plugins (logging, budget, journal) | 8.5 |
| 8.7 | Plugin loading from config | 8.5 |
| 8.8 | ClawHub client (skill marketplace) | 8.1, 8.2 |
| 8.9 | Tests | All |
