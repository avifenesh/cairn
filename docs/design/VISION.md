# Cairn — Vision & Architecture

> An open-source, self-hosted, always-on personal agent OS written in Go.
> Models propose, humans dispose. No irreversible side effects without explicit approval.

## The End Goal

Cairn is a **personal agent operating system** — not a chatbot, not a coding assistant, not a notification hub. It's all three, unified under a single runtime that:

1. **Watches your world** — aggregates signals from every source that matters (GitHub, email, calendar, feeds, webhooks, agent channels) and filters noise from signal
2. **Acts on your behalf** — writes code, triages email, plans trips, creates documents, manages tasks — with appropriate autonomy boundaries
3. **Learns and improves** — accumulates knowledge about you, your projects, your preferences, and uses that knowledge to get better over time
4. **Stays on** — runs 24/7 on your machine, proactively working when you're away, surfacing what matters when you're back
5. **Talks to other agents** — speaks MCP protocol so it can delegate to and receive work from external agents

The differentiator from everything that exists today:

| Existing | Cairn |
|----------|--------|
| Coding agents (Claude Code, Cursor, OpenCode) | Coding is ONE capability, not the whole product |
| Notification hubs (Novu, ntfy) | Notifications are signals that feed decision-making, not endpoints |
| OpenClaw (318k⭐, 22 channels, 13.7k skills) | Go single-binary vs TS monorepo. Simpler. Faster. Self-contained. Same skill format. |
| Agent frameworks (ADK, Eino, LangChain) | Not a framework — a complete system you deploy and live with |

### How Cairn Relates to OpenClaw

OpenClaw is the current gold standard. We study it, respect it, and differentiate:

| OpenClaw | Cairn |
|----------|--------|
| TypeScript monorepo, Node.js runtime | Go single binary, zero dependencies |
| 75 extensions, complex plugin system | Lean core, same SKILL.md format, ClawHub-compatible |
| 22+ messaging channels (WhatsApp, Telegram, Slack...) | Web + Telegram first, channel adapters as plugins |
| ClawHub marketplace (13.7k skills, vector search) | Compatible consumer of ClawHub + own registry later |
| Gateway daemon (WebSocket control plane) | Single process (Go goroutines, no WS control plane needed) |
| nanobot exists because OpenClaw is "too complex" | Simplicity is the design goal — the nanobot of Go |
| Session transcripts as JSONL files | Event-sourced SQLite (queryable, branchable, compactable) |
| No native worktree isolation for coding | Git worktree per coding task (first-class) |
| Security audit tool built-in | Permission engine with wildcard rules + approval gates |

**The thesis:** OpenClaw proved the "always-on personal agent" category works. Cairn takes the same vision but builds it in Go for: performance, simplicity, single-binary deployment, and proper coding task isolation. Skills are compatible. The ecosystem is shared.

## Why Go

- **Single binary** — `scp cairn /usr/local/bin/ && systemctl start pub`. No npm, no node_modules, no Python venvs
- **True concurrency** — goroutines for parallel LLM streams, tool execution, polling, SSE. No single-threaded event loop bottleneck
- **Predictable memory** — no GC pauses during critical streaming paths. No heap growth over 24/7 uptime
- **Fast compilation** — iterate quickly, deploy in seconds
- **SQLite-native** — pure Go SQLite (modernc.org/sqlite) or CGO (mattn). No `npm rebuild better-sqlite3`
- **Static typing + generics** — type-safe tools, agents, events without runtime reflection hacks
- **Ecosystem** — mcp-go, ADK-Go, go-openai are all production-quality

## Why Open Source

