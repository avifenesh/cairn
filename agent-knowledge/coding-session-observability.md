# Learning Guide: Real-Time AI Coding Session Observability Panels and Dashboards

**Generated**: 2026-03-21
**Sources**: 22 resources analyzed
**Depth**: medium

---

## Prerequisites

- Basic understanding of AI coding agents (Claude Code, Cursor, Devin, OpenHands)
- Familiarity with SSE (Server-Sent Events) or WebSocket concepts
- Familiarity with LLM concepts: tokens, tool calls, context windows
- For implementation: experience with a frontend framework (React/Svelte) and a backend capable of event streaming

---

## TL;DR

- Every major AI coding agent surfaces observability through a different UI metaphor: Claude Code uses a terminal stream with verbose/thinking mode; Devin uses a multi-pane session viewer (shell + editor + browser); OpenHands uses a tabbed chat/changes/terminal UI; Cursor uses an inline diff + checkpoint timeline.
- Tool calls are the atomic unit of agent activity. All modern platforms expose them as discrete events with name, input, output, and timing — either in a feed, a timeline waterfall, or an inline diff view.
- "Thought flow" (chain-of-thought / extended thinking) is displayed as dimmed/italic gray text in verbose mode (Claude Code `Ctrl+O`), streaming reasoning blocks in the Vercel AI SDK, or hidden entirely behind a summarized response.
- Platform-agnostic observability (Langfuse, AgentOps, Arize Phoenix, LangSmith) uses a hierarchical trace / span model built on OpenTelemetry: one root trace per session, child spans per LLM call or tool use, with token counts and latency on each span.
- SSE is the dominant transport for real-time coding agent UIs. The Vercel AI SDK data stream protocol defines the canonical part-type vocabulary: `text-delta`, `tool-input-start/delta/available`, `tool-output-available`, `reasoning-delta`, `finish-step`.

---

## Core Concepts

### 1. The Agentic Loop as the Observability Model

AI coding agents operate in a three-phase cycle: **gather context -> take action -> verify results**. Each phase generates observable events. Effective observability panels map directly onto this loop:

- **Context gathering**: file reads, grep/glob searches, memory lookups
- **Action taking**: file writes, bash command executions, web fetches, MCP tool calls
- **Verification**: test runs, linter output, diff inspection

The agentic loop is powered by two components: **models** (reasoning) and **tools** (acting). An observability panel must surface both: what the model is thinking and what tools it is invoking.

### 2. Tool Calls as the Atomic Unit of Display

Every AI coding agent platform surfaces tool calls as the fundamental unit of activity display. A tool call has a well-defined lifecycle:

1. **Invocation** — name, input parameters, timestamp
2. **Execution** — running state indicator
3. **Result** — output, duration, success/failure

In the Vercel AI SDK stream protocol, this maps to four part types:
- `tool-input-start` — input generation begins
- `tool-input-delta` — incremental input chunks stream in
- `tool-input-available` — input complete, tool about to execute
- `tool-output-available` — result returned

The key insight: **tool call state is the minimal viable observability signal**. A simple ordered list of `[tool_name] [input_summary] -> [result_summary]` provides 80% of the observability value.

### 3. Reasoning Trace / Thought Flow Visualization

Different platforms handle chain-of-thought display differently:

**Claude Code**: Extended thinking is enabled by default. Press `Ctrl+O` to toggle verbose mode and see internal reasoning displayed as **gray italic text** inline with the conversation.

**Vercel AI SDK**: Reasoning blocks stream using a `reasoning-start / reasoning-delta / reasoning-end` sequence, allowing frontends to progressively render thinking in a dedicated collapsible section.

**LangGraph Studio**: Displays "a stream of real-time information about what steps are happening" with the ability to pause and inspect state at each graph node boundary.

**Devin**: Does not expose LLM reasoning directly. Instead, it exposes the outcome of reasoning through "progress steps" in the session sidebar.

The dominant UI pattern for thought flow: **collapsed by default, expandable per turn**, rendered in a muted/secondary color to visually distinguish reasoning from action.

### 4. Session Activity Feeds and Event Streams

An activity feed is an ordered list of events emitted during a coding session. The events are heterogeneous (file edit, bash run, LLM call, tool result, error) but share a common shape:

```json
{
  "timestamp": "ISO8601",
  "type": "tool_call | file_edit | bash | llm_turn | error",
  "tool": "string",
  "input": "any",
  "output": "any",
  "duration_ms": 0,
  "status": "running | success | error"
}
```

**OpenHands** exposes this through an EventStream component (client-server REST architecture over a Docker sandbox). Actions flow: agent -> EventStream -> action executor -> observations back to agent.

