# Coding Session Panel - Design Document

## Overview

A dedicated UI panel for observing, steering, and reviewing AI coding sessions in real-time. Supports both interactive chat sessions (enhanced tool visibility) and autonomous background sessions (dedicated panel with full observability).

**Core principle**: Tool calls are the atomic unit of display. The panel shows what the agent is thinking, what tools it calls, what files it changes, and lets the user steer or approve at any point.

## Problem

Today Cairn has:
- Chat panel: shows agent text responses but tool calls are opaque (just a mention in text)
- Activity page: shows post-hoc activity entries (summary + tool count), not real-time events
- SSE broadcaster: streams `assistant_delta`, `assistant_tool_call`, `assistant_reasoning` - but no UI consumes them for live observability

Missing:
- No way to watch an idle/background coding session in real-time
- No live diff viewer for file changes as they happen
- No steering (inject directions mid-session)
- No inline approval with diff context
- No session-scoped event timeline

## Architecture

```
Agent ReAct Loop
    |
    | emits RunEvents (Event with Parts: TextPart, ToolPart, ReasoningPart)
    v
EventBus (existing)
    |
    | TextDelta, ToolCallEvent, ReasoningDelta, StreamStarted/Ended
    v
SSE Broadcaster (existing) ──── text/event-stream ────> Browser
    |                                                        |
    | NEW: session-scoped events                    EventSource API
    | NEW: file_change, approval_request            (existing SSE store)
    v                                                        |
Session Event Store (NEW)                           Session Panel Store (NEW)
    |                                                        |
    | persist to SQLite                             Svelte 5 runes
    | replay on reconnect                                    |
    v                                               Coding Session Panel (NEW)
                                                     - Activity Stream
                                                     - Diff Viewer
                                                     - Steering Input
                                                     - Approval Cards
```

## What Already Exists (leverage, don't rebuild)

| Component | Location | What it provides |
|-----------|----------|-----------------|
| `EventBus` | `internal/eventbus/` | Typed pub/sub with `Subscribe[E]` generics |
| `SSEBroadcaster` | `internal/server/sse.go` | Broadcasts bus events as SSE, replay buffer (1000 events) |
| `Event` / `RunEvent` | `internal/agent/types.go` | `TextPart`, `ToolPart` (with status lifecycle), `ReasoningPart` |
| `ToolCallEvent` | `internal/eventbus/events.go` | `TaskID`, `ToolName`, `Phase` (start/end) |
| `TextDelta` / `ReasoningDelta` | `internal/eventbus/events.go` | Streaming text and reasoning chunks |
| `ActivityStore` | `internal/agent/activity_store.go` | Post-hoc activity entries + per-tool stats |
| `Session` / `SessionStore` | `internal/agent/session_store.go` | Session CRUD, event append, history replay |
| SSE frontend store | `frontend/src/lib/stores/sse.svelte.ts` | EventSource subscription, event dispatch |
| Chat panel | `frontend/src/routes/chat/` | Existing chat UI with streaming text |
| Activity page | `frontend/src/routes/activity/` | Activity entries + tool stats |

## Implementation Plan

### Phase 1: Backend - Session Event Stream (Go)

#### 1.1 New EventBus events

Add to `internal/eventbus/events.go`:

```go
// SessionEvent is emitted for every observable action in a coding session.
type SessionEvent struct {
    EventMeta
    SessionID string `json:"sessionId"`
    EventType string `json:"eventType"` // tool_call, tool_result, file_change, text_delta, thinking, state_change, approval_request, user_steer
    Payload   any    `json:"payload"`
}
```

#### 1.2 Emit session events from ReAct loop

Modify `internal/agent/react.go` - at each stage of the ReAct loop, publish `SessionEvent` through the bus. Key emission points:

