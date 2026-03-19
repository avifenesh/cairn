# Phase 6-8: Tools & Skills, Protocols, Channels, Intelligence

> Tools and skills first — the agent must be useful before it connects to others.
> Based on patterns from OpenCode, Gollem, ADK-Go research repos.

## Current State (updated 2026-03-19)

~400 backend tests, 227 frontend tests, 16 packages, ~30K lines of Go.
35 built-in tools (GLM+Vision) / 24 (other providers). 5 bundled SKILL.md files.

**Phase 6 COMPLETE** — backend PRs #21, #24, #26. Frontend PRs #28-50 (15/15 done).
**Phase 6.5 COMPLETE** — PR A (#37), PR B (#39).
**Phase 7 MCP Server COMPLETE** — PR #42 (24 tools + resources via mcp-go).
**Phase 7 Frontend COMPLETE** — PR #50 (MCP status, connections, external tool badges).
**Phase 8 Channels COMPLETE** — PR #46 (Telegram), #59 (Discord+Slack), #62 (frontend channel UI + feed actions).
**Z.ai MCP tools COMPLETE** — PR #49, #52 (web search, reader, zread — 5 HTTP tools).
**Z.ai Web Search FIXED** — PR #61 (GLM built-in web_search + SearXNG fallback chain).
**Z.ai Vision MCP COMPLETE** — PR #64 (8 tools via stdio subprocess, @z_ai/mcp-server).
**File Upload COMPLETE** — PR #65 (C.6 — paperclip, paste, drag-and-drop, preview chip, POST /v1/upload).
**Embeddings COMPLETE** — PR #63 (local Ollama nomic-embed-text 768d, hybrid search).
**Session Compaction COMPLETE** — PR #67 (SummaryBuffer 80K trigger, orphan stripping).
**Memory Edit/Delete COMPLETE** — PR #69 (DELETE + PUT /v1/memories/{id}, inline edit UI).
**Intelligence UI COMPLETE** — PR #71 (embeddings + compaction status in settings).
**Polish COMPLETE** — PR #68 (mode selector dropdown, drag-and-drop upload).
**Editable Settings COMPLETE** — PR #72 (PATCH /v1/config, file persistence, editable compaction/budget/channels).
**Voice COMPLETE** — PR #73 (backend Whisper STT + edge-tts), PR #74 (frontend VoiceButton wiring).
Phase 7 PRs 6-7 (MCP client, A2A) deferred. Remaining: auto-extract memories (PR 11c), Gmail/Calendar pollers (PR 11d).

---

## Phase 6: Tools & Skills (BACKEND COMPLETE)

### PR 1 — Backend: Web + Memory + Feed Tools ✅ MERGED (#21)

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

### PR 2 — Backend: Task + Communication Tools ✅ MERGED (#24)

| # | Tool | Inputs | Outputs | Pattern |
|---|------|--------|---------|---------|
| 6b.1 | `createTask` | description, type (default "general"), priority (0-9) | task ID | Wraps task.Engine.Submit |
| 6b.2 | `listTasks` | status (optional), type (optional), limit (default 10) | task list with status/description | Wraps task.Engine/Store.List |
| 6b.3 | `completeTask` | id, output (optional) | confirmation | Wraps task.Engine.Complete |
| 6b.4 | `compose` | title, body, priority (low/medium/high) | event ID | Creates feed event via signal.EventStore.Ingest source="agent" |
| 6b.5 | `getStatus` | (none) | JSON: uptime, poller status, memory stats, budget, active tasks, unread count | Aggregates from all services |
| 6b.6 | Tests | | | |

### PR 3 — Backend: Skill Tools + Bundled Skills ✅ MERGED (#26)

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
| 6f.1 | Tool call display upgrade | `ToolCallChip.svelte` | **DONE PR #29** — expandable output, duration badge, error state, disabled a11y. |
| 6f.2 | Inline memory creation | New: `QuickMemoryButton.svelte` | **DONE PR #34** — action bar "Remember this", category picker, POST /v1/memories. |
| 6f.3 | Inline feed actions | New: `FeedActionBar.svelte` | When agent shows feed items, show "Mark read" / "Mark all read" inline actions. |
| 6f.4 | Task creation from chat | New: `CreateTaskButton.svelte` | **DONE PR #38** — priority picker, POST /v1/tasks, response normalization. |

#### Memory View (`src/routes/memory/`, `src/lib/components/memory/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.5 | Memory create form | Extend `MemoryEditor.svelte` | **DONE PR #34** — categories from shared constants, aria-label. |
| 6f.6 | Memory search scores | Extend `MemorySearch.svelte` | **DONE PR #38** — score mapped to confidence, shown as progress bar. |
| 6f.7 | Batch accept/reject | New: `MemoryBatchActions.svelte` | **DONE PR #35** — multi-select checkboxes, bulk accept/reject, a11y. |

#### Skills View (`src/routes/skills/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.8 | Skill browser | Extend skills page | **DONE PR #32** — search/filter, expandable cards, inclusion badges, a11y. |
| 6f.9 | Skill detail | New: `SkillDetail.svelte` | Full SKILL.md rendered as markdown. "Load into chat" button. |
| 6f.10 | Active skill indicator | Extend `ChatPanel.svelte` | **DONE PR #40** — ActiveSkillChip, reads from skillStore.activeSkills. |

#### Ops View (`src/routes/ops/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.11 | System status card | New: `SystemStatus.svelte` | **DONE PR #29** — uptime, version, SSE status, costs. |
| 6f.12 | Task creation form | Extend ops page | **DONE PR #38** — TaskCreateForm with type/priority, POST /v1/tasks. |

#### Settings View (`src/routes/settings/`)

| # | What | Component | Details |
|---|------|-----------|---------|
| 6f.13 | Budget display | New: `BudgetCard.svelte` | **DONE PR #36** — progress bars, color-coded, in settings. |

#### Infrastructure

| # | What | File | Details |
|---|------|------|---------|
| 6f.14 | New API methods | `client.ts` | **DONE PR #40** — getSkillDetail, createTask. getPlugins/getBudget/getJournal remain. |
| 6f.15 | Skills store | New: `skills.svelte.ts` | **DONE PR #40** — skill list, active skills, selected skill, 5 tests. |
| 6f.16 | Status store | New: `status.svelte.ts` | **DONE PR #40** — budget, uptime, version, 3 tests. |
| 6f.17 | SSE: `budget_update` | `sse.svelte.ts` | **DONE PR #40** — routes to statusStore.setBudget. |
| 6f.18 | SSE: `tool_executed` | `sse.svelte.ts` | Pending — backend doesn't emit this event yet. |
| 6f.19-22 | Tests | | **DONE** — ToolCallChip (8), skills store (5), status store (3), + component tests. |

---

## Phase 6.5: Skill Activation System (COMPLETE)

> Research finding: Skills are prompt injection, NOT tool declaration (OpenCode pattern).
> `allowed-tools` in SKILL.md frontmatter SCOPES which tools are available, doesn't create new ones.
> Plugins (Go code, `internal/plugin/`) are the code-based tool-declaring extension layer.
> This matches Claude Code's architecture: skills/slash commands are the primary extensibility;
> MCP servers are secondary, for specific external service bridges.

### PR A — Backend: Skill Activation + Session Scoping ✅ MERGED (#37)

| # | What | Details | Pattern |
|---|------|---------|---------|
| 6.5a.1 | Active skill tracking | `Session.ActiveSkills []string`. `cairn.loadSkill` adds skill to session. | OpenCode: skill loaded into conversation |
| 6.5a.2 | Tool filtering | When skill with `allowed-tools` is active, only those tools + always-available tools sent to LLM. No skill = all tools. | OpenCode: permission gate |
| 6.5a.3 | Skill in system prompt | Active skill content injected into system prompt (not just tool output). Stacks with always-on skills. | OpenCode: `<skill_content>` blocks |
| 6.5a.4 | Bundled files | `cairn.loadSkill` lists files in skill directory (scripts, references). Agent reads with `cairn.readFile`. | OpenCode: `<skill_files>` listing |
| 6.5a.5 | Permission gate | `disable-model-invocation: true` skills require approval before activation. | OpenCode: `ctx.ask({ permission: "skill" })` |
| 6.5a.6 | Tests | | |

### PR B — Backend: Skill Install + Discovery ✅ MERGED (#39)

| # | What | Details | Pattern |
|---|------|---------|---------|
| 6.5b.1 | Multi-dir discovery | Scan `~/.cairn/skills/`, project `.cairn/skills/`, config `SKILL_DIRS` | OpenCode: global + project + config |
| 6.5b.2 | URL install | `cairn install skill <git-url>` clones into skills dir | OpenCode: `DiscoveryService.pull(url)` |
| 6.5b.3 | Validation | Vet `allowed-tools` against known tools, warn on `cairn.shell` without `disable-model-invocation` | Cairn convention |
| 6.5b.4 | Tests | | |

### PR C — Backend: Plugin Tool Declaration (optional, if needed later)

| # | What | Details | Pattern |
|---|------|---------|---------|
| 6.5c.1 | `Tools()` method on Plugin | Plugins can return `[]tool.Tool` registered at startup | OpenCode plugin: `tool: { key: ToolDefinition }` |
| 6.5c.2 | Hot-reload | Plugin tools re-registered on reload | ADK-Go plugin lifecycle |

---

## Phase 7: Protocols (MCP + A2A) — Reduced Scope

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

### PR 11 — Backend: Intelligence — SPLIT INTO 3 SUB-PRs

Full plan: `docs/design/INTELLIGENCE_PLAN.md` (108 research sources analyzed)

**PR 11a — Embeddings ✅ MERGED (#63)**
- OpenAIEmbedder (local Ollama nomic-embed-text, 768d)
- Backfill on startup, hybrid search active

**PR 11b — Session Compaction** (`internal/agent/compaction.go`)
- SummaryBufferMemory pattern (keep system + last N pairs + summarize old)
- Tool output truncation (60/40 head/tail)
- Orphaned tool result stripping
- Integration in react.go before LLM call

**PR 11c — Auto-Extract Memories** (`internal/memory/extractor.go`)
- Mem0 two-stage pipeline (extract facts → classify ADD/UPDATE/DELETE/NONE)
- Post-session fire-and-forget extraction
- Contradiction detection via embedding similarity + LLM judge

**PR 11d — Remaining Intelligence** (separate)
- Gmail + Calendar pollers (OAuth2)
- Voice: Whisper STT + TTS + endpoint

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
Phase 6 (DONE):
  PR 1 (Web+Memory+Feed tools)  ─── merged #21
  PR 2 (Task+Comm tools)        ─── merged #24
  PR 3 (Skill tools+bundles)    ─── merged #26
  PR 4 (Frontend Phase 6)       ─── merged PRs #28-50 (15/15 done)

Phase 6.5 (DONE):
  PR A (Skill activation)       ─── merged #37
  PR B (Skill install)          ─── merged #39

Phase 7 (MCP Server DONE, Client/A2A DEFERRED):
  PR 5 (MCP Server)             ─── merged #42
  PR 6 (MCP Client)             ─── deferred (Z.ai tools added directly instead)
  PR 7 (A2A Server)             ─── deferred
  PR 8 (Frontend Phase 7)       ─── merged #50

Z.ai Integration (DONE):
  PR 49 (Z.ai MCP tools)        ─── merged #49
  PR 52 (Accept header fix)     ─── merged #52
  PR 61 (Web search fix)        ─── merged #61 (GLM built-in + SearXNG fallback)
  PR 64 (Vision MCP)            ─── merged #64 (8 tools, stdio subprocess)

Phase 8 Channels (DONE):
  PR 9  (Framework + Telegram)  ─── merged #46
  PR 10 (Discord + Slack)       ─── merged #59
  PR 11 (Frontend channel UI)   ─── merged #62

Chat Features + Polish:
  PR 65 (File upload C.6)          ─── merged #65 (paperclip, paste, drag-and-drop)
  PR 68 (Mode selector + DnD)      ─── merged #68 (dropdown, drag-and-drop upload)
  PR 69 (Memory edit/delete)        ─── merged #69 (DELETE + PUT endpoints, inline edit)
  C.7 (Voice input/output)         ─── merged #73-78 (whisper STT + edge-tts TTS + Telegram voice)

Phase 8 Intelligence (ALL COMPLETE — see docs/design/INTELLIGENCE_PLAN.md):
  PR 11a (Embeddings)               ─── merged #63 (local Ollama nomic-embed-text, 768d)
  PR 11b (Session Compaction)        ─── merged #67 (SummaryBuffer 80K trigger)
  PR 11c (Auto-Extract Memories)     ─── merged #70 (Mem0 extract→classify pipeline)
  PR 11c+ (Contradiction Detection)  ─── merged #79 (LLM YES/NO judge in 0.8-0.9 zone)
  PR 71  (Frontend Intelligence UI)  ─── merged #71 (embeddings + compaction in settings)
  PR 73-78 (Voice STT/TTS)          ─── merged (whisper + edge-tts + Telegram + sanitization)

Feed Signal Intelligence (see docs/design/FEED_SIGNAL_PLAN.md):
  PR A (Wire Feed API + types)       ─── next (foundation — unblocks everything)
  PR B (GitHub Signal Intelligence)  ─── after A (external engagement + growth metrics + bot filter)
  PR C (Gmail + Calendar pollers)    ─── after A (filtered email + calendar via gws CLI)
  PR D (Future integrations spec)    ─── docs only (X, RSS, SO, cron)

Google Workspace:
  GWS tools                          ─── merged (cairn.gwsQuery + cairn.gwsExecute, 17 services)
```

## Summary

| Phase | Backend | Frontend | New capabilities |
|-------|---------|----------|-----------------|
| 6 | 3 PRs ✅ DONE | 15/15 ✅ DONE | Web, memory, feed, tasks, skills |
| 6.5 | 2 PRs ✅ DONE | — | Skill activation, install, validation |
| 7 | 1 PR ✅ DONE (MCP server) | 1 PR ✅ DONE | MCP tool exposure |
| 8 Channels | 3 PRs ✅ DONE | 1 PR ✅ DONE | Telegram + Discord + Slack, channel UI |
| 8 Intelligence | 4/4 PRs ✅ DONE | 1 PR ✅ DONE | Embeddings + compaction + settings UI |
| 8 Voice | 2 PRs ✅ DONE | 2 PRs ✅ DONE | Whisper STT + edge-tts TTS + playback button (C.7) |
| Z.ai HTTP | 3 PRs ✅ DONE | — | Web search (GLM built-in + SearXNG), reader, zread |
| Z.ai Vision | 1 PR ✅ DONE | — | 8 vision tools via stdio subprocess (GLM-4.6V) |
| File Upload | 1 PR ✅ DONE | 1 PR ✅ DONE | C.6 — paperclip, paste, drag-and-drop, POST /v1/upload |
| Polish | — | 4 PRs ✅ DONE | Mode dropdown, DnD, memory edit/delete, editable settings |
| **Total** | **28 merged** | **25/25 done** | **35 tools, 5 skills, MCP, 3 channels, file upload, voice + TTS, embeddings, compaction** |
