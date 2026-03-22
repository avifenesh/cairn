# Learning Guide: Architecture Patterns for AI Coding Session Panels

**Generated**: 2026-03-21
**Sources**: 22 resources analyzed
**Depth**: medium

---

## Prerequisites

- Familiarity with REST APIs and HTTP fundamentals
- Basic knowledge of a reactive frontend framework (Svelte, React, or similar)
- Understanding of async/await and event-driven programming
- Awareness of LLM tool-calling concepts (tool_call / tool_result message pairs)

---

## TL;DR

- Model a coding session as an **append-only event log** (event sourcing). Each action — tool call, LLM text chunk, file write, approval request — is an immutable event with an ID, timestamp, source, and typed payload.
- **SSE (Server-Sent Events) beats WebSockets** for agent observability streams: simpler, HTTP-native, auto-reconnects, and multiplexes for free over HTTP/2. Use WebSockets (or Socket.IO) only if you need bidirectional messaging within the same stream.
- The **Anthropic streaming API** emits a well-documented SSE event lifecycle: `message_start` → `content_block_start` → `content_block_delta` (text or tool input JSON) → `content_block_stop` → `message_stop`. Mirror this taxonomy in your internal session event types.
- Split the session panel into three zones: **activity stream** (append-only log), **diff viewer** (file change overlay), and **chat/approval sidebar**. Keep a single store that owns session state; derive display data from it.
- Persist the raw event log to a durable store (SQLite rows, append-only file, or a message queue topic). On reconnect, replay events from the log to reconstruct UI state without re-running agent logic.

---

## Core Concepts

### 1. Event Sourcing for Coding Sessions

Event sourcing means treating every state change as an immutable fact appended to a log, rather than overwriting current state. Martin Fowler's canonical definition: "we record all changes to an application state as a sequence of events." Current state is derived by replaying the event log.

For a coding session, this maps directly:

- **Why it fits**: an autonomous coding task is inherently a sequence of decisions and side-effects. The log *is* the work.
- **Audit and replay**: you can reconstruct exactly what the agent did, when, and why — invaluable for debugging runaway agents.
- **Temporal query**: examine what the session looked like at any past timestamp.
- **Compensating events**: if a bad file write happened, append a revert event rather than deleting history.

**Snapshot strategy**: for long sessions (hundreds of events), maintain a periodic snapshot of materialized state every N events so replay does not start from zero. Azure Architecture Center recommends this explicitly: "consider creating snapshots at specific intervals such as a specified number of events."

**CQRS pairing**: separate the write path (append events) from the read path (materialized views for the UI). The UI reads from a projection, not the raw event log. This keeps writes fast (append-only) and reads cheap (pre-computed).

**Gotcha — eventual consistency**: the UI's materialized view is always slightly behind the event log. Design the panel to be comfortable with this; don't assume UI state equals authoritative truth.

### 2. The Session Event Data Model

A coding session event has a small universal envelope plus a typed payload:

```typescript
// Universal envelope — all events share this
interface SessionEvent {
  id: number;              // monotonically increasing sequence number
  session_id: string;      // stable conversation/session ID
  timestamp: string;       // ISO 8601
  source: "agent" | "user" | "environment";
  cause?: number;          // id of the triggering event (causal chain)
  type: SessionEventType;
  payload: EventPayload;   // discriminated union by type
}

type SessionEventType =
  | "text_delta"           // streaming LLM text fragment
  | "thinking_delta"       // extended thinking fragment
  | "tool_call"            // agent invokes a tool
  | "tool_result"          // tool returns output
  | "file_change"          // file written / deleted / renamed
  | "approval_request"     // agent requests human approval
  | "approval_response"    // human approves or denies
  | "user_message"         // user sends a message
  | "agent_state_change"   // agent lifecycle: idle / running / paused / stopped / error
  | "phase_change"         // session enters a new logical phase
  | "error"                // error occurred
  | "session_start"        // session created
  | "session_end";         // session completed or cancelled

// Example payloads
interface ToolCallPayload {
  tool_id: string;         // unique call ID for correlation
  tool_name: string;
  input: Record<string, unknown>;
}

interface ToolResultPayload {
  tool_id: string;         // matches ToolCallPayload.tool_id
  output: string;
  is_error: boolean;
}

interface FileChangePayload {
  path: string;
  operation: "write" | "delete" | "rename";
  diff?: string;           // unified diff string (optional, can be large)
  previous_path?: string;  // for renames
  source: "llm_edit" | "tool" | "user";
}

interface ApprovalRequestPayload {
  approval_id: string;
  description: string;
  operation: string;       // what will happen if approved
  severity: "low" | "medium" | "high";
  expires_at?: string;
}
```

