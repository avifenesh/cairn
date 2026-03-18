# Cairn

> Open-source, self-hosted, always-on personal agent OS. Written in Go.
> Models propose, humans dispose. No irreversible side effects without explicit approval.

Not a chatbot. Not a coding assistant. Not a notification hub. All three, unified under a single binary that watches your world, acts on your behalf, learns over time, and stays on 24/7.

## Stack

Go 1.25 single binary + SQLite (modernc, pure Go, no CGO) + SvelteKit 5 frontend (Svelte 5 runes, Tailwind v4, static adapter embedded via `embed.FS`).

## Modules (update status before starting any piece)

| # | Piece | Status | Package |
|---|-------|--------|---------|
| 1 | Event Bus - typed async pub/sub backbone | Done | `internal/eventbus/` |
| 2 | LLM Client - multi-provider streaming, retry/fallback/budget | Done | `internal/llm/` |
| 3 | Tool System - type-safe tools, registry, mode filtering, permissions | Done | `internal/tool/` |
| 4 | Agent Core - ReAct loop, sessions, modes (talk/work/coding) | Done | `internal/agent/` |
| 5 | Task Engine - priority queue, worktree isolation, leases | Done | `internal/task/` |
| 6 | Memory System - semantic + episodic + procedural, RAG, Soul | Done | `internal/memory/` |
| 7 | Signal Plane - source polling, webhooks, event ingestion, dedup | Done | `internal/signal/` |
| 8 | Skill System - SKILL.md parser, discovery, hot-reload, injection | Done | `internal/skill/` |
| 9 | Server & Protocols - HTTP, SSE, REST API, auth, static files | Done | `internal/server/` |
| 10 | Frontend - Svelte 5 dashboard, embedded in Go binary | Done (10.1-10.11 + Phase 6, 169 tests) | `frontend/` |
| 11 | Channel Adapters - web, Telegram, Slack, CLI, API, voice | Not started | — |

Frontend complete (10.1-10.11 + Phase 6 hardening). 169 tests across 19 files. Next: 10.12 (Go embed) + Phase 5.2 (integration testing against Go server).

## Phases

```
Phase 1: Foundation (event bus + LLM + SQLite)                [DONE]
Phase 2: Core Systems (tools | tasks | memory) in parallel    [DONE]
Phase 3: Agent Core (ReAct loop wires all together)           [DONE]
Phase 4: Server + Skills + Signal Plane (4a+4b+4c)             [DONE]
Phase 5: Integration, always-on loop, open-source release
```

## Architecture

```
Signal Plane → Event Bus ← Agent Core → Tool System
     ↕              ↕            ↕           ↕
  Pollers        SQLite      LLM Client   Permissions
  Webhooks       Store       Sessions     Mode filtering
  SSE push       Memory      ReAct loop   MCP adapter
```

Key design decisions:
- Event bus uses Go generics: `Subscribe[E](bus, handler)`, `Publish[E](bus, event)`
- LLM providers implement `Provider` interface: `ID()`, `Stream()`, `Models()`
- Streaming returns `<-chan Event` with variants: TextDelta, ReasoningDelta, ToolCallDelta, MessageEnd, StreamError
- Migrations embedded via `//go:embed`, applied in filename order, tracked in schema_migrations
- SQLite: WAL mode, single writer, foreign keys ON, MMAP 256MB
- Frontend uses Svelte 5 runes (`.svelte.ts` stores), `tailwind-variants` for component styling

## Planned Differentiators (not yet implemented unless marked Done above)

1. **Worktree isolation** - every coding task gets its own git worktree.
2. **Permission engine** - wildcard rules scoped per agent mode, per tool, per file pattern.
3. **Always-on with proactive behavior** - Soul, episodic + semantic + procedural memory.
4. **Skill ecosystem compatibility** - OpenClaw SKILL.md format.
5. **Multi-protocol** - A2A, MCP, ACP first-class.
6. **Event-sourced sessions** - append-only, branchable, compactable, replayable.
7. **Single binary** - `scp cairn server:/usr/local/bin/`. No Node, no Python, no Docker.

