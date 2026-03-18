# Cairn

Self-hosted, always-on personal agent OS. Single Go binary.

Cairn watches your world (GitHub, HN, Reddit, npm, crates.io, webhooks), acts on your behalf through an LLM-powered agent with tools, learns over time through episodic memory and reflection, and stays on 24/7.

## Quick Start

```bash
# Build
make build

# Chat (requires LLM_API_KEY)
export GLM_API_KEY=your-key
./cairn chat "what's in package.json?"

# Serve (HTTP API + SSE + frontend)
./cairn serve
```

## What It Does

**Signal Plane** - Polls 5+ sources every 5 minutes. Deduplicates into SQLite. Serves via feed API + SSE streaming.

- GitHub notifications + org events (PRs, issues, releases, pushes, stars)
- Hacker News (keyword + score filtering, concurrent fetches)
- Reddit (subreddit monitoring)
- npm + crates.io (package version tracking)
- Webhooks (HMAC-SHA256 signature verification)
- Digest generation (LLM-powered event summarization)

**Agent** - ReAct loop with tool execution, three modes (talk/work/coding), session persistence.

- 8 built-in tools: readFile, writeFile, editFile, deleteFile, listFiles, searchFiles, shell, gitRun
- Permission engine with wildcard rules per mode
- Session journaling (episodic memory via LLM summarization)
- Reflection engine (pattern detection across sessions, proposes memories + SOUL patches)
- Always-on tick loop with task execution

**Memory** - Three-tier system: semantic (facts via RAG), episodic (session journal), procedural (SOUL.md + skills).

- Keyword + vector search with MMR re-ranking
- Hot-reloadable SOUL.md for behavioral rules
- SKILL.md parser with discovery and prompt injection
- Memory compaction and confidence decay

**Server** - 25+ REST routes, SSE broadcaster, WebAuthn-ready auth, static file serving.

- Rate limiting (sliding window per IP)
- CORS with credentials support
- Webhook endpoint with signature verification
- Static files from embedded FS (production) or filesystem (dev)

## Architecture

```
Signal Plane --> Event Bus <-- Agent Core --> Tool System
     |               |             |              |
  Pollers         SQLite       LLM Client    Permissions
  Webhooks        Store        Sessions      Mode filtering
  Digest          Memory       ReAct loop    Built-in tools
                  Journal      Reflection
```

Single binary. No Node, no Python, no Docker. Pure Go + SQLite.

## Build

```bash
# Development (reads frontend/dist/ from filesystem)
make build

# Production (embeds frontend into binary)
make build-prod

# Run tests
make test

# Lint (formatting + vet)
make lint

# Release (via goreleaser, triggered by git tag)
git tag v0.1.0
git push --tags
```

## Configuration

Set via environment variables. Only `LLM_API_KEY` is required.

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_API_KEY` | - | LLM provider API key (or `GLM_API_KEY` / `OPENAI_API_KEY`) |
| `LLM_PROVIDER` | auto | `glm` or `openai` (auto-detected from key variable) |
| `LLM_MODEL` | provider default | Model ID |
| `PORT` | 8787 | HTTP server port |
| `DATABASE_PATH` | ./data/cairn.db | SQLite database path |
| `SOUL_PATH` | ./SOUL.md | Path to SOUL.md behavioral rules |
| `GH_TOKEN` | - | GitHub token for polling |
| `GH_ORGS` | - | Comma-separated GitHub orgs to track |
| `HN_KEYWORDS` | - | Comma-separated HN keyword filter |
| `HN_MIN_SCORE` | 0 | Minimum HN story score |
| `REDDIT_SUBS` | - | Comma-separated subreddits |
| `NPM_PACKAGES` | - | Comma-separated npm packages to track |
| `CRATES_PACKAGES` | - | Comma-separated crates to track |
| `WEBHOOK_SECRETS` | - | JSON map of webhook name to HMAC secret |
| `POLL_INTERVAL` | 300 | Source poll interval in seconds |

## Stack

- **Go 1.25** - single binary, no CGO
- **SQLite** via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - pure Go, WAL mode
- **SvelteKit 5** frontend - Svelte 5 runes, Tailwind v4, embedded via `embed.FS`
- **LLM providers** - GLM (Z.ai) and OpenAI-compatible APIs

## Project Structure

```
cmd/cairn/          CLI entry point (chat, serve, version)
internal/
  agent/            ReAct loop, sessions, journaler, reflection, always-on loop
  config/           Env-based configuration
  db/               SQLite + migrations
  eventbus/         Typed pub/sub (Go generics)
  llm/              Provider interface, GLM + OpenAI, SSE parser, retry, budget
  memory/           Semantic store, RAG search, Soul loader, compaction
  server/           HTTP routes, SSE, auth, rate limiting, static files
  signal/           Event store, scheduler, pollers, webhooks, digest
  skill/            SKILL.md parser, discovery, hot-reload
  task/             Priority queue, worktree isolation, lease engine
  tool/             Tool interface, registry, permissions, built-in tools
frontend/           SvelteKit 5 app + embed.FS package
```

## License

MIT