This mirrors OpenHands' approach: their `Event` dataclass uses `action` vs `observation` discrimination, `tool_call_metadata`, `llm_metrics`, `cause` (causal ID), and `source` (AGENT/USER/ENVIRONMENT). The serialization format uses `TOP_KEYS` (`id`, `timestamp`, `source`, `message`, `cause`, `action`/`observation`, `tool_call_metadata`) with action events having an `args` object and observation events having `content` + `extras`.

### 3. Session Phases and Lifecycle

Organize long coding tasks into phases to enable progress tracking and coarse-grained resumption:

```typescript
interface CodingSession {
  id: string;
  created_at: string;
  updated_at: string;
  status: "initializing" | "planning" | "executing" | "paused" | "awaiting_approval" | "completed" | "failed";
  title?: string;
  repository?: string;
  branch?: string;
  phases: SessionPhase[];
  current_phase_id?: string;
  event_count: number;
  snapshot_at_event_id?: number;  // last snapshot for fast replay
  metadata: Record<string, unknown>;
}

interface SessionPhase {
  id: string;
  name: string;            // e.g., "Planning", "File edits", "Testing"
  started_at: string;
  ended_at?: string;
  status: "pending" | "active" | "completed" | "skipped";
  event_ids: number[];     // events belonging to this phase
}
```

OpenHands' conversation management API illustrates this: `POST /api/conversations` creates a session, `POST /api/conversations/{id}/start` begins the agent loop, `POST /api/conversations/{id}/stop` halts it, and `GET /api/conversations/{id}` returns status and runtime info. Event history retrieval uses a window around a target event ID for context reconstruction.

### 4. SSE Streaming Architecture

Server-Sent Events is the right transport for agent-to-browser streaming:

```
Agent loop (server)
    |
    | appends events to log
    v
SSE broadcast layer  ──── text/event-stream ────►  Browser
                                                        |
                                              EventSource API
                                                        |
                                               Svelte/React store
                                                        |
                                               Session panel UI
```

**SSE endpoint pattern** (SvelteKit `+server.ts`):

```typescript
// src/routes/api/sessions/[id]/stream/+server.ts
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ params, request }) => {
  const sessionId = params.id;
  const lastEventId = request.headers.get('Last-Event-ID');

  const stream = new ReadableStream({
    start(controller) {
      const encoder = new TextEncoder();

      function send(event: string, data: unknown, id?: number) {
        let chunk = '';
        if (id !== undefined) chunk += `id: ${id}\n`;
        chunk += `event: ${event}\n`;
        chunk += `data: ${JSON.stringify(data)}\n\n`;
        controller.enqueue(encoder.encode(chunk));
      }

      // If reconnecting, replay missed events from log
      if (lastEventId) {
        const missed = getEventsSince(sessionId, Number(lastEventId));
        missed.forEach(e => send(e.type, e.payload, e.id));
      }

      // Subscribe to new events
      const unsubscribe = subscribeToSession(sessionId, (event) => {
        send(event.type, event.payload, event.id);
      });

      // Clean up on client disconnect
      request.signal.addEventListener('abort', () => {
        unsubscribe();
        controller.close();
      });
    }
  });

  return new Response(stream, {
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      'Connection': 'keep-alive',
    }
  });
};
```

**Key SSE properties**:
- The `id` field on each event enables the browser to send `Last-Event-ID` on reconnect, letting you replay missed events — built-in resume semantics.
- Use comment lines (`: keepalive`) to prevent proxy timeouts on idle sessions.
- With HTTP/2, the six-connection-per-origin limit no longer applies; you can have one SSE stream per session tab.