**AgentOps** calls this a "Session Waterfall" — a timeline showing all LLM calls, tool invocations, and errors with precise event details. The waterfall view is the best pattern for understanding temporal ordering and identifying bottlenecks.

**Claude Code hooks** (`PreToolUse`, `PostToolUse`, `SubagentStart`, `SubagentStop`, `Stop`, `SessionStart`, `SessionEnd`) provide the raw event stream needed to build a custom activity feed.

### 5. Dashboard Patterns for Monitoring Coding Sessions

The major patterns observed across platforms:

**Multi-pane session viewer (Devin model)**
Three synchronized panes: shell terminal + code editor + browser. Users can watch the agent work across all three surfaces simultaneously. The sidebar shows clickable "progress steps" as the agent works.

**Tabbed panel (OpenHands model)**
Chat panel (agent reasoning narrative) + Changes tab (file modifications audit trail) + Terminal tab (commands + output) + VS Code editor + App/Browser tabs.

**Inline terminal stream (Claude Code model)**
All activity is rendered inline in a terminal REPL. Tool calls appear as formatted text blocks. File edits show as diff-style output.

**IDE sidebar (Cursor model)**
Agent chat panel embedded in the editor sidebar. File edits appear as inline diffs in the editor itself. Checkpoints (automatic snapshots before significant changes) create a timeline that users can rewind.

**External observability dashboard (Langfuse / AgentOps / Arize Phoenix model)**
Separate tool from the coding agent itself. Collects traces via SDK instrumentation. Renders a hierarchical trace tree (session -> LLM calls -> tool spans), timeline waterfall, cost/token metrics, and per-session scoring.

### 6. SSE and WebSocket Streaming Patterns

All real-time coding agent UIs use either SSE or WebSocket to push events from server to frontend.

**SSE (dominant for text streaming)**
The Vercel AI SDK uses SSE with a structured data stream protocol. SSE is preferred for unidirectional server-to-client streams and handles reconnection natively.

Key SSE considerations for coding agent UIs:
- Use `ping` messages to keep the connection alive during long-running tool executions
- Emit a `finish-step` event after each LLM call so the frontend knows to close the current tool accumulation window
- Include a unique `tool_use_id` in every tool event to allow frontend correlation across input and output parts
- Stream tool inputs delta-by-delta so users can see the agent's intended action before it executes

**WebSocket (preferred for bidirectional)**
OpenHands uses a REST+WebSocket architecture where the frontend sends user messages and the backend streams agent events back. WebSocket is preferred when the user needs to interrupt or inject messages during execution.

### 7. Token Usage and Cost Monitoring

**What to display:**
- Input tokens (prompt + context) per LLM call
- Output tokens per LLM call
- Thinking/reasoning tokens (separately, as they are billed)
- Cumulative tokens for the session
- Context window fill percentage
- Estimated cost per call and cumulative for session

### 8. OpenTelemetry Semantic Conventions for LLM Observability

The key signal types:
- **Traces**: Complete journey of a coding task (prompt -> intermediate steps -> final output)
- **Spans**: Individual LLM calls, tool executions, retrieval steps
- **Metrics**: Token counters, request volume, latency histograms
- **Logs**: Reasoning steps, decision points, error details

Recommended architecture:
1. One root span per user request / session turn
2. Child spans for each LLM call (with model, temperature, tokens in/out)
3. Child spans for each tool execution (with tool name, input hash, duration, success)

---

## Code Examples

### Basic SSE Event Feed (Vercel AI SDK Pattern)

```typescript
// Server: stream tool call events via SSE
import { streamText, tool } from 'ai';

const result = streamText({
  model: yourModel,
  tools: {
    readFile: tool({
      description: 'Read a file from the workspace',
      parameters: z.object({ path: z.string() }),
      execute: async ({ path }) => readFileSync(path, 'utf8'),
      experimental_onToolCallStart: ({ toolCallId, toolName, input }) => {
        feedEmitter.emit({ type: 'tool_start', toolCallId, toolName, input, ts: Date.now() });
      },
      experimental_onToolCallFinish: ({ toolCallId, output, durationMs }) => {
        feedEmitter.emit({ type: 'tool_end', toolCallId, output, durationMs, ts: Date.now() });
      },
    }),
  },
});

for await (const part of result.fullStream) {
  if (part.type === 'tool-input-start') { /* show tool starting */ }
  if (part.type === 'tool-output-available') { /* show tool result */ }
  if (part.type === 'reasoning') { /* show thinking trace */ }
}
```

### Claude Code Hook for Custom Observability Dashboard