## Current Structure

```
cmd/cairn/main.go             CLI entry point (cairn chat | cairn serve)
internal/
  config/config.go            Env-based config, provider auto-detection (GLM/OpenAI)
  db/                         SQLite open + WAL pragmas, embedded migrations
  eventbus/                   Typed pub/sub (generics), sync + async + stream delivery
  llm/                        Provider interface, GLM + OpenAI providers, SSE parser, retry, budget
  tool/                       Tool interface, Define[P] generics, registry, permission engine
  tool/builtin/               Built-in tools: readFile, writeFile, editFile, shell, gitRun, etc.
  task/                       Task store, priority queue, worktree manager, lease claiming, reaper
  memory/                     Memory store, RAG search + MMR, embedder interface, Soul loader
  agent/                      ReAct loop, session store, modes (talk/work/coding), system prompt builder
  server/                     HTTP server, REST routes, SSE broadcaster, auth, static files
  skill/                      SKILL.md parser, discovery, hot-reload, prompt injection
  signal/                     Signal plane: event store, scheduler, GitHub + HN pollers
frontend/                     SvelteKit 5 app (Svelte 5 runes, Tailwind v4, shadcn-svelte)
  src/routes/                 today, chat, ops, memory, agents, skills, soul, settings
  src/lib/stores/             Reactive stores (app, chat, feed, memory, tasks, sse, offline-queue, keyboard-nav)
  src/lib/components/         chat/, feed/, layout/, memory/, tasks/, shared/
  src/lib/api/client.ts       Typed REST client (mock fallback via pub_use_mocks localStorage)
  src/lib/utils/              markdown (marked+DOMPurify), time (relative), tts (playback)
docs/design/                  Architecture specs (VISION, PHASES, pieces/01-11)
```

## Commands

```bash
# Backend (from repo root)
go vet ./...                    # Lint - run before every commit
go test -race ./...             # Tests with race detector
go build -o cairn ./cmd/cairn   # Build binary
./cairn chat "hello"            # CLI chat (ReAct agent)
./cairn serve                   # HTTP server on :8787

# Frontend (from frontend/)
pnpm dev                        # Dev server
pnpm build                      # Static build to dist/
pnpm check                      # Svelte + TypeScript check
pnpm test                       # Vitest

# Full validation (no CI pipeline yet - run manually before merge)
go vet ./... && go test -race ./... && cd frontend && pnpm check && pnpm test
```

Tests: `*_test.go` alongside source (Go), `*.test.ts` alongside stores (frontend).

## Env Vars

**Required (one of):**
- `LLM_API_KEY` / `GLM_API_KEY` / `OPENAI_API_KEY`

**LLM config:**
- `LLM_PROVIDER` - "glm" or "openai" (auto-detected from key var name)
- `LLM_MODEL` - model ID (default: glm-5-turbo or gpt-4o depending on provider)
- `LLM_BASE_URL` - API endpoint (default: provider-specific)
- `LLM_FALLBACK_MODEL` - fallback on persistent failure

**Server:**
- `PORT` (8787), `HOST` (0.0.0.0)
- `DATABASE_PATH` (./data/cairn.db)
- `WRITE_API_TOKEN`, `READ_API_TOKEN` - API auth tokens
- `FRONTEND_ORIGIN` - CORS origin

**Signal plane:**
- `GH_TOKEN` / `GITHUB_TOKEN` - GitHub API token for polling
- `GH_ORGS` - comma-separated org names to track
- `HN_KEYWORDS` - comma-separated HN keyword filter
- `HN_MIN_SCORE` (0) - minimum HN story score
- `POLL_INTERVAL` (300) - poll interval in seconds
- `REDDIT_SUBS` - comma-separated subreddit names
- `NPM_PACKAGES` - comma-separated npm packages to track
- `CRATES_PACKAGES` - comma-separated crates to track
- `WEBHOOK_SECRETS` - JSON map of name->secret (e.g. '{"github":"abc"}')