### 5. Anthropic Streaming Event Taxonomy

The Anthropic API's SSE event stream is the definitive reference for LLM streaming UI patterns. Each stream follows this lifecycle:

```
message_start
  content_block_start  (index: 0, type: "text" or "thinking")
    content_block_delta  (text_delta or thinking_delta, repeating)
  content_block_stop
  content_block_start  (index: 1, type: "tool_use")
    content_block_delta  (input_json_delta, partial JSON, repeating)
  content_block_stop
message_delta  (stop_reason: "tool_use" | "end_turn")
message_stop
```

Map these directly to your session event types:

| Anthropic event | Session event type | Notes |
|---|---|---|
| `content_block_delta` (text_delta) | `text_delta` | Append to current assistant turn |
| `content_block_delta` (thinking_delta) | `thinking_delta` | Show in collapsible "reasoning" block |
| `content_block_start` (tool_use) | `tool_call` | Show pending tool call card |
| `content_block_stop` (after tool_use) | — | Tool input is now complete; execute |
| Tool execution result | `tool_result` | Attach to tool call card |
| `message_stop` (end_turn) | `agent_state_change` | Agent is idle |

For `tool_use` streaming: accumulate `input_json_delta` strings; parse the complete JSON only after `content_block_stop`. The API emits partial JSON fragments; never parse mid-stream.

### 6. SvelteKit / Svelte Patterns for Streaming Panels

**Store architecture for a session panel:**

```typescript
// src/lib/stores/session.ts
import { writable, derived, readable } from 'svelte/store';
import type { SessionEvent, CodingSession } from '$lib/types';

// Core event log — append-only array
export const sessionEvents = writable<SessionEvent[]>([]);

// Session metadata
export const sessionMeta = writable<CodingSession | null>(null);

// Active SSE subscription — readable wrapping EventSource
export function createSessionStream(sessionId: string) {
  return readable<SessionEvent | null>(null, (set) => {
    const es = new EventSource(`/api/sessions/${sessionId}/stream`);

    // Handle typed events
    const eventTypes: SessionEventType[] = [
      'text_delta', 'tool_call', 'tool_result',
      'file_change', 'approval_request', 'agent_state_change'
    ];

    eventTypes.forEach(type => {
      es.addEventListener(type, (e: MessageEvent) => {
        const event: SessionEvent = JSON.parse(e.data);
        sessionEvents.update(events => [...events, event]);
        set(event);
      });
    });

    es.onerror = () => {
      // EventSource auto-reconnects; Last-Event-ID header sent automatically
    };

    return () => es.close();
  });
}

// Derived: group events by type for panel consumption
export const fileChanges = derived(sessionEvents, ($events) =>
  $events.filter(e => e.type === 'file_change')
);

export const pendingApprovals = derived(sessionEvents, ($events) =>
  $events.filter(e =>
    e.type === 'approval_request' &&
    !$events.some(r => r.type === 'approval_response' &&
      (r.payload as any).approval_id === (e.payload as any).approval_id)
  )
);

export const agentStatus = derived(sessionEvents, ($events) => {
  const last = [...$events].reverse().find(e => e.type === 'agent_state_change');
  return (last?.payload as any)?.state ?? 'idle';
});
```

**Activity stream component pattern** (`SessionActivity.svelte`):

```svelte
<script lang="ts">
  import { sessionEvents } from '$lib/stores/session';
  import EventCard from './EventCard.svelte';
  import { tick } from 'svelte';

  let scrollEl: HTMLElement;

  // Auto-scroll to bottom on new events
  $effect(() => {
    $sessionEvents; // reactive dependency
    tick().then(() => {
      scrollEl?.scrollTo({ top: scrollEl.scrollHeight, behavior: 'smooth' });
    });
  });
</script>

<div class="activity-stream" bind:this={scrollEl}>
  {#each $sessionEvents as event (event.id)}
    <EventCard {event} />
  {/each}
</div>
```

### 7. Split-Panel Layout Architecture

A coding session panel should have three distinct zones:

