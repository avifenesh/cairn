# Piece 4: Agent Core

> Agent interface, ReAct loop, session management, state machine.

## Interface

```go
// ADK-Go inspired iterator-based agent interface
type Agent interface {
    Name() string
    Description() string
    Run(ctx *InvocationContext) iter.Seq2[*Event, error]
}

type ResumableAgent interface {
    Agent
    Resume(ctx *InvocationContext, checkpoint *Checkpoint) iter.Seq2[*Event, error]
}

type InvocationContext struct {
    Context     context.Context
    SessionID   string
    UserMessage string
    Mode        tool.Mode
    Session     *Session
    Tools       *tool.Registry
    LLM         llm.Client
    Memory      *memory.Service
    Bus         *eventbus.Bus
    Config      *AgentConfig
}
```

## Event Model (ADK-Go inspired, event-sourced)

```go
type Event struct {
    ID        string
    SessionID string
    Timestamp time.Time
    Author    string   // agent name or "user"
    Round     int      // tool loop iteration
    Parts     []Part   // content parts
    Actions   *Actions // state mutations
}

// Part union — OpenCode MessageV2.Part pattern
type Part interface { partMarker() }

type TextPart struct { Text string }
type ToolPart struct {
    ToolName string
    CallID   string
    Status   ToolStatus // pending, running, completed, failed
    Input    json.RawMessage
    Output   string
    Error    string
    Duration time.Duration
}
type ReasoningPart struct { Text string }
type FilePart struct { Path, MimeType string; Size int64 }

type Actions struct {
    StateDelta map[string]any
    Transfer   string // transfer to sub-agent
    Interrupt  *InterruptInfo
}
```

## ReAct Loop (Eino-inspired state machine)

```go
type ReActAgent struct {
    name        string
    mode        tool.Mode
    maxRounds   int // talk: 10, work: 10, coding: 100
    systemBuild func(*InvocationContext) string
}

func (a *ReActAgent) Run(ctx *InvocationContext) iter.Seq2[*Event, error] {
    return func(yield func(*Event, error) bool) {
        messages := ctx.Session.History()
        messages = append(messages, userMessage(ctx.UserMessage))

        for round := 0; round < a.maxRounds; round++ {
            // 1. Call LLM with messages + tools
            var roundText, roundReasoning strings.Builder
            var toolCalls []llm.ToolCall

            for event := range ctx.LLM.Stream(ctx.Context, &llm.Request{
                Model:    ctx.Config.Model,
                Messages: messages,
                System:   a.systemBuild(ctx),
                Tools:    ctx.Tools.ForLLM(a.mode),
            }) {
                switch e := event.(type) {
                case llm.TextDelta:
                    roundText.WriteString(e.Text)
                    if !yield(&Event{Parts: []Part{TextPart{e.Text}}}, nil) { return }
                case llm.ReasoningDelta:
                    roundReasoning.WriteString(e.Text)
                case llm.ToolCall:
                    toolCalls = append(toolCalls, e)
                case llm.MessageEnd:
                    // emit accumulated reasoning
                    if r := roundReasoning.String(); r != "" {
                        yield(&Event{Parts: []Part{ReasoningPart{r}}}, nil)
                    }
                }
            }

            // 2. If no tool calls → done
            if len(toolCalls) == 0 { return }

            // 3. Execute tools (potentially parallel)
            messages = append(messages, assistantMessage(roundText.String(), toolCalls))
            for _, tc := range toolCalls {
                result := ctx.Tools.Execute(toolCtx, tc.Name, tc.Input)
                messages = append(messages, toolResultMessage(tc.ID, result))
                yield(&Event{Parts: []Part{toolPart(tc, result)}}, nil)
            }

            // 4. Continue loop with tool results
        }
    }
}
```

## Session Management

```go
type Session struct {
    ID        string
    ParentID  string   // for branching
    Title     string
    Mode      tool.Mode
    Events    []*Event // append-only history
    State     map[string]any
    CreatedAt time.Time
    UpdatedAt time.Time
}

type SessionStore interface {
    Create(ctx context.Context, session *Session) error
    Get(ctx context.Context, id string) (*Session, error)
    List(ctx context.Context, opts ListOpts) ([]*Session, error)
    AppendEvent(ctx context.Context, sessionID string, event *Event) error
    UpdateState(ctx context.Context, sessionID string, delta map[string]any) error
    Branch(ctx context.Context, parentID string) (*Session, error) // fork session
    Compact(ctx context.Context, sessionID string) error           // summarize old events
}
```

## Agent Modes

| Mode | Max Rounds | Tools | System Prompt Addendum |
|------|-----------|-------|----------------------|
| talk | 10 | read-only (search, read, web, memory) | Quick answers, conversational |
| work | 10 | operational (+ write, shell, create, deploy) | Deep work, artifacts, triage |
| coding | 100 | everything (+ file edit, git, PR) | AGENTS.md loaded, full coding workflow |

## Sub-Agents

```go
// Agents can spawn sub-agents for delegation
type SubAgentConfig struct {
    Name   string
    Mode   tool.Mode
    Prompt string
}

// Via tool: pub.spawnSubagent
// Creates a new task in the task engine, returns taskID
// Sub-agent runs in its own goroutine with its own session
```

## Subphases

| # | Subphase | Depends On | Status |
|---|----------|------------|--------|
| 4.1 | Agent interface + Event model | 1 (event bus) | Done (PR #3) |
| 4.2 | ReAct loop implementation | 2 (LLM), 3 (tools), 4.1 | Done (PR #3) |
| 4.3 | Session store (SQLite) | 4.1 | Done (PR #3) |
| 4.4 | Session compaction | 4.2, 4.3, 2 (LLM for summarization) | Deferred |
| 4.5 | Agent modes + system prompt building | 4.2 | Done (PR #3) |
| 4.6 | Sub-agent spawning | 4.2, 5 (task engine) | Deferred |
| 4.7 | Checkpoint/resume (Eino pattern) | 4.2, 4.3 | Deferred |
| 4.8 | Tests | All | Done (19 tests) |

### Phase 5 Additions (PR #10)
| # | Addition | Status |
|---|----------|--------|
| 5.3 | Always-on agent loop (tick cycle, task execution) | Done |
| 5.5 | Session journaler (episodic memory via LLM) | Done |
| 5.6 | Reflection engine (pattern detection, memory proposals) | Done |