The v1 prototype proved the concept. v2 should be open because:
- The "personal agent OS" category doesn't have a definitive open-source solution
- OpenClaw is the closest but it's TypeScript, focused on coding, not truly "always-on personal"
- Open source enables a skill ecosystem (like OpenClaw's ClawHub with 13,700+ skills)
- Contributors can add providers, tools, integrations that one person never would
- Trust — users need to see what runs on their machine 24/7

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Cairn Binary                         │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐ │
│  │  Signal   │  │  Agent   │  │  Action  │  │ Memory │ │
│  │  Plane    │  │  Core    │  │  Plane   │  │ System │ │
│  │          │  │          │  │          │  │        │ │
│  │ Pollers  │  │ LLM Loop │  │ Tasks    │  │ RAG    │ │
│  │ Webhooks │  │ Tools    │  │ Worktrees│  │ Journal│ │
│  │ Push     │  │ Skills   │  │ Approvals│  │ Soul   │ │
│  │ SSE      │  │ Modes    │  │ Artifacts│  │        │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └───┬────┘ │
│       │             │             │             │       │
│  ┌────┴─────────────┴─────────────┴─────────────┴────┐ │
│  │              Event Bus (typed, async)              │ │
│  └────┬─────────────┬─────────────┬─────────────┬────┘ │
│       │             │             │             │       │
│  ┌────┴────┐  ┌─────┴────┐  ┌────┴────┐  ┌────┴────┐ │
│  │  HTTP   │  │  Plugin  │  │  Proto   │  │ SQLite  │ │
│  │  Server │  │  System  │  │  Layer   │  │  Store  │ │
│  │         │  │          │  │          │  │         │ │
│  │ REST    │  │ Hooks    │  │ MCP      │  │ Events  │ │
│  │ SSE     │  │ Skills   │  │ server   │  │ Tasks   │ │
│  │ WebAuth │  │ Ext Tools│  │ client   │  │ Memory  │ │
│  └─────────┘  └──────────┘  └──────────┘  └─────────┘ │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │           Permission Engine                      │   │
│  │  Wildcard rules · Agent modes · Approval gates   │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
         │                              │
    ┌────┘                              └────┐
    ▼                                        ▼
  Web UI (SvelteKit 5, embed.FS)         External Agents
  Static files served by Go              via MCP
```

## The Modules

Each piece is a self-contained module with clear interfaces. See individual design docs for details.

| # | Piece | Description | Design Doc |
|---|-------|-------------|------------|
| 1 | **Event Bus** | Typed async pub/sub backbone | [01-event-bus.md](pieces/01-event-bus.md) |
| 2 | **LLM Client** | Multi-provider streaming with retry/fallback/budget | [02-llm-client.md](pieces/02-llm-client.md) |
| 3 | **Tool System** | Type-safe tools, registry, mode filtering, permissions | [03-tool-system.md](pieces/03-tool-system.md) |
| 4 | **Agent Core** | Agent interface, ReAct loop, session, state machine | [04-agent-core.md](pieces/04-agent-core.md) |
| 5 | **Task Engine** | Task lifecycle, queue, worktree isolation, leases | [05-task-engine.md](pieces/05-task-engine.md) |
| 6 | **Memory System** | Semantic + episodic + procedural memory, RAG | [06-memory-system.md](pieces/06-memory-system.md) |
| 7 | **Signal Plane** | Source polling, webhooks, event ingestion, dedup | [07-signal-plane.md](pieces/07-signal-plane.md) |
| 8 | **Plugin & Skill System** | Lifecycle hooks, skill discovery, ClawHub-compatible | [08-plugin-skills.md](pieces/08-plugin-skills.md) |
| 9 | **Server & Protocols** | HTTP, SSE, MCP server + client, auth, permissions | [09-server-protocols.md](pieces/09-server-protocols.md) |
| 10 | **Frontend** | Svelte 5 dashboard, embedded in Go binary | [10-frontend.md](pieces/10-frontend.md) |
| 11 | **Channel Adapters** | Multi-channel I/O: web, Telegram, Slack, CLI, API, voice | [11-channel-adapters.md](pieces/11-channel-adapters.md) |

## What Takes It One Level Above

### 1. True Worktree Isolation (vs everyone else sharing one working tree)
Every coding task gets its own git worktree. No branch stomping, no stale refs, no agent conflicts. Merge back via rebase when done. This is what Uzi does and nobody else (including Claude Code, OpenCode, Cursor) has solved properly.

### 2. Permission Engine with Wildcard Rules (vs binary allow/deny)
OpenCode's 3-tier permission system (allow/ask/deny with wildcard patterns) is the best in the industry. We take it further: permissions are scoped per agent mode, per tool, per file pattern, AND per approval policy. The owner configures once; the system enforces everywhere.

### 3. Always-On with Proactive Behavior (vs reactive chatbots)
OpenClaw calls this "always-on personal agent." We go further: the agent has a Soul (behavioral identity), episodic memory (what happened), semantic memory (what it knows), and procedural memory (rules it's learned). It doesn't wait to be spoken to — it watches, learns, acts, and reaches out.

### 4. Skill Ecosystem Compatibility (vs walled gardens)
Skills follow the OpenClaw SKILL.md format. Existing ClawHub skills work. We add: typed tool integration (not just prompt injection), sandboxed execution, and skill-level permissions. The skill marketplace isn't ours alone — it's the shared ecosystem.

### 5. MCP-Native Tool Interop (vs HTTP-only)
MCP for tool discovery and execution — first-class, not an afterthought. Cairn exposes its tools as an MCP server and consumes external MCP servers as tools.

### 6. Event-Sourced Sessions (vs mutable state)
Every interaction is an append-only event stream. Sessions can be branched ("what if I tried this instead?"), compacted (summarize old turns), and replayed (debug what happened). No mutable state means no state corruption.

### 7. Industry-Grade File Edit Safety (vs naive overwrite)
File editing tools include: read-before-write enforcement, ambiguous match detection, fuzzy matching with context, automatic checkpointing (undo via `cairn.undoEdit`), offset/line-range support, line count validation, and structured diagnostics. File change events are surfaced at the agent layer for full observability, not buried in tool internals.

### 8. Single Binary Deployment (vs npm install hell)
`curl -L https://github.com/avifenesh/cairn/releases | sh` → one binary, runs on Linux/macOS/WSL. No Node, no Python, no Docker required. SQLite is embedded. Whisper.cpp sidecar optional.

## Edge Cases & Challenges

### Challenge: LLM Provider Diversity
GLM-5 Turbo is primary, but the system must support Anthropic, OpenAI, Google, and local models. Each has different streaming formats, tool calling conventions, and quirks.
**Solution:** Provider adapter interface with per-provider message normalization (borrowing OpenCode's transform layer).

### Challenge: Long-Running Tasks vs Short Requests
A coding task might run for 30 minutes (100 tool rounds). A quick question takes 2 seconds.
**Solution:** Task engine with priority queue. Long tasks run in background goroutines with context cancellation. Short requests get dedicated goroutines with fast-path routing.

### Challenge: Memory Growth Over Months
A personal agent accumulates thousands of memories, sessions, events. SQLite can handle it, but context injection becomes the bottleneck.
**Solution:** Three-tier memory with compaction. RAG with MMR re-ranking for diversity. Episodic memory decays (half-life). Procedural memory (Soul) stays compact.

### Challenge: Concurrent Git Operations
Multiple coding tasks writing to the same repo, even with worktrees, can conflict on shared refs.
**Solution:** Git operation queue with per-repo mutex. Worktrees are independent but merges (rebase) are serialized.

### Challenge: Plugin Security
Third-party skills and plugins can be malicious.
**Solution:** Skill vetting (content scanning for secrets, prompt injection), sandboxed execution (no network by default), and approval-gated side effects.

### Challenge: Frontend Without a Framework
The current app.js monolith has stale refs, no type safety, and O(n²) complexity.
**Solution:** Phase 4 splits into ES modules with a minimal reactive layer. The Go backend serves static files — the frontend is decoupled and can evolve independently.

## Success Criteria

1. **The binary runs for 30 days** without memory leaks, crashes, or stuck tasks
2. **Coding tasks complete in isolation** — no branch conflicts, no stale working trees
3. **Sub-second response** for chat messages (streaming first token)
4. **Skills from ClawHub install and work** without modification
5. **The agent proactively surfaces** useful information daily
6. **Permission system prevents** any irreversible action without approval
7. **A contributor can add a new LLM provider** in <100 lines
8. **A contributor can create a new tool** in <50 lines
9. **The whole system builds** in <30 seconds on a modern machine
