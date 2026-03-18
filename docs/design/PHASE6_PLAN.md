# Phase 6-8: Tools & Skills, Protocols, Channels, Intelligence

> Tools and skills first — the agent must be useful before it connects to others.

## Current State

269 backend tests, 169 frontend tests, 14 packages, ~21,100 lines of Go.
Foundation complete: event bus, LLM, tools (8 filesystem), tasks, memory (context builder), agent (loop+journal+reflection+plugin hooks), signal plane (5 pollers+webhooks+digest), server, skills infrastructure, CI/CD.

**Gap**: Cairn has 8 filesystem tools. The agent can't search the web, manage memories, interact with its feed, organize tasks, or invoke skills. These are what make an agent useful.

---

## Phase 6: Tools & Skills (agent becomes useful)

**Goal:** Complete tool suite for real work. Based on OpenCode (websearch/webfetch/task/skill), Gollem (memory/planning/stateful), ADK-Go (memory load/MCP bridge).

### 6a: Web + Memory + Feed Tools

| # | Tool | What | Pattern |
|---|------|------|---------|
| 6a.1 | webSearch | Search via SearXNG or Exa API | OpenCode: HTTP POST, permission gate |
| 6a.2 | webFetch | Fetch URL, HTML to markdown, 5MB cap | OpenCode: format negotiation, Cloudflare retry |
| 6a.3 | createMemory | Create memory with category/scope | Gollem: memory Put |
| 6a.4 | searchMemory | RAG search with scores | Gollem: memory Search |
| 6a.5 | manageMemory | Accept/reject/delete memories | Gollem: memory CRUD |
| 6a.6 | readFeed | List feed events with filters | Service wrapper |
| 6a.7 | markRead | Mark events read (single or all) | Service wrapper |
| 6a.8 | digest | LLM digest of unread events | Existing DigestRunner |
| 6a.9 | journalSearch | Search episodic memory | ADK-Go: load memory |
| 6a.10 | Tests | | |

### 6b: Task + Communication Tools

| # | Tool | What | Pattern |
|---|------|------|---------|
| 6b.1 | createTask | Submit task to queue | OpenCode: task spawner |
| 6b.2 | listTasks | List with status/type filters | Service wrapper |
| 6b.3 | completeTask | Mark done with output | Service wrapper |
| 6b.4 | compose | Message to user's feed | Feed event creation |
| 6b.5 | getStatus | System status (pollers, budget, memory) | Service aggregation |
| 6b.6 | Tests | | |

### 6c: Skill Tools + Bundled Skills

| # | What | Pattern |
|---|------|---------|
| 6c.1 | loadSkill tool — discover and inject skill content | OpenCode: skill.ts |
| 6c.2 | listSkills tool — list available skills | OpenCode: Skill.available() |
| 6c.3 | Plugin-provided tools — Hooks.Tools registration | ADK-Go: tools in config |
| 6c.4 | 5 bundled skills: web-search, code-review, digest, deploy, self-review | |
| 6c.5 | Tests | |

### 6f: Frontend

| # | What | View |
|---|------|------|
| 6f.1 | Tool execution display (name, status, duration, output) | Chat |
| 6f.2 | Memory management (create, search, accept/reject) | Memory + Chat |
| 6f.3 | Feed actions from chat (mark read, digest) | Chat + Feed |
| 6f.4 | Task creation from chat | Chat + Ops |
| 6f.5 | Skill browser (list, load into context) | Skills |
| 6f.6 | System status (budget, pollers, memory stats) | Settings/Ops |

---

## Phase 7: Protocols (MCP + A2A)

### 7a: MCP Server | 7b: MCP Client | 7c: A2A Server

### 7f: Frontend — MCP connections, external tools, A2A tasks

---

## Phase 8: Channels + Intelligence

### 8a: Channel Core + Telegram + Notifications
### 8b: Embeddings + Compaction + Gmail + Voice

### 8f: Frontend — Telegram config, voice button, Gmail feed

---

## PR Plan (12 PRs)

| PR | Phase | What | Who |
|----|-------|------|-----|
| 1 | 6a | Web + Memory + Feed tools | Backend |
| 2 | 6b | Task + Communication tools | Backend |
| 3 | 6c | Skill tools + bundled skills | Backend |
| 4 | 6f | Tool display, memory UI, feed actions, skill browser | Frontend |
| 5 | 7a | MCP server | Backend |
| 6 | 7b | MCP client | Backend |
| 7 | 7c | A2A server | Backend |
| 8 | 7f | MCP/A2A UI | Frontend |
| 9 | 8a | Channels + Telegram + notifications | Backend |
| 10 | 8af | Telegram config, notification prefs | Frontend |
| 11 | 8b | Embeddings + compaction + Gmail + voice | Backend |
| 12 | 8bf | Voice button, Gmail feed, compaction UI | Frontend |

## Design Principles (from research)

1. **Tools are service wrappers** — call through existing services, not direct DB (all repos)
2. **Permission gates on dangerous tools** — OpenCode ctx.ask(), ADK-Go confirmation flow
3. **Output truncation** — stay within LLM context (OpenCode, Eino large_tool_result)
4. **Stateful tools** — some maintain state across rounds (Gollem ExportState/RestoreState)
5. **Plugin-provided tools** — registered at startup (ADK-Go tools slice, Gollem toolset)
6. **Skills are prompt-based** — not tool execution, content injected into context (OpenCode)