| ReAct Stage | SessionEvent.EventType | Payload |
|-------------|----------------------|---------|
| LLM text delta | `text_delta` | `{ text: string }` |
| LLM reasoning delta | `thinking` | `{ text: string, round: int }` |
| Tool call start | `tool_call` | `{ toolId, toolName, input }` |
| Tool call end | `tool_result` | `{ toolId, output, isError, durationMs }` |
| File written by tool | `file_change` | `{ path, operation, diff }` |
| Agent state change | `state_change` | `{ state: running|paused|waiting|completed|failed }` |
| Round complete | `round_complete` | `{ round, toolCalls, tokensUsed }` |

The existing `ToolCallEvent`, `TextDelta`, `ReasoningDelta` events continue to work. `SessionEvent` is a new unified envelope that carries session-scoped context.

#### 1.3 Session-scoped SSE endpoint

New endpoint: `GET /v1/sessions/{id}/stream`

- Filters SSE events to only those matching the session ID
- Supports `Last-Event-ID` header for reconnection replay
- Keepalive pings every 15s
- Returns 404 if session doesn't exist

Implementation in `internal/server/routes.go` - add a new handler that creates a filtered SSE subscription.

#### 1.4 Steering endpoint

New endpoint: `POST /v1/sessions/{id}/steer`

```json
{
  "content": "Focus on the API endpoint, skip tests for now",
  "priority": "normal"  // "normal" | "urgent" | "stop"
}
```

- `normal`: queued for injection at next tool-call boundary
- `urgent`: injected immediately (cancels current LLM generation)
- `stop`: cancels the session

Implementation: add a steering channel to `InvocationContext`. The ReAct loop checks it between rounds. The session store records steering messages as events.

#### 1.5 File change detection

Hook into the existing `writeFile`, `editFile`, `shell` tools to detect file modifications. When a tool writes a file:

1. Compute unified diff (using `go-diff` or `exec git diff`)
2. Emit `SessionEvent` with `file_change` type
3. Include path, operation (write/delete/rename), and diff string

This requires a thin wrapper or plugin hook on file-writing tools. The cleanest approach: add a `PostToolUse` plugin that checks if the tool is file-modifying and computes the diff.

### Phase 2: Frontend - Session Panel (Svelte 5)

#### 2.1 New route: `/session/[id]`

Dedicated coding session page. Layout:

```
+------------------------------------------------------------------+
| Session Header                                                     |
| [title] [status pill] [elapsed] [tokens] [Stop] [Pause]          |
+---------------------------+--------------------+------------------+
|                           |                    |                  |
| Activity Stream           | Diff Viewer        | Steering        |
| (scrollable, auto-scroll) | (selected file)    | Sidebar         |
|                           |                    |                  |
| > [thinking] Planning...  | File: handler.go   | [approval card] |
| > [tool] readFile main.go | - old line         |                  |
| > [tool] editFile handler | + new line         | [user msg]      |
| > [tool] shell go test    |                    | [agent ack]     |
| > [approval] git push     |                    |                  |
|                           |                    | [input bar]     |
+---------------------------+--------------------+------------------+
| Footer: Round 3/40 | Tools: 12 | Files: 3 | Cost: $0.42         |
+------------------------------------------------------------------+
```

Three-panel resizable layout. On narrow screens, collapse to tabbed view (Activity | Diffs | Steer).

#### 2.2 Session store (`session.svelte.ts`)

```typescript
// Svelte 5 runes-based session store
let events = $state<SessionEvent[]>([]);
let sessionMeta = $state<SessionMeta | null>(null);
let agentStatus = $derived(/* last state_change event */);
let fileChanges = $derived(/* filter file_change events */);
let pendingApprovals = $derived(/* unresolved approval_request events */);
let currentRound = $derived(/* last round_complete event */);
```

SSE subscription via `EventSource` to `/v1/sessions/{id}/stream`. Auto-reconnect with `Last-Event-ID`.

#### 2.3 Activity Stream component

Renders session events as cards:

