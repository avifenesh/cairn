# Cairn — Vision & Architecture

Cairn is an open-source, self-hosted, always-on **personal agent OS** written in Go. It ships as a single binary that watches your world (GitHub, email, calendar, feeds, webhooks), acts on your behalf (writes code, triages email, manages tasks), learns over time (semantic memory, episodic recall, behavioral identity), and stays on 24/7 - reaching out proactively when something matters. Models propose, humans dispose. No irreversible side effects without explicit approval.

## Design Principles

### Models propose, humans dispose
Every side effect with real-world consequence passes through an approval gate. The agent can draft, plan, and recommend - but destructive actions (file writes, email sends, deployments, memory mutations) require human sign-off unless the permission engine explicitly allows them. Approvals flow through any connected channel: web UI, Telegram, Discord, Slack.

### Single binary
One `go build` produces one binary. SQLite is embedded (pure Go, no CGO). The SvelteKit frontend compiles to static files and embeds via `embed.FS`. Deploy means `scp cairn server:/usr/local/bin/`. No Node, no Python, no Docker required.

### Event-sourced sessions
Every interaction is an append-only event stream stored in SQLite. Sessions can be compacted (summarize old turns to reclaim context), branched, and replayed. No mutable state means no state corruption across restarts.

### Three-tier memory
- **Semantic** - facts, preferences, project knowledge. RAG-retrieved with MMR re-ranking for diversity. Auto-extracted from conversations, deduplicated by cosine similarity.
- **Episodic** - what happened and when. Decays over time (configurable half-life). Provides temporal context for decisions.
- **Procedural (Soul)** - behavioral identity, communication style, hard rules. Compact, human-editable markdown. Patchable by the agent with owner approval.

### Skill ecosystem
Skills follow the OpenClaw SKILL.md format - plain markdown files describing capabilities, triggers, and prompts. ClawHub marketplace skills install and work without modification. Cairn adds typed tool integration, sandboxed execution, and skill-level permissions on top of the shared format.

### MCP-native
Cairn exposes its 52+ built-in tools as an MCP server and consumes external MCP servers as tool sources. This makes it a first-class participant in the emerging agent interop ecosystem - not a walled garden.

## Architecture

```
+-----------------------------------------------------------+
|                      Cairn Binary                         |
|                                                           |
|  +----------+  +----------+  +----------+  +----------+  |
|  |  Signal  |  |  Agent   |  |  Action  |  |  Memory  |  |
|  |  Plane   |  |  Core    |  |  Plane   |  |  System  |  |
|  |          |  |          |  |          |  |          |  |
|  | Pollers  |  | LLM Loop |  | Tasks    |  | RAG      |  |
|  | Webhooks |  | Tools    |  | Worktrees|  | Journal  |  |
|  | Push     |  | Skills   |  | Approvals|  | Soul     |  |
|  | SSE      |  | Modes    |  | Artifacts|  | Embed    |  |
|  +----+-----+  +----+-----+  +----+-----+  +----+-----+  |
|       |             |             |              |         |
|  +----+-------------+-------------+--------------+------+ |
|  |              Event Bus (typed, async)                 | |
|  +----+-------------+-------------+--------------+------+ |
|       |             |             |              |         |
|  +----+-----+  +----+-----+  +----+-----+  +----+-----+  |
|  |  HTTP    |  |  Plugin  |  |  Protocol|  |  SQLite  |  |
|  |  Server  |  |  System  |  |  Layer   |  |  Store   |  |
|  |          |  |          |  |          |  |          |  |
|  | REST API |  | Hooks    |  | MCP srv  |  | Events   |  |
|  | SSE push |  | Skills   |  | MCP cli  |  | Tasks    |  |
|  | WebAuthn |  | Ext Tools|  | Channels |  | Memory   |  |
|  +----------+  +----------+  +----------+  +----------+  |
|                                                           |
|  +-----------------------------------------------------+ |
|  |              Permission Engine                       | |
|  |  Wildcard rules - Agent modes - Approval gates       | |
|  +-----------------------------------------------------+ |
+-----------------------------------------------------------+
        |                                |
   +----+                               +----+
   v                                         v
 Web UI (SvelteKit 5, embed.FS)          External Agents
 Static files served by Go              via MCP / Channels
```

**Signal Plane** polls 11 sources (default 5min, configurable per source). Events are deduplicated, normalized, and published to the event bus. The **Agent System** operates in three layers: an always-on loop (60s tick) checks for due crons and pending tasks; when idle, an LLM-powered **Orchestrator** gathers system state and decides what to do proactively (approve memories, spawn subagents, submit tasks, notify, escalate). Actual work is executed by **ReAct agents** - a main agent plus 4 subagent types (researcher, coder, reviewer, executor) with tool scoping and two-level max nesting. The **Memory System** provides context injection (RAG), session compaction at 150K tokens, and persists learned knowledge. Everything converges through a typed async event bus backed by SQLite.

## Why Go

