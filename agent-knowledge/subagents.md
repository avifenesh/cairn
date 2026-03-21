# Learning Guide: Subagent Architectures for AI Agents

**Generated**: 2026-03-21
**Sources**: 22 resources analyzed
**Depth**: deep

## Prerequisites

- Understanding of LLM agent loops (ReAct, tool-use)
- Familiarity with at least one agent framework (Claude Code, LangGraph, etc.)
- Basic understanding of event-driven architectures and SSE streaming
- Knowledge of Go concurrency patterns (for Cairn implementation)

## TL;DR

- Subagents are isolated agent instances that a parent agent spawns to handle specific tasks, then returns results
- The dominant pattern is **agent-as-tool**: subagents are wrapped as callable tools with typed input/output
- Two-level hierarchies (parent + children) significantly outperform both flat and deeper (3+) nesting
- Context isolation is critical: subagents get clean context windows, preventing bloat in the parent
- UI must show: spawn event, streaming progress, completion/failure, and allow user intervention
- Three execution modes: synchronous (blocking), asynchronous (background), and parallel (multiple concurrent)

## Core Concepts

### 1. What Are Subagents?

A subagent is an agent instance spawned by a parent agent to handle a delegated task. Unlike peer-to-peer multi-agent systems, subagents operate in a strict parent-child hierarchy:

- **Parent decides** when to spawn, what context to pass, and what tools the child gets
- **Child executes** independently in its own context window with its own tool set
- **Results flow upward** - the child returns a result to the parent, not directly to the user
- **Lifecycle is bounded** - the child exists only for the duration of its task

**Key insight**: Subagents are NOT the same as agent teams. Subagents work within a single session with shared lifecycle. Agent teams coordinate across separate sessions with independent lifecycles.

### 2. The Agent-as-Tool Pattern

The dominant abstraction across all major frameworks wraps subagents as callable tools:

```
Parent Agent
  ├── tool: "research"    → spawns ResearchSubagent
  ├── tool: "code-review" → spawns ReviewSubagent
  └── tool: "test-runner" → spawns TestSubagent
```

The LLM sees subagents as tools with descriptions. It decides when to invoke them based on the task. This is the pattern used by:
- **Claude Code**: Agent tool with `agent_type` parameter
- **LangGraph**: Subagents wrapped as `@tool` functions
- **OpenAI Agents SDK**: Handoffs registered in `handoffs` array
- **Vercel AI SDK**: `tool()` that calls `subagent.generate()` or `.stream()`
- **CrewAI**: Delegation tools generated from agent definitions
- **AutoGen**: Agents as participants in group chats

Two variants exist:
1. **Tool-per-agent**: Each subagent is a distinct tool (e.g., `research_tool`, `review_tool`)
2. **Single dispatch tool**: One parameterized `Agent(type)` tool routes to different subagents by name

### 3. Orchestration Patterns

#### Supervisor (Stateful Orchestrator)
A full agent that maintains conversation context and dynamically decides which subagents to call across multiple turns. The supervisor:
- Keeps the conversation history
- Decides when to delegate vs. handle directly
- Synthesizes results from multiple subagents
- Can re-delegate if results are insufficient

**Use when**: Complex multi-step tasks, iterative refinement needed, results depend on each other.

#### Router (Stateless Dispatcher)
A single classification step that dispatches to an agent without maintaining ongoing state. The router:
- Classifies the request once
- Hands off entirely to the selected agent
- Does not synthesize or iterate

**Use when**: Triage scenarios, FAQ routing, single-domain delegation.

#### Hierarchical Delegation
Parent spawns children who may spawn grandchildren (limited depth). Research shows two-level hierarchies (parent + children) significantly outperform:
- Flat architectures (single agent doing everything)
- Deep hierarchies (3+ levels add coordination overhead without proportional benefit)

**Critical constraint in Claude Code**: Subagents cannot spawn other subagents. If you need nested delegation, chain subagents from the parent or use skills within subagents.

### 4. Context Management

Context isolation is the primary reason subagents exist. Without it, verbose tool output (test results, code exploration, doc fetches) bloats the parent's context window.

#### Isolation Strategies