| Event Type | Card Design |
|-----------|-------------|
| `text_delta` | Streaming text with typing cursor, muted color |
| `thinking` | Collapsible gray italic block, collapsed by default |
| `tool_call` | Tool name badge + input summary + spinner |
| `tool_result` | Attached to tool_call card: output preview + duration + success/error indicator |
| `file_change` | File path + operation badge (A/M/D) + "View Diff" button |
| `state_change` | Status pill transition chip |
| `approval_request` | Amber card with action preview + Approve/Reject buttons |
| `user_steer` | Blue italic message from user |

Auto-scroll to bottom on new events. Manual scroll-up pauses auto-scroll (with "Jump to latest" button).

#### 2.4 Diff Viewer component

Options (from research - pick one):
- **diff2html** (recommended): lightweight, renders unified diff strings as HTML with syntax highlighting. No heavy editor dependency. Supports side-by-side and unified views.
- **Monaco DiffEditor**: richest experience but heavy (~2MB). Use only if we want inline editing.

For v1: **diff2html** with unified view. Rendered in the middle panel. File tree on top shows changed files with A/M/D badges. Click a file to view its diff.

#### 2.5 Steering sidebar

- Persistent input bar at bottom
- Quick action buttons: Stop (red), Pause (amber), Explain (gray)
- Approval cards appear inline when agent hits a gate
- User messages echo in the sidebar with timestamp
- Agent acknowledgment appears after steering is processed

#### 2.6 Chat panel enhancement

For chat sessions (not idle/background), enhance the existing chat panel:
- Add a collapsible "Activity" sidebar that shows tool calls in real-time
- Show inline diff previews for file changes
- Approval cards appear inline in chat bubbles

This is lighter than the full session panel - it augments the existing chat UI.

### Phase 3: Session List and Navigation

#### 3.1 Sessions list page or section

Add to `/ops` or as a new `/sessions` route:
- List of active and recent sessions
- Status indicators (running/paused/waiting/completed/failed)
- Click to open the session panel
- Badge count for pending approvals

#### 3.2 Navigation from other pages

- `/today` command center: "Active Sessions" section with status pills
- `/chat`: "View Session" button to open full panel for current chat
- `/activity`: link from activity entries to their session panel
- Notification toast when a background session needs approval

### Phase 4: Persistence and Replay

#### 4.1 Session events table

```sql
CREATE TABLE session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    event_type TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'agent',
    payload JSON NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
CREATE INDEX idx_session_events_session ON session_events(session_id, id);
```

#### 4.2 Replay on reconnect

When the frontend connects to `/v1/sessions/{id}/stream`:
1. If `Last-Event-ID` header present, replay events after that ID
2. Otherwise, replay last N events (configurable, default 100)
3. Then switch to live streaming

#### 4.3 Session summary on completion

When a session ends, generate a summary:
- Files changed (with final diffs)
- Tool calls made (count by tool)
- Duration and cost
- Steering messages and approvals
- Store as the session's `result` field

## File Changes Summary

### New Files

| File | What |
|------|------|
| `internal/agent/session_events.go` | SessionEvent types, file change detection, steering channel |
| `internal/server/session_stream.go` | Session-scoped SSE endpoint + steering route |
| `frontend/src/routes/session/[id]/+page.svelte` | Coding session panel page |
| `frontend/src/lib/stores/session.svelte.ts` | Session event store (Svelte 5 runes) |
| `frontend/src/lib/components/session/ActivityStream.svelte` | Event feed component |
| `frontend/src/lib/components/session/DiffViewer.svelte` | Diff rendering (diff2html) |
| `frontend/src/lib/components/session/SteeringSidebar.svelte` | Steering input + approvals |
| `frontend/src/lib/components/session/SessionHeader.svelte` | Status, controls, metrics |
| `frontend/src/lib/components/session/EventCard.svelte` | Individual event cards |
| `frontend/src/lib/components/session/FileTree.svelte` | Changed files list with badges |

### Modified Files

