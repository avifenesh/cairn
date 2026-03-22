# Cairn

Self-hosted, always-on personal agent OS. Single Go binary.

Cairn watches your world (GitHub, Gmail, Calendar, HN, Reddit, npm, crates.io, RSS, Stack Overflow, Dev.to, webhooks), acts on your behalf through an LLM-powered agent with 54+ tools, learns over time through episodic memory and reflection, and stays on 24/7.

## Quick Start

```bash
# Build
make build

# Chat (requires an LLM API key)
export LLM_API_KEY=your-key   # or GLM_API_KEY / OPENAI_API_KEY
./cairn chat "what's in package.json?"

# Serve (HTTP API + SSE + embedded frontend)
cd frontend && pnpm install && pnpm build && cd ..
make build-prod
./cairn serve   # serves on :8787

# Install a skill
./cairn install skill https://github.com/user/my-skill.git
```

## What It Does

**Signal Plane** - Polls 11 sources. Deduplicates into SQLite. Serves via feed API + SSE streaming.

- GitHub notifications + org events (PRs, issues, releases, stars)
- GitHub signal intelligence (engagement metrics, stargazers, followers)
- Gmail + Google Calendar (auto-archive GitHub emails)
- Hacker News (keyword + score filtering)
- Reddit (subreddit monitoring)
- npm + crates.io (package version tracking, download metrics)
- RSS feeds, Stack Overflow, Dev.to
- Webhooks (HMAC-SHA256 signature verification)
- LLM-powered digest generation

**Agent** - Three-layer agent system: always-on loop, LLM orchestrator, ReAct execution.

- **Orchestrator**: LLM-powered management brain that runs when idle. Gathers system state, decides actions (approve memories, spawn subagents, submit tasks, notify, escalate). Runs every 5min.
- **ReAct agents**: 40+ tools (56 with all integrations enabled), three modes (talk/work/coding), streaming sessions
- **Automation rules**: declarative "when X → do Y" engine with expr-lang conditions, event triggers, throttling, execution log
- **Subagents**: 4 types (researcher, coder, reviewer, executor) with tool scoping and isolation. Two-level max nesting.
- File tools: read, write, edit, delete, undo, list, search (checkpointing, fuzzy match, path traversal protection)
- Shell: policy engine, env filtering, shell detection
- Git, web search, web fetch, memory CRUD, feed, tasks, cron, notifications
- Z.ai integration: vision analysis, repo structure, search docs (GLM provider)
- Google Workspace tools (query + execute)
- Skill management: CRUD, install from git, ClawHub marketplace search
- Permission engine with wildcard rules per agent mode
- Session compaction at 150K tokens, steering messages, approval gates
- Always-on with proactive behavior (SOUL.md identity, reflection engine)

**Memory** - Three-tier system: semantic, episodic, procedural.

- Keyword + vector search with MMR re-ranking (LLM provider embedding API)
- Auto-extraction of memories from conversations (contradiction detection)
- Session compaction (SummaryBuffer at 150K tokens)
- Hot-reloadable SOUL.md for behavioral identity
- 17 bundled skills + user-installed via ClawHub marketplace (SKILL.md format, ClawHub-compatible)
- Confidence decay over time

**Channels** - Multi-channel I/O with session continuity.

- Telegram (commands, inline keyboards, voice messages)
- Discord (slash commands, button interactions)
- Slack (slash commands, block kit)
- Notification routing (priority-based, quiet hours, muted sources)

**Voice** - Speech-to-text and text-to-speech.

- Whisper STT (local whisper.cpp)
- edge-tts TTS playback

**Server** - REST API, SSE, WebAuthn, MCP server.

- 50+ REST routes with rate limiting
- SSE broadcaster with reconnection replay
- WebAuthn biometric authentication (passkeys)
- MCP server exposing all tools to Claude Code, Cursor, etc. (stdio + HTTP)
- CORS, static file serving (embedded or filesystem)

**Frontend** - Svelte 5 dashboard (242 tests).

- Today: command center with agent status, approvals, activity stream, quick chat
- Chat: text, voice, file upload, vision, streaming markdown, tool chips, mode selector
- Feed: source filters, archive/delete, bulk actions
- Skills: CRUD, ClawHub marketplace (search, browse, install with security review)
- Memory: search, edit, delete, batch accept/reject
- Settings: 11 sections, all editable (cron manager, notification prefs, agent config)
- Activity: observability tab with live stream, tool stats, error tracking
- Rules: automation rule list, create form, toggle, execution history with real-time SSE
- Soul: SOUL.md editor with patch review flow
- Command palette (Cmd+K), keyboard navigation, dark/light themes, mood packs

**Cron** - Scheduled tasks with SQLite persistence.

- Create, list, toggle, delete cron jobs
- Agent-managed via tools (natural language scheduling)
- Frontend cron manager with inline editing

## Architecture