**Feature flags:**
- `CODING_ENABLED` (false), `IDLE_MODE_ENABLED` (false)

**Paths:**
- `SOUL_PATH` (./SOUL.md), `SKILL_DIRS` (./.pub/skills), `DATA_DIR` (./data)

## Design Docs

Full design specs live in `docs/design/`:
- `VISION.md` - architecture, differentiators, success criteria
- `PHASES.md` - implementation phases with dependency graph
- `FRONTEND_AGENT_BRIEF.md` - frontend spec, API contract, SSE events, views
- `pieces/01-event-bus.md` through `pieces/11-channel-adapters.md` - per-piece design

<critical-rules>

## Rules (non-negotiable - violations are bugs)

### Communication

- No emojis. Plain text markers only: [OK], [ERROR], [WARN], [CRITICAL].
- Concise, direct. Say what is needed, nothing more.
- Save tokens. No verbose summaries, no fluff.
- Never create summary files, plan files, audit files, or temp docs.
- In prose, use a single dash (-) not double dash (--) for separators and asides.
- If unsure, ask. Never assume.
- Tell me when I am wrong. Do not sugarcoat.
- Never ignore my instructions. If I instruct, nothing is more valuable than that.

### Plan Adherence

- The design docs in `docs/design/` are the source of truth. Every implementation must match them.
- Before committing any piece, audit it against the corresponding `pieces/*.md` spec. Check every task checkbox - if the spec says implement it, implement it. If you deliberately skip something, document why.
- After any agent (subagent, background task, or autonomous work) completes, verify its output against the plan before accepting. Agents drift. Catch it before it merges.
- The phase dependency graph in `PHASES.md` is strict. Don't start Phase N+1 work until Phase N deliverables are verified.
- If you find a gap between implementation and spec, fix it immediately. Don't defer unless there's a real dependency blocker.
- Frontend must follow `FRONTEND_AGENT_BRIEF.md`. Backend must follow the piece docs. No freelancing.

### Code Quality

- Correctness above all. Verify with tests, not assumptions.
- No stubs or omissions. Complete implementations only - no TODOs, no `// ... rest remains`.
- Before every commit, review your own code.
- Fix all failures. Never skip as "out of scope" or "pre-existing".
- Always run git hooks. Never use `--no-verify`. If a hook blocks, fix the issue.
- A task is not done unless covered by tests.

### Workflow

- Create PRs for non-trivial changes. No direct pushes to main.
- Branch naming: `feat/<piece>-<description>`, `fix/<description>`, `refactor/<description>`.
- Commit frequently with meaningful messages - logical changes separated.
- For non-trivial tasks, go into plan mode unless instructed not to.
- Report script/tool failures before manual fallback. Never silently work around broken tooling - report error, diagnose, fix.
- Address ALL review comments before merging - every single one, no exceptions. There are no ignorable issues. Fix them all. Disagree = respond in the review explaining why, but still fix or improve.

### Pre-Push Checklist (MANDATORY)

Before pushing any branch, run these in order. Do not skip steps.

1. **`/deslop`** - Clean AI artifacts: ghost code, debug statements, console.logs, stale comments.
2. **`/simplify`** - Review changed code for reuse, quality, efficiency. Fix issues found.
3. **`/sync-docs`** - Update documentation to match code changes (CLAUDE.md, design docs, comments).
4. **`/drift-detect`** - Compare implementation against `docs/design/` specs. Flag and fix drift.
5. **`/orchestrate-review`** - Multi-pass code review (quality, security, performance, test coverage).
6. **`/enhance`** - Run ONLY when skills, memory, hooks, or agent prompts are involved. Analyze and fix all HIGH/MEDIUM issues.

If any step finds issues, fix them before proceeding to the next step. The push happens only after all steps pass clean.

### Problem Solving

- Never guess-fail-guess-fail. Search the web for the correct approach.
- Fetch web resources fresh. Don't rely on cached/stale data.
- Do not give up easily. Keep digging when a challenge appears.

### Safety

- NEVER kill all node processes - only kill specific PIDs if necessary.

</critical-rules>
