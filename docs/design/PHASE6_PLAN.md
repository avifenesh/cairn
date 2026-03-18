# Phase 6-8: Protocols, Channels & Intelligence

> Phases 1-5 built the core. Phases 6-8 make Cairn connected, multi-channel, and production-grade.

## Current State

242 backend tests, 169 frontend tests, 13 packages, ~19,500 lines of Go.
Phases 1-5 complete: event bus, LLM, tools, tasks, memory, agent (loop+journal+reflection), signal plane (5 pollers+webhooks+digest), server, skills, CI/CD, Go embed frontend.

---

## Phase 6: Protocols (MCP + A2A)

**Goal:** Cairn speaks MCP and A2A. Other agents can use its tools, it can use external tools, agents can submit tasks.

### 6a: MCP Server (`internal/mcp/`)

| # | What |
|---|------|
| 6a.1 | MCP server core (mcp-go), tool listing + execution |
| 6a.2 | Resource providers: feed events, memories, sessions |
| 6a.3 | Transport: stdio (Claude Code, Cursor) |
| 6a.4 | Transport: HTTP/SSE (remote, port 3001) |
| 6a.5 | Per-session write rate limiting |
| 6a.6 | Tests |

### 6b: MCP Client (`internal/mcp/`)

| # | What |
|---|------|
| 6b.1 | Connect to external MCP servers, discover tools |
| 6b.2 | Wrap MCP tools as Cairn tool.Tool interface |
| 6b.3 | Config-driven registration (`MCP_SERVERS` JSON) |
| 6b.4 | Lifecycle: connect on startup, reconnect, close on shutdown |
| 6b.5 | Tests |

### 6c: A2A Server (`internal/a2a/`)

| # | What |
|---|------|
| 6c.1 | Agent card (`/.well-known/agent.json`) |
| 6c.2 | Task submission (`POST /a2a/tasks`) |
| 6c.3 | Task status (`GET /a2a/tasks/:id`) |
| 6c.4 | Streaming results via SSE |
| 6c.5 | Tests |

**Frontend:** MCP connections panel in Settings. A2A tasks in Ops view.
**Config:** `MCP_SERVER_ENABLED`, `MCP_PORT` (3001), `MCP_SERVERS`, `A2A_ENABLED`

---

## Phase 7: Channel Adapters

**Goal:** Interact with Cairn from Telegram. Messages follow the user across channels.

### 7a: Channel Core (`internal/channel/`)

| # | What |
|---|------|
| 7a.1 | Channel interface, IncomingMessage, OutgoingMessage types |
| 7a.2 | Channel router (session -> active channel tracking) |
| 7a.3 | Markdown normalization (CommonMark -> Telegram V2 / Slack / plain) |
| 7a.4 | Web adapter (wraps existing SSE + REST) |
| 7a.5 | Tests |

### 7b: Telegram Adapter (`internal/channel/telegram/`)

| # | What |
|---|------|
| 7b.1 | Bot setup (telego, webhook or long-poll) |
| 7b.2 | Chat (messages -> agent, streaming via message edit) |
| 7b.3 | Commands (/chat, /tasks, /memory, /digest, /settings) |
| 7b.4 | Approvals (InlineKeyboard for approve/deny) |
| 7b.5 | Voice (receive voice -> whisper STT -> agent) |
| 7b.6 | Files (send/receive documents) |
| 7b.7 | Tests |

### 7c: Notification Router (`internal/channel/`)

| # | What |
|---|------|
| 7c.1 | Priority levels (critical, high, medium, low) |
| 7c.2 | Presence tracking (which channels user is on) |
| 7c.3 | Quiet hours support |
| 7c.4 | Low-priority items queued for digest |
| 7c.5 | Tests |

**Frontend:** Telegram config in Settings. Notification preferences. Channel indicator.
**Config:** `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `NOTIFICATION_QUIET_START`, `NOTIFICATION_QUIET_END`

---

## Phase 8: Intelligence & Polish

**Goal:** Production-grade memory, remaining signal sources, voice.

### 8a: Embeddings + Compaction (`internal/memory/`)

| # | What |
|---|------|
| 8a.1 | Embedding provider (OpenAI/GLM API or local HTTP) |
| 8a.2 | Generate vectors on memory create |
| 8a.3 | Hybrid search upgrade (vector + keyword, MMR done) |
| 8a.4 | Session compaction (LLM summarizes old events) |
| 8a.5 | Context builder (token-budgeted memory + journal injection) |
| 8a.6 | Tests |

### 8b: Gmail + Calendar (`internal/signal/`)

| # | What |
|---|------|
| 8b.1 | OAuth2 token flow + credential storage |
| 8b.2 | Gmail poller (list/get, label filtering) |
| 8b.3 | Gmail push (Pub/Sub webhook, optional) |
| 8b.4 | Calendar poller (upcoming events, free/busy) |
| 8b.5 | Tests |

### 8c: Voice Pipeline (`internal/voice/`)

| # | What |
|---|------|
| 8c.1 | Whisper STT (HTTP to whisper.cpp or API) |
| 8c.2 | TTS output (OpenAI/ElevenLabs or local) |
| 8c.3 | Voice endpoint (POST /v1/voice -> transcribe -> agent -> TTS) |
| 8c.4 | Wire with Telegram voice (7b.5) |
| 8c.5 | Tests |

**Frontend:** Voice button in chat. Gmail/calendar in feed. Session compaction indicator.
**Config:** `EMBEDDING_PROVIDER`, `GOOGLE_CLIENT_ID`, `GMAIL_ENABLED`, `CALENDAR_ENABLED`, `WHISPER_URL`, `TTS_PROVIDER`

---

## Dependency Graph

```
Phase 6a (MCP Server) ──┐
Phase 6b (MCP Client) ──┤ parallel
Phase 6c (A2A Server) ──┘
                         │
Phase 7a (Channel Core) ──→ Phase 7b (Telegram) ──→ Phase 8c (Voice)
              │
              └──→ Phase 7c (Notifications)

Phase 8a (Embeddings) ── independent
Phase 8b (Gmail/Cal)  ── independent
```

## PR Plan (9 PRs)

| PR | Phase | Content |
|----|-------|---------|
| 1 | 6a | MCP server (mcp-go, stdio+HTTP, resources, rate limiting) |
| 2 | 6b | MCP client (external servers as tools) |
| 3 | 6c | A2A server (agent card, task submission, streaming) |
| 4 | 7a | Channel core (interface, router, markdown normalization) |
| 5 | 7b | Telegram adapter (chat, commands, keyboards, voice, files) |
| 6 | 7c | Notification router (priority, presence, quiet hours) |
| 7 | 8a | Embeddings + session compaction + context builder |
| 8 | 8b | Gmail + Google Calendar (OAuth2, pollers) |
| 9 | 8c | Voice pipeline (Whisper STT + TTS) |