```
Signal Plane --> Event Bus <-- Agent System --> Tool System
     |               |             |                |
  11 Pollers      SQLite       Always-On Loop   40+ Tools
  Webhooks        Store        Orchestrator     Permissions
  Digest          Memory       ReAct Agents     Mode filtering
                  Sessions     Subagents        MCP adapter
                  Approvals    Compaction       Skills
                  Crons        Rules engine
```

Single binary. No Node, no Python, no Docker. Pure Go + SQLite.

## Build

```bash
make build          # Dev binary (filesystem frontend)
make build-prod     # Production binary (embedded frontend)
make test           # Tests with race detector
make lint           # Formatting + vet
make dev            # Run dev server
```

## Configuration

Set via environment variables. Only `LLM_API_KEY` is required.

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_API_KEY` | - | LLM provider API key (or `GLM_API_KEY` / `OPENAI_API_KEY`) |
| `LLM_PROVIDER` | auto | `glm` or `openai` (auto-detected from key var name) |
| `LLM_MODEL` | provider default | Model ID |
| `PORT` | 8787 | HTTP server port |
| `DATABASE_PATH` | ./data/cairn.db | SQLite database path |
| `SOUL_PATH` | ./SOUL.md | Path to SOUL.md behavioral identity |
| `GH_TOKEN` | - | GitHub token for polling |
| `GH_ORGS` | - | Comma-separated GitHub orgs to track |
| `HN_KEYWORDS` | - | Comma-separated HN keyword filter |
| `REDDIT_SUBS` | - | Comma-separated subreddits |
| `NPM_PACKAGES` | - | npm packages to track |
| `CRATES_PACKAGES` | - | Crates to track |
| `POLL_INTERVAL` | 300 | Source poll interval in seconds |
| `TELEGRAM_BOT_TOKEN` | - | Telegram bot token |
| `DISCORD_BOT_TOKEN` | - | Discord bot token |
| `SLACK_BOT_TOKEN` | - | Slack bot token |
| `IDLE_MODE_ENABLED` | false | Enable always-on proactive agent loop |
| `CODING_ENABLED` | false | Enable coding mode (worktree isolation) |
| `MCP_SERVER_ENABLED` | false | Enable MCP server |
| `RULES_ENABLED` | false | Enable automation rules engine |
| `BUDGET_DAILY_CAP` | 0 | Daily LLM spend cap USD (0 = unlimited) |

See `CLAUDE.md` for full env var reference.

## Stack

- **Go 1.25** - single binary, no CGO
- **SQLite** via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - pure Go, WAL mode
- **SvelteKit 5** frontend - Svelte 5 runes, Tailwind v4, shadcn-svelte, embedded via `embed.FS`
- **LLM providers** - GLM (Z.ai) and OpenAI-compatible APIs
- **MCP** via [mcp-go](https://github.com/mark3labs/mcp-go) - tool exposure to external agents
- **Telegram** via [telego](https://github.com/mymmrac/telego)

## Project Structure

```
cmd/cairn/          CLI entry point (chat, serve, install skill, version)
internal/
  agent/            Always-on loop, orchestrator, ReAct agents, subagents, compaction, sessions
  auth/             WebAuthn biometric authentication
  channel/          Telegram, Discord, Slack adapters, notification routing
  config/           Env-based configuration with live patching
  cron/             Cron scheduler + SQLite store
  db/               SQLite + embedded migrations
  eventbus/         Typed pub/sub (Go generics)
  llm/              Provider interface, GLM + OpenAI, SSE parser, retry, budget
  mcp/              MCP server (stdio + HTTP transport)
  memory/           Semantic store, RAG search, Soul, embeddings, compaction, extraction
  plugin/           Lifecycle hooks, logging, budget plugins
  server/           HTTP routes, SSE, auth, rate limiting, static files
  signal/           Event store, scheduler, 11 pollers, webhooks, digest
  rules/            Automation rules engine (trigger-condition-action, expr-lang)
  skill/            SKILL.md parser, discovery, hot-reload, ClawHub marketplace
  task/             Priority queue, worktree isolation, lease engine, approvals
  tool/             Tool interface, registry, permissions, 40+ built-in tools (config-dependent)
  voice/            Whisper STT + edge-tts TTS
frontend/           SvelteKit 5 app + embed.FS package (242 tests)
skills/             17 bundled SKILL.md files
docs/design/        Architecture specs and phase plans
```

## Roadmap

Planned features, roughly in priority order:

1. **Automation rules engine** - declarative "when X happens, do Y" rules (e.g., "on new PR, run code review")
2. **Webhook-triggered workflows** - skills auto-invoked from webhook events
3. **Agent activity analytics** - historical tool call frequency, error rates, cost graphs
4. **Memory RAG improvements** - conversation-aware retrieval, time-decay search scoring
5. **Multi-file atomic edits** - single tool call for cross-file refactors
6. **PWA mobile experience** - push notifications, offline support, swipe gestures
7. **Session cleanup lifecycle** - automatic memory reclamation for long-running servers

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute.

## License

MIT