```
┌─────────────────────────────────────────────────────────────┐
│  Session Header: title · status pill · elapsed · controls   │
├────────────────────────┬────────────────┬───────────────────┤
│                        │                │                   │
│   Activity Stream      │  Diff Viewer   │  Chat / Approval  │
│   (scrollable log)     │  (file changes)│  Sidebar          │
│                        │                │                   │
│  - thinking blocks     │  - unified diff│  - user messages  │
│  - tool call cards     │  - file tree   │  - approval cards │
│  - tool results        │  - before/after│  - quick replies  │
│  - agent state changes │                │                   │
│                        │                │                   │
└────────────────────────┴────────────────┴───────────────────┘
```

**Component hierarchy** (framework-agnostic):

```
CodingSessionPanel           ← owns session store, SSE subscription
├── SessionHeader            ← status pill, elapsed timer, stop button
├── PanelContainer           ← resizable split layout
│   ├── ActivityStream       ← scrollable event log, auto-scroll
│   │   ├── ThinkingBlock    ← collapsible reasoning display
│   │   ├── ToolCallCard     ← pending / executing / complete states
│   │   │   └── ToolResult   ← output, error badge
│   │   ├── TextDeltaBlock   ← streaming text with cursor
│   │   ├── FileChangeEntry  ← compact change line + open diff button
│   │   └── AgentStateChip   ← running / paused / waiting for approval
│   ├── DiffViewer           ← driven by fileChanges derived store
│   │   ├── FileTree         ← changed files list
│   │   └── DiffPane         ← syntax-highlighted unified diff
│   └── ChatSidebar          ← user input + approval queue
│       ├── ApprovalCard[]   ← approve / deny with reason
│       └── MessageInput     ← POST to /api/sessions/{id}/messages
└── SessionFooter            ← token count, cost, phase progress
```

**Key design principle**: lift all session state to `CodingSessionPanel`. Child components are pure display — they receive data via props/stores and emit events upward. This matches React's "Thinking in React" and Svelte's store reactivity model.

### 8. Session Persistence and Replay

