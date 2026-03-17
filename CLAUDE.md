# Cairn

> Open-source, self-hosted, always-on personal agent OS. Written in Go.
> Models propose, humans dispose. No irreversible side effects without explicit approval.

Not a chatbot. Not a coding assistant. Not a notification hub. All three, unified under a single binary that watches your world, acts on your behalf, learns over time, and stays on 24/7.

## Stack

Go 1.25 single binary + SQLite (modernc, pure Go, no CGO) + SvelteKit 5 frontend (Svelte 5 runes, Tailwind v4, static adapter embedded via `embed.FS`).

## The Nine Pieces

| # | Piece | Status | Package |
|---|-------|--------|---------|
| 1 | Event Bus - typed async pub/sub backbone | Done | `internal/eventbus/` |
| 2 | LLM Client - multi-provider streaming, retry/fallback/budget | Done | `internal/llm/` |
| 3 | Tool System - type-safe tools, registry, mode filtering, permissions | Not started | — |
| 4 | Agent Core - ReAct loop, sessions, modes (talk/work/coding) | Not started | — |
| 5 | Task Engine - priority queue, worktree isolation, leases | Not started | — |
| 6 | Memory System - semantic + episodic + procedural, RAG, Soul | Not started | — |
| 7 | Signal Plane - source polling, webhooks, event ingestion, dedup | Not started | — |
| 8 | Plugin & Skill System - lifecycle hooks, SKILL.md, ClawHub-compatible | Not started | — |
| 9 | Server & Protocols - HTTP, SSE, MCP, A2A, ACP, auth | Not started | — |

Frontend scaffold is up with 52 tests across stores, utils, and API client.

## Phases

```
Phase 1: Foundation (event bus + LLM + SQLite)                [DONE]
Phase 2: Core Systems (tools | tasks | memory) in parallel    [NEXT]
Phase 3: Agent Core (ReAct loop wires all together)
Phase 4: Server + Signal Plane + Plugins in parallel
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

## What Makes Cairn Different

1. **Worktree isolation** - every coding task gets its own git worktree. No branch conflicts.
2. **Permission engine** - wildcard rules scoped per agent mode, per tool, per file pattern, per approval policy.
3. **Always-on with proactive behavior** - Soul (behavioral identity), episodic + semantic + procedural memory. Doesn't wait to be spoken to.
4. **Skill ecosystem compatibility** - OpenClaw SKILL.md format. ClawHub skills work unmodified.
5. **Multi-protocol** - A2A, MCP, ACP first-class. Not afterthoughts.
6. **Event-sourced sessions** - append-only, branchable, compactable, replayable.
7. **Single binary** - `scp cairn server:/usr/local/bin/`. No Node, no Python, no Docker.

## Current Structure

```
cmd/cairn/main.go           CLI entry point (cairn chat "message")
internal/
  config/config.go          Env-based config, provider auto-detection (GLM/OpenAI)
  db/db.go                  SQLite open + WAL pragmas
  db/migrate.go             Embedded SQL migrations
  db/migrations/001_init.sql  Tables: events, tasks, approvals, sessions, messages, memories, source_state
  eventbus/bus.go           Typed pub/sub (generics), sync + async delivery, backpressure
  eventbus/events.go        Event types: feed, LLM, task, memory, system
  llm/types.go              Request, Message, ContentBlock variants, Event variants, Provider interface
  llm/registry.go           Multi-provider registry, resolve, fallback, retry wrapper
  llm/openai.go             OpenAI-compatible provider
  llm/glm.go                GLM (ZhipuAI) provider
  llm/sse.go                SSE stream parser
  llm/budget.go             Token budget tracker
  llm/retry.go              Retry with exponential backoff + fallback
frontend/                   SvelteKit 5 app
  src/routes/               today, chat, ops, memory, agents, skills, soul, settings
  src/lib/stores/           Svelte 5 rune stores (app, chat, feed, memory, tasks, sse)
  src/lib/components/       chat/, feed/, layout/, memory/, tasks/
  src/lib/api/client.ts     Typed REST client
  src/lib/types.ts          Domain types matching Go API contract
```

## Commands

```bash
# Backend
go build -o cairn ./cmd/cairn
go test ./...
./cairn chat "hello"

# Frontend (from frontend/)
npm run dev         # Dev server
npm run build       # Static build to dist/
npm run check       # Svelte + TypeScript check
npm test            # Vitest (52 tests)
```

## Env Vars

- `LLM_API_KEY` / `GLM_API_KEY` / `OPENAI_API_KEY` - required
- `LLM_PROVIDER` - "glm" or "openai" (auto-detected from key var)
- `LLM_MODEL`, `LLM_BASE_URL`, `LLM_FALLBACK_MODEL` - overrides
- `PORT` (8787), `HOST` (0.0.0.0), `DATABASE_PATH` (./data/pub.db)
- `SOUL_PATH` (./SOUL.md), `SKILL_DIRS` (./.pub/skills)

## Design Docs

Full design specs live in `docs/design/`:
- `VISION.md` - architecture, differentiators, success criteria
- `PHASES.md` - implementation phases with dependency graph
- `FRONTEND_AGENT_BRIEF.md` - frontend spec, API contract, SSE events, views
- `pieces/01-event-bus.md` through `pieces/11-channel-adapters.md` - per-piece design

<critical-rules>

## Rules

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
- Commit frequently with meaningful messages - logical changes separated.
- For non-trivial tasks, go into plan mode unless instructed not to.
- Report script/tool failures before manual fallback. Never silently work around broken tooling - report error, diagnose, fix.
- Address ALL review comments before merging - even minor ones. Disagree = respond in the review, don't ignore.

### Problem Solving

- Never guess-fail-guess-fail. Search the web for the correct approach.
- Fetch web resources fresh. Don't rely on cached/stale data.
- Do not give up easily. Keep digging when a challenge appears.

### Safety

- NEVER kill all node processes - only kill specific PIDs if necessary.

</critical-rules>