| File | Change |
|------|--------|
| `internal/eventbus/events.go` | Add `SessionEvent` type |
| `internal/agent/react.go` | Emit `SessionEvent` at each ReAct stage |
| `internal/agent/types.go` | Add steering channel to `InvocationContext` |
| `internal/server/routes.go` | Register session stream + steer routes |
| `internal/server/sse.go` | Add `SessionEvent` subscription |
| `frontend/src/lib/types.ts` | Add session event types |
| `frontend/src/lib/api/client.ts` | Add session API methods |
| `frontend/src/lib/stores/sse.svelte.ts` | Handle session events |
| `frontend/src/routes/chat/+page.svelte` | Add collapsible activity sidebar |
| `frontend/src/routes/today/+page.svelte` | Add "Active Sessions" section |

### Dependencies

| Package | Purpose | New? |
|---------|---------|------|
| `diff2html` | Render unified diffs as HTML | Yes (frontend) |
| `diff` (jsdiff) | Compute diffs client-side if needed | Yes (frontend) |

No new Go dependencies - use `os/exec` with `git diff` for file change diffs (git is already available).

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v1/sessions` | Read | List sessions with status |
| GET | `/v1/sessions/{id}` | Read | Session metadata + summary |
| GET | `/v1/sessions/{id}/stream` | Read | SSE event stream (session-scoped) |
| GET | `/v1/sessions/{id}/events` | Read | Paginated event history |
| POST | `/v1/sessions/{id}/steer` | Write | Inject steering message |
| GET | `/v1/sessions/{id}/files` | Read | List changed files with diffs |

## Event Types

```typescript
type SessionEventType =
    | 'text_delta'          // LLM text chunk
    | 'thinking'            // Reasoning/chain-of-thought
    | 'tool_call'           // Tool invocation start
    | 'tool_result'         // Tool execution result
    | 'file_change'         // File written/deleted/renamed
    | 'state_change'        // Agent state transition
    | 'round_complete'      // ReAct round finished
    | 'approval_request'    // Agent needs human approval
    | 'approval_response'   // Human approved/rejected
    | 'user_steer'          // User steering message
    | 'session_start'       // Session created
    | 'session_end';        // Session completed/failed
```

## Implementation Order

1. **Backend events** - `SessionEvent` type + emit from ReAct loop (1-2 days)
2. **Session-scoped SSE** - filtered endpoint + replay (1 day)
3. **Steering backend** - channel + injection + route (1 day)
4. **Frontend store** - session store + SSE subscription (1 day)
5. **Activity Stream** - event cards + auto-scroll (1-2 days)
6. **Diff Viewer** - diff2html integration + file tree (1-2 days)
7. **Steering sidebar** - input + approvals + quick actions (1 day)
8. **Session header** - status, controls, metrics (0.5 day)
9. **Chat enhancement** - collapsible activity sidebar (1 day)
10. **Session list** - /sessions or /ops integration (0.5 day)
11. **Persistence** - session_events table + replay (1 day)
12. **File change detection** - PostToolUse diff computation (1 day)

## Verification

1. `go vet ./...` - clean
2. `go test -race ./...` - all pass
3. `cd frontend && pnpm check && pnpm test` - 0 errors
4. Manual: start a chat, verify tool calls appear in activity sidebar
5. Manual: start an idle coding task, open `/session/{id}`, verify live event stream
6. Manual: send a steering message, verify agent adjusts behavior
7. Manual: trigger an approval gate, verify card appears with diff context
8. Manual: disconnect and reconnect, verify events replay correctly
9. Manual: complete a session, verify summary generated

## Research References

Detailed research in `agent-knowledge/`:
- `coding-session-observability.md` - Platform comparison (Claude Code, Devin, OpenHands, Cursor)
- `coding-session-steering.md` - HITL patterns (LangGraph interrupt, OpenHands ConfirmRisky)
- `coding-session-diff-ui.md` - Diff libraries (diff2html, Monaco, CodeMirror, jsdiff, Shiki)
- `coding-session-architecture.md` - Event sourcing, SSE, data models, Svelte patterns