| Strategy | Framework | How It Works |
|----------|-----------|--------------|
| **Clean window** | Claude Code, Vercel AI SDK | Subagent starts fresh, receives only the task prompt |
| **Shared state graph** | LangGraph, Google ADK | Agents coordinate through a central data layer, not messages |
| **Conversation carry** | OpenAI Agents SDK | Full transcript transfers across handoff chains |
| **Selective injection** | All | Parent curates what context the child receives |

**Anthropic's approach** (from their multi-agent research system):
- Lead agents save research plans to external memory before context limits
- Fresh subagents spawn with clean contexts
- Completed work phases are summarized and stored externally
- Context stored in memory prevents loss when approaching token limits

#### What to Pass to Subagents

Good invocations include:
- Specific scope ("review auth module in src/lib/auth.ts")
- Relevant file references
- Clear success criteria
- Output format expectations

Bad invocations: "Fix authentication" (too vague, subagent wastes turns exploring).

### 5. Execution Modes

#### Synchronous (Foreground)
Parent blocks until subagent completes. Permission prompts pass through to user.
- **Use when**: Result needed before parent can continue
- **Example**: Research before implementation

#### Asynchronous (Background)
Parent continues working while subagent runs concurrently.
- **Use when**: Non-blocking research, parallel exploration
- **Example**: Security audit running while implementation continues
- **Challenge**: Permission handling - must pre-approve upfront since user isn't watching

#### Parallel (Multiple Concurrent)
Multiple subagents spawn simultaneously for independent tasks.
- **Use when**: 3+ unrelated tasks with no file overlap
- **Example**: Frontend, backend, database agents working on separate domains
- **Risk**: Over-parallelizing trivial tasks wastes tokens on coordination overhead

### 6. Communication Patterns

#### Parent-to-Child
- Via tool invocation parameters (task prompt, context)
- Via injected skills (preloaded domain knowledge)
- Via shared filesystem (files both can read)

#### Child-to-Parent
- Via tool results (final output)
- Via streaming partial results (progress updates)
- Via artifacts (generated files, data)

#### Bidirectional Handoffs (OpenAI SDK only)
Agents list each other as handoff targets, enabling circular flows. Most frameworks enforce one-way parent-to-child only.

### 7. UI Patterns for Subagent Management

Based on Vercel AI SDK, Claude Code, and generative UI research:

#### Spawn Indication
- Visual indicator when parent decides to delegate
- Show which subagent type was chosen and why
- Display the task description passed to the child

#### Progress Streaming
The Vercel AI SDK pattern using async generators:
```typescript
execute: async function* ({ task }) {
  const result = await subagent.stream({ prompt: task });
  for await (const message of readUIMessageStream({
    stream: result.toUIMessageStream(),
  })) {
    yield message;  // Each yield updates the UI
  }
}
```

Tool part states for rendering:
- `input-streaming`: Tool input being generated
- `input-available`: Tool ready to execute
- `output-available`: Tool produced output
- `output-error`: Tool execution failed

#### Dual-view Pattern
Show full subagent execution to users while the parent model sees only the summary:
```typescript
toModelOutput: ({ output }) => ({
  type: 'text',
  value: lastTextPart?.text ?? 'Task completed.'
})
```

This keeps the parent's context clean while giving users full transparency.

#### Status Cards
Per-subagent status cards showing:
- Agent name/type with color coding
- Current state (spawning, working, completing, failed)
- Elapsed time
- Tool calls made (count + last tool)
- Expandable detail view

#### User Intervention Points
- Ability to cancel a running subagent
- Ability to provide input to a stuck subagent
- Ability to retry a failed subagent
- Background-to-foreground promotion (Ctrl+B in Claude Code)

### 8. Handoff Patterns (OpenAI Agents SDK)

The handoff model treats agent-to-agent delegation as a conversation transfer:

```python
triage_agent = Agent(
    name="Triage",
    handoffs=[billing_agent, handoff(refund_agent)]
)
```

Key features:
- **Input types**: Structured metadata (reason, priority, language) generated by the LLM during handoff
- **Input filters**: Transform conversation history before the receiving agent sees it
- **On-handoff callbacks**: Execute side effects (logging, state updates) during transfer
- **Conditional availability**: `is_enabled` dynamically controls which handoffs are offered

