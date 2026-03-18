# Phase 6-8: Tools & Skills, Protocols, Channels, Intelligence

> Tools and skills first — the agent must be useful before it connects to others.
> Based on patterns from OpenCode, Gollem, ADK-Go research repos.

## Current State (updated 2026-03-18)

318 backend tests, 169 frontend tests, 14 packages, ~24,000 lines of Go.
24 built-in tools. 5 bundled SKILL.md files.

**Phase 6 backend COMPLETE** (PRs #21, #24, #26 — all merged).
Phase 6 frontend (PR 4), Phase 7, and Phase 8 remain.

---

## Phase 6: Tools & Skills

### PR 1 — Backend: Web + Memory + Feed Tools (`internal/tool/builtin/`)

| # | Tool | Inputs | Outputs | Pattern |
|---|------|--------|---------|---------|
| 6a.1 | `webSearch` | query, numResults (default 5) | title, url, snippet per result | OpenCode: HTTP POST to SearXNG/Exa, permission gate |
| 6a.2 | `webFetch` | url, format (text/markdown/html) | page content (truncated to 50K chars) | OpenCode: 5MB cap, HTML→markdown, Cloudflare retry |
| 6a.3 | `createMemory` | content, category (fact/preference/hard_rule/decision), scope | memory ID | Gollem: memory Put, wraps memory.Service.Create |
| 6a.4 | `searchMemory` | query, limit (default 10) | memories with relevance scores | Gollem: memory Search, wraps memory.Service.Search |
| 6a.5 | `manageMemory` | id, action (accept/reject/delete) | confirmation | Gollem: memory CRUD operations |
| 6a.6 | `readFeed` | source (optional), limit (default 20), unreadOnly (default true) | feed events | Wraps signal.EventStore.List |
| 6a.7 | `markRead` | id or "all" | count marked | Wraps signal.EventStore.MarkRead/MarkAllRead |
| 6a.8 | `digest` | (none) | summary + highlights + groups | Wraps signal.DigestRunner.Generate |
| 6a.9 | `journalSearch` | query (optional), hours (default 48) | journal entries with summaries | Wraps agent.JournalStore.Recent |
| 6a.10 | Tests — each tool with mock service deps | | | |

**Config:** `SEARXNG_URL`, `WEB_FETCH_TIMEOUT` (30s), `WEB_FETCH_MAX_SIZE` (5MB)

### PR 2 — Backend: Task + Communication Tools (`internal/tool/builtin/`)

| # | Tool | Inputs | Outputs | Pattern |
|---|------|--------|---------|---------|
| 6b.1 | `createTask` | description, type (default "general"), priority (0-9) | task ID | Wraps task.Engine.Submit |
| 6b.2 | `listTasks` | status (optional), type (optional), limit (default 10) | task list with status/description | Wraps task.Engine/Store.List |
| 6b.3 | `completeTask` | id, output (optional) | confirmation | Wraps task.Engine.Complete |
| 6b.4 | `compose` | title, body, priority (low/medium/high) | event ID | Creates feed event via signal.EventStore.Ingest source="agent" |
| 6b.5 | `getStatus` | (none) | JSON: uptime, poller status, memory stats, budget, active tasks, unread count | Aggregates from all services |
| 6b.6 | Tests | | | |

### PR 3 — Backend: Skill Tools + Bundled Skills

| # | What | Details |
|---|------|---------|
| 6c.1 | `loadSkill` tool | Input: name or search. Returns: skill content for context injection. Pattern: OpenCode skill.ts |
| 6c.2 | `listSkills` tool | Input: none. Returns: skills with name, description, inclusion. Pattern: OpenCode Skill.available() |
| 6c.3 | Plugin-provided tools | Extend plugin.Hooks with `Tools []tool.Tool`. Manager registers into tool.Registry at startup. Pattern: ADK-Go, Gollem |
| 6c.4 | Skill: `web-search` | Multi-step web research (search → fetch → summarize → cite) |
| 6c.5 | Skill: `code-review` | Review diffs for bugs, style, security |
| 6c.6 | Skill: `digest` | Prioritized feed digest generation |
| 6c.7 | Skill: `deploy` | Build, test, deploy workflow |
| 6c.8 | Skill: `self-review` | Agent reviews its own output before responding |
| 6c.9 | Tests | |

### PR 4 — Frontend: Tool & Skill UI

#### Chat View (`src/routes/chat/`, `src/lib/components/chat/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.1 | Tool call display upgrade | `ToolCallChip.svelte` | Add: expandable output preview (truncated), duration badge, error state (red). Click expands full output in modal. |
| 6f.2 | Inline memory creation | New: `QuickMemoryButton.svelte` | Message action bar button "Remember this". Popover with category selector. Calls `POST /v1/memories`. |
| 6f.3 | Inline feed actions | New: `FeedActionBar.svelte` | When agent shows feed items, show "Mark read" / "Mark all read" inline actions. |
| 6f.4 | Task creation from chat | New: `CreateTaskButton.svelte` | Message action bar "Create task". Pre-fills description from message. Calls `POST /v1/tasks`. |

#### Memory View (`src/routes/memory/`, `src/lib/components/memory/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.5 | Memory create form | Extend `MemoryEditor.svelte` | Category dropdown (fact/preference/hard_rule/decision/writing_style), scope selector (personal/project/global). |
| 6f.6 | Memory search scores | Extend `MemorySearch.svelte` | Relevance score as percentage badge. Highlight matching terms. |
| 6f.7 | Batch accept/reject | New: `MemoryBatchActions.svelte` | Multi-select proposed memories, accept/reject in batch. |

#### Skills View (`src/routes/skills/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.8 | Skill browser | Extend skills page | List with name, description, inclusion type, category badges. Search/filter. |
| 6f.9 | Skill detail | New: `SkillDetail.svelte` | Full SKILL.md rendered as markdown. "Load into chat" button. |
| 6f.10 | Active skill indicator | Extend `ChatPanel.svelte` | Chip below mode selector: "Active skill: web-search". |

#### Ops View (`src/routes/ops/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.11 | System status card | New: `SystemStatus.svelte` | Budget (spend vs cap), poller status (last poll, errors), memory stats, unread count. SSE-updated. |
| 6f.12 | Task creation form | Extend ops page | "New Task" button → form with description, type, priority. |

#### Settings View (`src/routes/settings/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.13 | Budget display | New: `BudgetCard.svelte` | Spend vs cap progress bar (green/yellow/red). Daily + weekly. |

#### Infrastructure

| # | What | File | Details |
|---|------|------|---------|
| 6f.14 | New API methods | `client.ts` | `getPlugins()`, `getPluginStats()`, `getBudget()`, `getJournal(hours)`, `getSkillDetail(name)` |
| 6f.15 | Skills store | New: `skills.svelte.ts` | Skill list, active skill state, load/search actions |
| 6f.16 | Status store | New: `status.svelte.ts` | Budget, pollers, memory stats. SSE `budget_update` handler |
| 6f.17 | SSE: `budget_update` | `sse.svelte.ts` | Updates status store after each LLM call |
| 6f.18 | SSE: `tool_executed` | `sse.svelte.ts` | Updates ToolCallChip with result/duration |
| 6f.19-22 | Tests | | ToolCallChip, MemoryBatchActions, skills store, status store |

---

## Phase 7: Protocols (MCP + A2A)

### PR 5 — Backend: MCP Server (`internal/mcp/`)

| # | What |
|---|------|
| 7a.1 | MCP server core (mcp-go), register all Cairn tools as MCP tools |
| 7a.2 | Resources: feed events, memories, sessions |
| 7a.3 | Transport: stdio (Claude Code, Cursor) |
| 7a.4 | Transport: HTTP/SSE (remote, port 3001) |
| 7a.5 | Session-scoped tool filtering (mcp-go pattern) |
| 7a.6 | Write rate limiting (ToolHandlerMiddleware) |
| 7a.7 | Tests |

### PR 6 — Backend: MCP Client (`internal/mcp/`)

| # | What |
|---|------|
| 7b.1 | Connect to external MCP servers, discover tools |
| 7b.2 | Wrap MCP tools as Cairn tool.Tool (ADK-Go mcptoolset pattern) |
| 7b.3 | Confirmation flow for dangerous tools |
| 7b.4 | Config: `MCP_SERVERS` JSON array |
| 7b.5 | Lifecycle: connect/reconnect/close |
| 7b.6 | Tests |

### PR 7 — Backend: A2A Server (`internal/a2a/`)

| # | What |
|---|------|
| 7c.1 | Agent card (`/.well-known/agent.json`) |
| 7c.2 | Task submission (`POST /a2a/tasks`) |
| 7c.3 | Task status + streaming results |
| 7c.4 | Tests |

### PR 8 — Frontend: Protocol UI

| # | What | View | Component |
|---|------|------|-----------|
| 7f.1 | MCP server status | Settings | `McpStatus.svelte` — enabled, client count, transport |
| 7f.2 | MCP client connections | Settings | `McpConnections.svelte` — server list, tool counts, status |
| 7f.3 | External tool badge | Chat | ToolCallChip: `[external]` badge for MCP tools |
| 7f.4 | A2A tasks | Ops | Task list: `[a2a]` badge, source agent info |
| 7f.5 | API methods | client.ts | `getMcpStatus()`, `getMcpConnections()` |
| 7f.6 | Tests | | |

---

## Phase 8: Channels + Intelligence

### PR 9 — Backend: Channels + Telegram (`internal/channel/`)

| # | What |
|---|------|
| 8a.1 | Channel interface + message types |
| 8a.2 | Router: session → channel tracking |
| 8a.3 | Markdown normalization (CommonMark → Telegram V2 / plain) |
| 8a.4 | Web adapter (wraps SSE + REST) |
| 8a.5 | Telegram adapter (telego, commands, keyboards) |
| 8a.6 | Notification router (priority, quiet hours) |
| 8a.7 | Tests |

### PR 10 — Frontend: Channel UI

| # | What | View | Component |
|---|------|------|-----------|
| 8af.1 | Telegram config | Settings | `TelegramConfig.svelte` — token, chat ID, test button |
| 8af.2 | Notification prefs | Settings | `NotificationPrefs.svelte` — quiet hours, channel priority |
| 8af.3 | Channel indicator | Header | `ChannelBadge.svelte` — active channel icon |
| 8af.4 | API + tests | | |

### PR 11 — Backend: Intelligence (`internal/memory/`, `internal/signal/`, `internal/voice/`)

| # | What |
|---|------|
| 8b.1 | Real embedding provider (OpenAI/GLM API) |
| 8b.2 | Vectors on memory create |
| 8b.3 | Session compaction (LLM summarize old events) |
| 8b.4 | Gmail + Calendar pollers (OAuth2) |
| 8b.5 | Voice: Whisper STT + TTS + endpoint |
| 8b.6 | Tests |

### PR 12 — Frontend: Intelligence UI

| # | What | View | Component |
|---|------|------|-----------|
| 8bf.1 | Voice input | Chat | Extend `VoiceButton.svelte` — MediaRecorder → POST /v1/voice |
| 8bf.2 | Gmail events | Feed | `FeedItem.svelte` — email subject, sender, snippet |
| 8bf.3 | Calendar events | Feed | `FeedItem.svelte` — time, title, location |
| 8bf.4 | Compaction indicator | Chat | `SessionPicker.svelte` — "compacted" badge |
| 8bf.5 | Embedding status | Settings | Model, vector count, enabled state |
| 8bf.6 | API + tests | | |

---

## Dependency Graph

```
PR 1 (Web+Memory+Feed tools) ─┐
PR 2 (Task+Comm tools)         ├── parallel backend
                                │
PR 3 (Skill tools+bundles) ────┘ after PR1/2
PR 4 (Frontend Phase 6) ────────  needs PR1-3

PR 5 (MCP Server) ──┐
PR 6 (MCP Client) ──┤ parallel, needs Phase 6
PR 7 (A2A Server) ──┘
PR 8 (Frontend Phase 7) ────────  needs PR5-7

PR 9 (Channels+Telegram) ─────── independent
PR 10 (Frontend Channels) ─────── needs PR9

PR 11 (Intelligence) ───────────── independent
PR 12 (Frontend Intelligence) ──── needs PR11
```

## Summary

| Phase | Backend | Frontend | New capabilities |
|-------|---------|----------|-----------------|
| 6 | 3 PRs (15 tools + 5 skills) | 1 PR (22 subphases) | Web, memory, feed, tasks, skills |
| 7 | 3 PRs (MCP + A2A) | 1 PR (6 subphases) | External agents, external tools |
| 8 | 2 PRs (channels + intelligence) | 2 PRs (10 subphases) | Telegram, embeddings, Gmail, voice |
| **Total** | **8 PRs** | **4 PRs** | |