```json
{
  "hooks": {
    "PreToolUse": [{ "matcher": ".*", "hooks": [{ "type": "http", "url": "http://localhost:4000/api/agent-events", "method": "POST" }] }],
    "PostToolUse": [{ "matcher": ".*", "hooks": [{ "type": "http", "url": "http://localhost:4000/api/agent-events", "method": "POST" }] }],
    "SubagentStart": [{ "matcher": ".*", "hooks": [{ "type": "http", "url": "http://localhost:4000/api/agent-events" }] }],
    "SubagentStop": [{ "matcher": ".*", "hooks": [{ "type": "http", "url": "http://localhost:4000/api/agent-events" }] }]
  }
}
```

### Activity Feed Component (Svelte 5 Runes)

```svelte
<script lang="ts">
  type AgentEvent = {
    id: string;
    type: 'tool_start' | 'tool_end' | 'llm_turn' | 'file_edit' | 'bash' | 'error';
    tool?: string;
    summary: string;
    status: 'running' | 'success' | 'error';
    ts: number;
    durationMs?: number;
  };

  let events = $state<AgentEvent[]>([]);

  const source = new EventSource('/api/stream');
  source.onmessage = (e) => {
    const event = JSON.parse(e.data);
    events = [...events, event];
  };
</script>

<div class="activity-feed">
  {#each events as event (event.id)}
    <div class="event event--{event.status}">
      <span class="event__tool">{event.tool ?? event.type}</span>
      <span class="event__summary">{event.summary}</span>
      {#if event.durationMs}
        <span class="event__duration">{event.durationMs}ms</span>
      {/if}
    </div>
  {/each}
</div>
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Displaying raw tool input/output verbatim | Tool inputs can contain entire file contents | Summarize: show tool name + key param + output length. Make full content expandable |
| No correlation between tool input and output events | Input and output arrive as separate SSE events | Use `tool_use_id` as correlation key; group under one card |
| Blocking UI on every tool call | Naive polling or synchronous rendering | Use SSE for push; virtual scrolling for long sessions |
| Showing thinking tokens as regular text | Reasoning blocks look like normal text | Render reasoning in collapsible "Thinking..." section with muted styling |
| No session boundary in feed | Multi-session data blurs together | Always include `session_id`; scope the activity feed |
| Expensive re-renders on every token delta | Text delta events fire at high frequency | Batch text deltas; only re-render active text node |
| Missing error states | Tool failures can be silent | Subscribe to PostToolUseFailure or catch error stream parts |
| Context window not surfaced | Users don't know when agent is degrading | Display context fill % as progress bar; warn at 70% and 90% |

---

## Best Practices

1. **Make tool calls the primary UI element.** Structure feed around tool events, not LLM turns.
2. **Use waterfall timeline for debugging, live feed for monitoring.** Build both if possible.
3. **Expose thought flow but hide it by default.** Collapsed under "Show reasoning" toggle, muted styling.
4. **Stream tool inputs as they are generated.** Enables intervention before costly actions.
5. **Always show status indicators.** Running (spinner), success (green check), error (red X).
6. **Use `session_id` as top-level grouping key.** Session is the unit of analysis.
7. **Track token usage per turn, cumulative per session.** Display context fill % prominently.
8. **Use OpenTelemetry for platform-agnostic instrumentation.** Build on OTEL GenAI semantic conventions.
9. **Give users interrupt/steer controls adjacent to the activity feed.** Where they notice off-track behavior.
10. **Persist the activity feed across sessions.** Store events in durable log (SQLite).

---

## Tool and Platform Reference

| Platform | Primary Observability Mechanism | Key UI Metaphor |
|----------|---------------------------------|-----------------|
| Claude Code | Terminal stream + verbose mode + hooks event system | Inline terminal REPL |
| Devin | Multi-pane session viewer (shell + editor + browser + progress steps) | Parallel panes |
| OpenHands | Tabbed chat/changes/terminal + REST event stream | Tabbed panels |
| Cursor | Inline diffs + checkpoint timeline + queued messages sidebar | IDE sidebar |
| Aider | Watch mode + AI comment markers in editor | Editor-embedded |
| LangGraph Studio | Graph state visualization + step-by-step replay | Agent graph IDE |
| Langfuse | Hierarchical trace tree + timeline waterfall + cost metrics | External dashboard |
| AgentOps | Session waterfall + LLM call history + error tracking | External dashboard |
| Arize Phoenix | Span hierarchy + prompt playground + dataset clustering | External dashboard |
| Vercel AI SDK | Data stream parts (SSE) + tool invocation state machine | Protocol / library |

---

*Generated by /learn from 22 sources.*
*See `resources/coding-session-observability-sources.json` for full source metadata.*