### 9. A2A Protocol (Agent-to-Agent)

Google's A2A protocol (April 2025) standardizes inter-agent communication across frameworks:

- **Agent Cards**: JSON discovery documents (like DNS for agents) with capabilities, auth requirements
- **Tasks**: Units of work with lifecycle states (submitted, working, input-required, completed, failed)
- **Transport**: HTTPS + JSON-RPC 2.0
- **Streaming**: SSE for real-time progress
- **Artifacts**: Typed deliverables (documents, images, data) exchanged between agents

A2A complements MCP: MCP connects agents to tools/data; A2A connects agents to each other. Example: inventory agent uses MCP to query database, then uses A2A to notify supplier agent.

### 10. Reliability & Production Patterns

From Anthropic's engineering blog and framework comparisons:

- **41-87% production failure rates** documented for multi-agent systems
- **Circular message relays** persisting 9+ days have been observed
- **Two-level hierarchies** significantly outperform deeper nesting
- **Max iteration limits** are essential (Claude Code uses 15)
- **Graceful degradation**: Agents should adapt when tools fail, not halt
- **Resumable checkpoints**: Save state externally for recovery
- **Rainbow deployments**: Gradual traffic shifts prevent disrupting running agents

Anthropic uses 15x more tokens for multi-agent research vs. single chat, but achieves 90.2% improvement on research tasks.

## Architecture for Cairn

Based on all research, here's how subagents should work in Cairn:

### Core Design

```
User (Chat UI)
  └── Cairn Agent (parent, always-on)
        ├── Subagent: researcher (read-only, web search)
        ├── Subagent: coder (worktree-isolated, full tools)
        ├── Subagent: reviewer (read-only, code analysis)
        └── Subagent: executor (gated, shell access)
```

### Spawn Flow
1. User sends message in chat
2. Cairn agent's ReAct loop decides to delegate
3. Agent creates a Task with `subagent_type` and prompt
4. Task engine spawns subagent with isolated context
5. Subagent gets: system prompt + task prompt + allowed tools
6. SSE streams progress to UI in real-time
7. On completion, result returns to parent agent
8. Parent synthesizes and responds to user

### Go Implementation Considerations

```go
// Subagent definition
type SubagentDef struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    SystemPrompt string  `json:"system_prompt"`
    Tools       []string `json:"tools"`       // allowed tool names
    Model       string   `json:"model"`       // "" = inherit parent
    MaxTurns    int      `json:"max_turns"`
    Background  bool     `json:"background"`
    Isolation   string   `json:"isolation"`   // "none" | "worktree"
}

// Spawn a subagent as a tool call from the parent
type SpawnSubagentParams struct {
    AgentType string `json:"agent_type"`
    Task      string `json:"task"`
    Context   string `json:"context,omitempty"`
}
```

### UI Components (SvelteKit)

1. **SubagentSpawnBubble**: Shows when parent decides to delegate
2. **SubagentProgressCard**: Live streaming progress with tool call count
3. **SubagentResultCollapse**: Expandable final result
4. **SubagentControlBar**: Cancel / retry / promote-to-foreground buttons
5. **ChatModeDropdown**: Like Cursor - switch between direct/delegate/auto modes

### SSE Events

```
event: subagent_spawn
data: {"id": "sa-123", "type": "researcher", "task": "...", "background": false}

event: subagent_progress
data: {"id": "sa-123", "tool": "webSearch", "status": "running", "turn": 3}

event: subagent_stream
data: {"id": "sa-123", "delta": "Found 5 relevant results..."}

event: subagent_complete
data: {"id": "sa-123", "result": "...", "turns": 7, "tools_used": 12}

event: subagent_error
data: {"id": "sa-123", "error": "max turns exceeded", "partial": "..."}
```

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Context bloat from subagent results | Parent stores full verbose output | Use dual-view: show users everything, send parent only summary |
| Infinite delegation loops | No depth limit on agent nesting | Enforce max depth (2 levels), subagents cannot spawn subagents |
| Vague task delegation | Parent doesn't provide enough context | Include file paths, success criteria, output format in spawn |
| Over-parallelization | Spawning agents for trivial tasks | Set minimum complexity threshold before delegating |
| Permission deadlocks | Background agent needs approval no one sees | Pre-approve permissions at spawn time |
| Stale subagent references | Agent memory references completed/failed subagents | Implement subagent lifecycle cleanup in memory system |