- **Single binary** - `scp cairn /usr/local/bin/ && systemctl start cairn`. No npm, no node_modules, no Python venvs
- **True concurrency** - goroutines for parallel LLM streams, tool execution, polling, SSE. No single-threaded event loop bottleneck
- **Predictable memory** - no GC pauses during critical streaming paths. No heap growth over 24/7 uptime
- **Fast compilation** - iterate quickly, deploy in seconds
- **SQLite-native** - pure Go SQLite (modernc.org/sqlite). No `npm rebuild better-sqlite3`
- **Static typing + generics** - type-safe tools, agents, events without runtime reflection hacks
- **Ecosystem** - mcp-go, go-openai, and the broader Go networking stack are production-quality

## Differentiators

1. **Worktree isolation** - every coding task gets its own git worktree. No branch stomping, no stale refs, no agent conflicts. Multiple coding sessions run in parallel without interference.

2. **Permission engine** - wildcard rules scoped per agent mode, per tool, per file pattern, and per approval policy. The owner configures once; the system enforces everywhere. Not binary allow/deny - graduated control with pattern matching.

3. **Always-on with orchestrator brain** - an LLM-powered orchestrator runs every 5 minutes when idle, gathering system state (feeds, errors, memories, subagents) and deciding what to do proactively: approve memories, spawn subagents, submit tasks, notify the user, or escalate to human review. The agent has a Soul (behavioral identity), episodic memory (what happened), semantic memory (what it knows), and procedural memory (rules it has learned).

4. **Skill ecosystem compatibility** - skills follow the OpenClaw SKILL.md format, and ClawHub marketplace skills install without modification. Cairn adds typed tool integration, sandboxed execution, and skill-level permissions on top of the shared ecosystem.

5. **MCP-native tool interop** - Cairn exposes its tools as an MCP server and consumes external MCP servers as tool sources. Bidirectional, first-class, not an afterthought.

6. **Event-sourced sessions** - every interaction is an append-only event stream. Sessions are compactable (summarize old turns when context fills), replayable (debug what happened), and durable across restarts.

7. **Industry-grade file edit safety** - read-before-write enforcement, ambiguous match detection, fuzzy matching with context, automatic checkpointing (undo via `cairn.undoEdit`), offset/line-range support, line count validation, and structured diagnostics.

8. **Single binary deployment** - one binary, runs on Linux/macOS/WSL. SQLite embedded. Frontend embedded. No containers, no runtimes, no package managers.

## Roadmap

The core system is complete (9 phases, 175+ PRs, 839 tests). The roadmap focuses on depth and autonomy:

- **Automation rules engine** - declarative "when X happens, do Y" rules. Event-pattern matching triggers agent actions without manual intervention.
- **Webhook-triggered workflows** - external services push events that trigger multi-step agent workflows (CI failure -> diagnose -> open PR).
- **Agent activity analytics dashboard** - visualize tool usage, LLM costs, task throughput, error rates, and session patterns over time.
- **Memory RAG improvements** - conversation-aware retrieval (weight recent context), time-decay scoring, cross-session knowledge graphs.
- **Multi-file atomic edits** - transactional file operations that commit or roll back as a unit. Essential for large refactors.
- **PWA mobile experience** - progressive web app shell for mobile access to the agent. Push notifications, offline queue, responsive chat.
- **Session lifecycle cleanup** - automatic archival of stale sessions, configurable retention policies, storage reclamation.
- **Skill authoring from chat** - describe what you want in natural language; the agent generates, tests, and installs the skill.

## Challenges

### LLM provider diversity
GLM is primary, but the system supports any OpenAI-compatible provider. Each has different streaming formats, tool calling conventions, context limits, and quirks.
**Current solution:** Provider adapter interface with per-provider message normalization. Fallback chains (glm-5-turbo -> glm-5 -> glm-4.7). Budget tracking per provider.

### Long-running tasks vs short requests
A coding task might run for 30 minutes across hundreds of tool rounds. A quick question takes 2 seconds.
**Current solution:** Task engine with priority queue. Long tasks run in background goroutines with context cancellation. Short requests get fast-path routing. Orchestrator manages concurrency.

### Memory growth over months
A personal agent accumulates thousands of memories, sessions, and events. SQLite handles the storage, but context injection becomes the bottleneck.
**Current solution:** Three-tier memory with compaction. RAG with MMR re-ranking for diversity. Episodic memory decays (configurable half-life). Semantic deduplication (cosine similarity >= 0.92). Session compaction triggers at 150K tokens.

### Concurrent git operations
Multiple coding tasks writing to the same repo, even with worktrees, can conflict on shared refs.
**Current solution:** Git operation queue with per-repo mutex. Worktrees are independent but merges (rebase) are serialized.

### Plugin security
Third-party skills and plugins can be malicious.
**Current solution:** LLM-powered security review on marketplace install, content scanning for secrets and prompt injection, sandboxed execution, and approval-gated side effects.