**Persistence schema** (SQLite — fits Cairn's architecture):

```sql
-- Session metadata
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'initializing',
  title TEXT,
  repository TEXT,
  branch TEXT,
  metadata JSON
);

-- Append-only event log
CREATE TABLE session_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,  -- global sequence number
  session_id TEXT NOT NULL REFERENCES sessions(id),
  timestamp TEXT NOT NULL,
  source TEXT NOT NULL,                   -- agent | user | environment
  event_type TEXT NOT NULL,
  cause_id INTEGER REFERENCES session_events(id),
  payload JSON NOT NULL,
  CONSTRAINT session_events_idx UNIQUE (session_id, id)
);
CREATE INDEX idx_session_events_session ON session_events(session_id, id);

-- Snapshots for fast replay
CREATE TABLE session_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id),
  at_event_id INTEGER NOT NULL,           -- snapshot state after this event
  snapshot_state JSON NOT NULL,           -- materialized session state
  created_at TEXT NOT NULL
);
```

**Replay pattern**:

```typescript
async function replaySession(sessionId: string): Promise<MaterializedState> {
  // Try snapshot first
  const snapshot = await db.getLatestSnapshot(sessionId);
  const fromEventId = snapshot?.at_event_id ?? 0;
  const initialState = snapshot?.snapshot_state ?? emptyState();

  // Replay events after snapshot
  const events = await db.getEventsSince(sessionId, fromEventId);
  return events.reduce(applyEvent, initialState);
}

function applyEvent(state: MaterializedState, event: SessionEvent): MaterializedState {
  switch (event.type) {
    case 'file_change':
      return { ...state, files: applyFileChange(state.files, event.payload) };
    case 'agent_state_change':
      return { ...state, agentStatus: event.payload.state };
    case 'text_delta':
      return { ...state, currentText: (state.currentText ?? '') + event.payload.text };
    // ...
    default:
      return state;
  }
}
```

### 9. WebSocket vs SSE — Decision Guide

| Criterion | SSE | WebSocket / Socket.IO |
|---|---|---|
| Communication direction | Server → client only | Bidirectional |
| Protocol | Plain HTTP | Custom (WS handshake) |
| Auto-reconnect | Built in, browser-managed | Must implement (or use Socket.IO) |
| HTTP/2 multiplexing | Free, one connection | Each WS = one TCP connection |
| Firewall / proxy compatibility | Excellent (standard HTTP) | Sometimes blocked (packet inspection) |
| Browser connection limit | 6 per origin (HTTP/1.1) / 100 (HTTP/2) | No per-origin limit |
| Binary data | UTF-8 only | Binary + UTF-8 |
| Complexity | Low (EventSource API) | Higher (protocol upgrade, state) |

**Recommendation for agent observability panels**:

- Use **SSE** for the agent-to-browser event stream. The stream is inherently unidirectional (agent produces events, browser displays them). Auto-reconnect is critical for long-running tasks. HTTP/2 makes the connection limit a non-issue.
- Use a **separate REST endpoint** (or a small bidirectional channel) for user-to-agent messages (approvals, chat messages). These are low-frequency and don't need a persistent WS connection.
- Use **Socket.IO** only if you need presence features, rooms with multiple collaborators, or real-time cursor sharing — i.e., features closer to collaborative editing than agent observation.

OpenHands uses Socket.IO (bidirectional), which makes sense for their architecture where user messages and agent observations share one channel. For a simpler single-user observability panel, SSE + REST is the right trade-off.

### 10. Existing Open-Source Implementations

**OpenHands (All-Hands-AI)**
- Python backend with `WebSession` per client: queue-based event emission, `_monitor_publish_queue` loop, `_on_event` subscriber on the agent event stream.
- Events classified as `Action` (agent does something) vs `Observation` (environment responds).
- `ConversationStore` abstract base with `save_metadata`, `get_metadata`, `exists`, `validate_metadata`, parallel batch retrieval via `wait_all()`.
- Frontend: React + TypeScript, `stores/` for state management, `api/` for HTTP client, `hooks/` for shared logic.
- Session creation: `POST /api/conversations` with `InitSessionRequest` (repo, branch, initial messages, optional `replay_json`).

**AgentScope (ModelScope)**
- Multi-agent framework with "MsgHub" for participant routing and message passing.
- Supports streaming responses and human-in-the-loop interruption.
- ReAct agent pattern with reasoning loop visualization.

**VS Code Copilot Chat**
- "Chat participant" model: extensions register as domain-expert agents within the unified chat view.
- Tool calling via LanguageModelTool API; tool results integrated into streaming response.
- Session management: parallel sessions with status tracking, file change review, resume.

---

## Code Examples

### Basic: SSE Endpoint with Event Replay (Go)

```go
// GET /v1/sessions/{id}/stream
func (h *SessionHandler) Stream(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "id")

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "SSE not supported", http.StatusInternalServerError)
        return
    }

    // Replay missed events if reconnecting
    lastEventID := r.Header.Get("Last-Event-ID")
    if lastEventID != "" {
        fromID, _ := strconv.Atoi(lastEventID)
        missed, _ := h.store.GetEventsSince(r.Context(), sessionID, fromID)
        for _, ev := range missed {
            writeSSEEvent(w, ev)
            flusher.Flush()
        }
    }

    // Subscribe to live events
    sub := h.bus.Subscribe(sessionID)
    defer h.bus.Unsubscribe(sessionID, sub)

    for {
        select {
        case ev := <-sub:
            writeSSEEvent(w, ev)
            flusher.Flush()
        case <-r.Context().Done():
            return
        case <-time.After(30 * time.Second):
            // Keepalive comment
            fmt.Fprintf(w, ": keepalive\n\n")
            flusher.Flush()
        }
    }
}

func writeSSEEvent(w http.ResponseWriter, ev SessionEvent) {
    fmt.Fprintf(w, "id: %d\n", ev.ID)
    fmt.Fprintf(w, "event: %s\n", ev.Type)
    data, _ := json.Marshal(ev.Payload)
    fmt.Fprintf(w, "data: %s\n\n", data)
}
```

### Svelte Store: Session Stream with Auto-Reconnect

```typescript
// src/lib/stores/sessionStream.ts
import { writable, derived } from 'svelte/store';

export interface StreamedEvent {
  id: number;
  type: string;
  payload: unknown;
  timestamp: string;
}

export function createSessionStore(sessionId: string) {
  const events = writable<StreamedEvent[]>([]);
  const connectionState = writable<'connecting' | 'open' | 'closed'>('connecting');

  // Readable SSE connection
  const es = new EventSource(`/api/sessions/${sessionId}/stream`);

  es.onopen = () => connectionState.set('open');
  es.onerror = () => connectionState.set('closed'); // auto-reconnects

  // Typed event listeners
  const sessionEventTypes = [
    'text_delta', 'thinking_delta', 'tool_call', 'tool_result',
    'file_change', 'approval_request', 'approval_response',
    'agent_state_change', 'user_message', 'phase_change', 'error'
  ];

  sessionEventTypes.forEach(type => {
    es.addEventListener(type, (e: MessageEvent) => {
      events.update(prev => [...prev, JSON.parse(e.data)]);
    });
  });

  // Derived projections
  const fileChanges = derived(events, $e =>
    $e.filter(e => e.type === 'file_change')
  );

  const pendingApprovals = derived(events, $e => {
    const requests = $e.filter(e => e.type === 'approval_request');
    const resolved = new Set(
      $e.filter(e => e.type === 'approval_response')
         .map(e => (e.payload as any).approval_id)
    );
    return requests.filter(r => !resolved.has((r.payload as any).approval_id));
  });

  const agentStatus = derived(events, $e => {
    const last = [...$e].reverse().find(e => e.type === 'agent_state_change');
    return (last?.payload as any)?.state ?? 'idle';
  });

  function destroy() { es.close(); }

  return { events, connectionState, fileChanges, pendingApprovals, agentStatus, destroy };
}
```

### Session Event Schema (TypeScript — for SQLite serialization)

```typescript
// Domain event types for an AI coding session
export type SessionEventPayload =
  | { kind: 'text_delta'; text: string; response_id: string }
  | { kind: 'thinking_delta'; thinking: string }
  | { kind: 'tool_call'; tool_id: string; tool_name: string; input: Record<string, unknown> }
  | { kind: 'tool_result'; tool_id: string; output: string; is_error: boolean; duration_ms: number }
  | { kind: 'file_change'; path: string; operation: 'write' | 'delete' | 'rename'; diff?: string }
  | { kind: 'approval_request'; approval_id: string; description: string; severity: 'low' | 'medium' | 'high' }
  | { kind: 'approval_response'; approval_id: string; approved: boolean; reason?: string }
  | { kind: 'agent_state_change'; state: 'idle' | 'running' | 'paused' | 'awaiting_approval' | 'stopped' | 'error' }
  | { kind: 'user_message'; content: string; attachments?: string[] }
  | { kind: 'phase_change'; phase: string; description?: string }
  | { kind: 'error'; message: string; code?: string };

export interface SessionEvent {
  id: number;
  session_id: string;
  timestamp: string;
  source: 'agent' | 'user' | 'environment';
  cause_id?: number;
  payload: SessionEventPayload;
}
```

### Materializing State from Event Log

```typescript
interface MaterializedSessionState {
  status: string;
  currentPhase?: string;
  agentText: string;          // accumulated streaming text for current turn
  pendingToolCalls: Map<string, unknown>;
  files: Map<string, string>; // path -> latest content
  pendingApprovals: Map<string, unknown>;
}

function applyEvent(
  state: MaterializedSessionState,
  event: SessionEvent
): MaterializedSessionState {
  const p = event.payload;
  switch (p.kind) {
    case 'text_delta':
      return { ...state, agentText: state.agentText + p.text };
    case 'tool_call': {
      const calls = new Map(state.pendingToolCalls);
      calls.set(p.tool_id, p);
      return { ...state, pendingToolCalls: calls };
    }
    case 'tool_result': {
      const calls = new Map(state.pendingToolCalls);
      calls.delete(p.tool_id);
      return { ...state, pendingToolCalls: calls };
    }
    case 'file_change': {
      const files = new Map(state.files);
      if (p.operation === 'delete') files.delete(p.path);
      else files.set(p.path, p.diff ?? '');
      return { ...state, files };
    }
    case 'approval_request': {
      const approvals = new Map(state.pendingApprovals);
      approvals.set(p.approval_id, p);
      return { ...state, pendingApprovals: approvals };
    }
    case 'approval_response': {
      const approvals = new Map(state.pendingApprovals);
      approvals.delete(p.approval_id);
      return { ...state, pendingApprovals: approvals };
    }
    case 'agent_state_change':
      return { ...state, status: p.state, agentText: p.state === 'idle' ? '' : state.agentText };
    case 'phase_change':
      return { ...state, currentPhase: p.phase };
    default:
      return state;
  }
}
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---|---|---|
| Parsing partial tool input JSON mid-stream | `input_json_delta` events emit fragments | Buffer deltas; parse only after `content_block_stop` |
| SSE connection limit exhausted | Multiple session tabs over HTTP/1.1 | Ensure HTTP/2 is enabled; share a single EventSource per session via a store |
| UI state diverges after reconnect | Missed events not replayed | Always include `id` field in SSE events; server reads `Last-Event-ID` header and replays |
| Unbounded event log growth | No snapshotting strategy | Snapshot every 100–200 events; only store diff, not full file content |
| Diff viewer freezes on large files | Rendering entire diff in the DOM | Virtualise the diff list (only render visible lines); limit diff size to 1000 lines |
| Blocking UI on approval during long task | Approval gates not surfaced visually | Derive `pendingApprovals` as a first-class store; show persistent amber banner |
| Race condition: replay overtakes live events | Sequence number gaps | Assign monotonic IDs at write time; client deduplicates by ID before appending to store |
| Memory leak from SSE subscription | `EventSource` not closed on component unmount | Return cleanup function from `readable` store; call `destroy()` in Svelte `onDestroy` |

---

## Best Practices

1. **Keep the event log as the authoritative source of truth.** Never mutate past events. For corrections, append a compensating event. (Sources: Fowler EventSourcing, Azure Architecture Center)

2. **Use sequence numbers, not timestamps, for event ordering.** Timestamps are unreliable under concurrent writes. SQLite `AUTOINCREMENT` or a Redis `INCR` gives you a global sequence. (Source: Azure Architecture Center — event ordering section)

3. **Emit SSE events with typed names, not generic "message".** Use `event: tool_call\n` not `event: message\n`. The browser `EventSource` API dispatches named events to specific listeners, making client filtering trivial. (Source: MDN SSE documentation)

4. **Build idempotent event consumers.** If the browser reconnects and replays events, duplicate processing must be a no-op. Deduplicate by event ID before appending to the store array. (Source: Azure Architecture Center — idempotency section)

5. **Expose a `Last-Event-ID`-aware replay endpoint.** This is SSE's native resume mechanism. The browser sends the header automatically; your server reads it and replays from that ID. Zero client-side reconnect logic needed. (Source: MDN SSE, Smashing Magazine SSE/WS article)

6. **Separate the diff storage from the file content.** Store unified diffs in events, not full file snapshots. Reconstruct content lazily when the diff viewer opens. This keeps event payloads small. (Source: derived from event sourcing snapshot patterns)

7. **Use derived Svelte stores for panel-specific projections.** `fileChanges`, `pendingApprovals`, `agentStatus` should be derived from the core event array — not separately managed state. Changes automatically propagate. (Source: Svelte store documentation)

8. **Gate destructive operations as approval events.** Destructive tool calls (delete file, run shell command with side effects) should emit an `approval_request` event before executing. The UI surfaces this immediately via the `pendingApprovals` derived store. (Source: Cairn project safety model)

9. **Write keepalive comments every 30 seconds.** Idle SSE connections are killed by load balancers and proxies. A `: keepalive\n\n` comment line (no `event:` prefix) keeps the connection alive without generating a UI event. (Source: MDN SSE documentation)

10. **Prefer HTTP/2 for SSE in multi-tab applications.** Under HTTP/1.1, six simultaneous SSE connections per browser per origin is a hard limit. HTTP/2 raises this to 100 concurrent streams over one TCP connection. (Source: Ably WebSockets vs SSE guide, Smashing Magazine)

---

## Further Reading

| Resource | Type | Why Recommended |
|---|---|---|
| [Event Sourcing — Martin Fowler](https://martinfowler.com/eaaDev/EventSourcing.html) | Reference article | Canonical definition, patterns, replay, temporal query |
| [Event Sourcing Pattern — Azure Architecture Center](https://learn.microsoft.com/en-us/azure/architecture/patterns/event-sourcing) | Official docs | Detailed implementation guide, gotchas, snapshotting, idempotency |
| [CQRS — Martin Fowler](https://martinfowler.com/bliki/CQRS.html) | Reference article | Separating read/write models; pairs with event sourcing |
| [Event-Driven Architecture Patterns — Fowler 2017](https://martinfowler.com/articles/201701-event-driven.html) | Article | Event notification vs event-carried state transfer vs event sourcing |
| [Using Server-Sent Events — MDN](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events) | Official docs | SSE protocol, event fields, reconnection, named events |
| [Anthropic Messages Streaming API](https://platform.claude.com/docs/en/api/messages-streaming) | Official docs | Definitive event taxonomy for LLM streaming; tool_use streaming pattern |
| [SvelteKit Server Routes (Streaming)](https://svelte.dev/docs/kit/routing#server) | Official docs | ReadableStream in +server.ts; SSE endpoint in SvelteKit |
| [Svelte Stores](https://svelte.dev/docs/svelte/stores) | Official docs | writable/readable/derived stores; SSE subscription cleanup pattern |
| [WebSockets vs SSE — Ably](https://ably.com/blog/websockets-vs-sse) | Technical article | Trade-off comparison; when to choose each; reconnection |
| [SSE vs WebSockets — HTTP/2 angle (Smashing Magazine)](https://www.smashingmagazine.com/2018/02/sse-websockets-data-flow-http2/) | Technical article | HTTP/2 multiplexing; infrastructure considerations; mobile |
| [OpenHands Event Model](https://github.com/All-Hands-AI/OpenHands/blob/main/openhands/events/event.py) | Open source | Production AI coding agent event schema (Python dataclass) |
| [OpenHands Session Management](https://github.com/All-Hands-AI/OpenHands/blob/main/openhands/server/session/session.py) | Open source | WebSession queue architecture; _on_event; agent lifecycle |
| [OpenHands Conversation Store](https://github.com/All-Hands-AI/OpenHands/blob/main/openhands/storage/conversation/conversation_store.py) | Open source | Abstract persistence interface; batch retrieval pattern |
| [OpenHands Conversation Routes](https://github.com/All-Hands-AI/OpenHands/blob/main/openhands/server/routes/manage_conversations.py) | Open source | Create/list/start/stop/replay API design |
| [Redux Normalized State](https://redux.js.org/usage/structuring-reducers/normalizing-state-shape) | Official docs | Normalized event log structure; byId + allIds pattern |
| [Socket.IO How It Works](https://socket.io/docs/v4/how-it-works/) | Official docs | Transport fallback, namespace multiplexing, reconnection |
| [Socket.IO Rooms](https://socket.io/docs/v4/rooms/) | Official docs | Session isolation; routing agent events to specific clients |
| [VS Code Chat Sample](https://github.com/microsoft/vscode-extension-samples/tree/main/chat-sample) | Open source | Chat participant model; tool calling flow; streaming in panel UI |
| [React — Thinking in React](https://react.dev/learn/thinking-in-react) | Official docs | Component hierarchy for streaming data; state lifting; single source of truth |
| [OpenHands Event Serialization](https://github.com/All-Hands-AI/OpenHands/blob/main/openhands/events/serialization/event.py) | Open source | action vs observation discrimination; TOP_KEYS structure; args/extras pattern |

---

## Self-Evaluation

```json
{
  "coverage": 9,
  "diversity": 8,
  "examples": 9,
  "accuracy": 9,
  "gaps": [
    "Real-time collaborative multi-user coding (Live Share) not deeply covered — inapplicable to single-agent single-user case",
    "CodeSandbox Pitcher architecture not accessible (500/404 errors)",
    "AgentScope web UI component details not publicly documented"
  ]
}
```

---

*Generated by /learn from 22 sources analyzed.*
*See `resources/coding-session-architecture-sources.json` for full source metadata.*