## Best Practices

Synthesized from 22 sources:

1. **Two-level max**: Parent + children only. No grandchildren. Chain from parent if deeper delegation needed.
2. **Agent-as-tool**: Wrap subagents as tools with clear descriptions so the LLM routes correctly.
3. **Context curate, don't dump**: Pass specific file paths and criteria, not "here's everything."
4. **Dual output**: Stream full detail to UI, send condensed summary to parent model.
5. **Fail fast**: Set `maxTurns` limits. 15 is Claude Code's default. For Cairn, consider 10 for standard tasks.
6. **Pre-approve background**: Background subagents can't prompt for permissions. Approve tools at spawn.
7. **Typed handoffs**: Use structured input types (reason, priority) for delegation metadata.
8. **Memory cleanup**: When a subagent completes, ensure parent memory doesn't reference it as "active."
9. **SSE everything**: Stream spawn, progress, tool calls, and completion events for UI responsiveness.
10. **Test with small tasks first**: Anthropic starts with 20-query test sets before scaling.

## Framework Comparison for Subagent Support

| Feature | Claude Code | LangGraph | OpenAI SDK | Vercel AI SDK | CrewAI | AutoGen |
|---------|-------------|-----------|------------|---------------|--------|---------|
| Agent-as-tool | Yes (Agent tool) | Yes (@tool) | Yes (handoffs) | Yes (tool()) | Yes (delegation) | Yes (group chat) |
| Context isolation | Full | Shared state | Configurable | Full | RAG-based | Transcript |
| Parallel spawn | Yes | Yes | No | Yes | Yes | Yes |
| Background exec | Yes (Ctrl+B) | Manual | No | Manual | No | No |
| Streaming progress | Yes | Yes | Yes | Yes (generators) | No | No |
| Nesting depth | 1 (no sub-sub) | Unlimited | Unlimited | Unlimited | 1 | Unlimited |
| UI components | Built-in CLI | LangGraph Studio | No | React hooks | AOP dashboard | No |
| Persistent memory | Yes (per-agent) | Checkpointer | No | No | RAG | No |
| Worktree isolation | Yes | No | No | No | No | No |

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [LangChain Subagents Docs](https://docs.langchain.com/oss/python/langchain/multi-agent/subagents) | Official Docs | Definitive supervisor vs router pattern reference |
| [Anthropic Multi-Agent Engineering](https://www.anthropic.com/engineering/multi-agent-research-system) | Engineering Blog | Production lessons from Anthropic's own multi-agent system |
| [Claude Code Subagents](https://code.claude.com/docs/en/sub-agents) | Official Docs | Complete reference for Claude Code's subagent system |
| [Vercel AI SDK Subagents](https://ai-sdk.dev/docs/agents/subagents) | Official Docs | Best UI streaming patterns for subagent progress |
| [OpenAI Handoffs](https://openai.github.io/openai-agents-python/handoffs/) | Official Docs | Handoff pattern with typed metadata |
| [How Agents Call Agents](https://dev.to/openwalrus/how-agents-call-agents-1f48) | Analysis | Cross-framework comparison of nesting and delegation |
| [A2A Protocol (IBM)](https://www.ibm.com/think/topics/agent2agent-protocol) | Reference | A2A protocol overview and MCP complementarity |
| [Top 10 Agent Frameworks 2026](https://o-mega.ai/articles/langgraph-vs-crewai-vs-autogen-top-10-agent-frameworks-2026) | Comparison | Latest framework comparison with orchestration focus |
| [Claude Fast Sub-Agent Patterns](https://claudefa.st/blog/guide/agents/sub-agent-best-practices) | Guide | Parallel vs sequential vs background execution patterns |

---

*Generated by /learn from 22 sources. Depth: deep.*
*See `resources/subagents-sources.json` for full source metadata.*
