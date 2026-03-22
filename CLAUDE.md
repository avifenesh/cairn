# Cairn

> Open-source, self-hosted, always-on personal agent OS. Written in Go.
> Models propose, humans dispose. No irreversible side effects without explicit approval.
> Stack: Go 1.25 + SQLite (pure Go) + SvelteKit 5 (embedded via embed.FS)

## Architecture

```
Signal Plane → Event Bus ← Agent System → Tool System
     ↕              ↕            ↕              ↕
  11 Pollers     SQLite      Always-On Loop   54 Tools
  Webhooks       Store       Orchestrator     Permissions
  SSE push       Memory      ReAct Agents     Mode filtering
                 Sessions    Subagents        MCP adapter
                 Approvals   Compaction       Skills
```

**Agent system (three layers):**
- **Loop** (60s tick) — checks crons → executes pending tasks → runs orchestrator if idle
- **Orchestrator** — LLM-powered management brain. Gathers system state (feeds, errors, memories, subagents), calls LLM to decide actions: approve/reject memories, spawn subagents, submit tasks, notify, escalate to human. Runs every 5min when idle. Max 5 actions per tick.
- **ReAct agents** — execute work. Main agent (talk/work/coding modes) + 4 subagent types (researcher/coder/reviewer/executor). Two-level max nesting. Session streaming via channels.

**Subagent types:**
- `researcher` (15 rounds, read-only tools) — investigation, data gathering
- `coder` (50 rounds, all tools, worktree isolation) — implementation
- `reviewer` (10 rounds, read + shell) — code quality analysis
- `executor` (10 rounds, shell + file tools) — command execution, validation

**Key design decisions:**
- Event bus: Go generics `Subscribe[E](bus, handler)`, `Publish[E](bus, event)`
- LLM: `Provider` interface with `ID()`, `Stream()`, `Models()`
- Streaming: `<-chan Event` (TextDelta, ReasoningDelta, ToolCallDelta, MessageEnd, StreamError)
- Sessions: append-only events, compaction at 150K tokens (keep 10 recent pairs, summarize old)
- Steering: user can inject messages into running sessions between ReAct rounds (normal/urgent/stop)
- Approvals: human-in-the-loop gates for irreversible actions (merge PR, send email, deploy)
- SQLite: WAL mode, single writer, foreign keys ON, MMAP 256MB
- Migrations: `//go:embed`, applied in filename order, tracked in schema_migrations
- Frontend: Svelte 5 runes (`.svelte.ts` stores), `tailwind-variants` for component styling

17 packages in `internal/`, 11 architecture specs in `docs/design/pieces/`.

## Project Structure

```
cmd/cairn/main.go             CLI entry point (cairn chat | cairn serve)
internal/
  config/config.go            Env-based config, provider auto-detection (GLM/OpenAI)
  db/                         SQLite open + WAL pragmas, embedded migrations
  eventbus/                   Typed pub/sub (generics), sync + async + stream delivery
  llm/                        Provider interface, GLM + OpenAI providers, SSE parser, retry, budget
  tool/                       Tool interface, Define[P] generics, registry, permission engine
  tool/builtin/               54 built-in tools: file ops, shell, git, memory, feed, tasks, cron, vision, etc.
  task/                       Task store, priority queue, worktree manager, lease claiming, approvals
  memory/                     Memory store, RAG search + MMR, embedder, Soul loader, extraction
  agent/                      Always-on loop, orchestrator, ReAct agents, subagents, compaction, sessions
  auth/                       WebAuthn biometric authentication (passkeys)
  channel/                    Telegram, Discord, Slack adapters, notification routing
  cron/                       Cron scheduler + SQLite store
  plugin/                     Lifecycle hooks (agent/tool/LLM), logging plugin, budget plugin
  server/                     HTTP server, REST routes, SSE broadcaster, auth, static (embed+FS), webhooks
  skill/                      SKILL.md parser, discovery, hot-reload, ClawHub marketplace
  mcp/                        MCP server (expose tools) + MCP client (consume external servers)
  signal/                     Signal plane: event store, scheduler, 11 pollers, webhooks, digest
  voice/                      Whisper STT + edge-tts TTS
frontend/                     SvelteKit 5 app + embed.FS package for production binary
  src/routes/                 today, chat, ops, memory, agents, skills, soul, settings
  src/lib/stores/             Reactive stores (app, chat, feed, memory, tasks, sse, offline-queue, keyboard-nav)
  src/lib/components/         chat/, feed/, layout/, memory/, tasks/, shared/
  src/lib/api/client.ts       Typed REST client with normalization layer
  src/lib/utils/              markdown (marked+DOMPurify), time (relative), tts (playback)
docs/design/                  Architecture specs (VISION, PHASES, pieces/01-11)
```

## Development

```bash
# Backend (from repo root)
go vet ./...                                        # Lint - run before every commit
go test -race ./...                                 # Tests with race detector
go build -o cairn ./cmd/cairn                       # Build binary (dev, filesystem frontend)
go build -tags embed_frontend -o cairn ./cmd/cairn  # Build with embedded frontend (production)
./cairn chat "hello"                                # CLI chat (ReAct agent)
./cairn serve                                       # HTTP server on :8787 (prod: PORT=8788)

# Frontend (from frontend/)
pnpm dev                        # Dev server
pnpm build                      # Static build to dist/
pnpm check                      # Svelte + TypeScript check
pnpm test                       # Vitest

# Make targets
make test                       # Tests with race detector
make lint                       # Formatting + vet
make build                      # Dev binary
make build-prod                 # Production binary with embedded frontend
make dev                        # Run dev server

# Full validation
go vet ./... && go test -race ./... && cd frontend && pnpm check && pnpm test
```

Tests: `*_test.go` alongside source (Go), `*.test.ts` alongside stores (frontend).

## Deployment (Production - agntic.garden)

Cairn serves agntic.garden via systemd + Caddy reverse proxy.

```
Cloudflare (DNS + proxy) → Caddy (:443, TLS) → Cairn (:8788)
```

**Services:**
- `cairn.service` — systemd unit, port 8788, env from `/home/ubuntu/.cairn/.env.cairn`
- `caddy.service` — TLS reverse proxy, config at `/etc/caddy/Caddyfile`

**Key paths:**
- Binary: `/home/ubuntu/cairn/cairn-prod`
- Env: `/home/ubuntu/.cairn/.env.cairn`
- DB: `/home/ubuntu/.cairn/data/cairn.db`
- SOUL: `/home/ubuntu/.cairn/SOUL.md`
- Caddyfile: `/etc/caddy/Caddyfile` (proxies all to 8788, CouchDB on /obsidian-vault)
- Certs: `/etc/caddy/certs/origin-cert.pem` + `origin-key.pem` (Cloudflare Origin CA)

**Build & deploy:**
```bash
./scripts/cairn-server.sh build     # Compiles frontend + Go binary (ONLY from main branch)
sudo systemctl restart cairn        # Deploy (picks up new binary)
```

**DO NOT** start cairn via `nohup` or manual process — always use systemd.
The `cairn-server.sh` script's start/stop/restart delegate to systemd.

**Logs:** `journalctl -u cairn -f`

**Auth:** WebAuthn biometric (fingerprint/face) + WRITE_API_TOKEN fallback.
Registration at Settings > Security. Sessions via `cairn_session` HttpOnly cookie.

### Build Safety Rules (CRITICAL — read before building)

Multiple agents work on this repo via git worktrees. The production binary is shared.
Unsafe builds caused data loss (split databases, lost settings, broken auth).

1. **`cairn-server.sh build` ONLY from main branch.** The script enforces this — it refuses
   to build from feature branches. This prevents incomplete feature code from overwriting prod.
2. **Frontend-only agents use `cairn-server.sh build-fe`** — builds SvelteKit only, never
   touches the Go binary. Safe from any branch.
3. **Never start cairn outside systemd.** No `nohup`, no `&`, no manual `./cairn-prod serve`.
   The script enforces this — start/stop/restart all delegate to `sudo systemctl`.
4. **All paths are absolute** in `.env.cairn`. Never use relative paths — different worktrees
   resolve `./data` to different directories, causing split databases.
5. **One database**: `/home/ubuntu/.cairn/data/cairn.db` — all worktrees,
   all agents, all processes must use this same file. Config overrides saved to
   `/home/ubuntu/.cairn/data/config.json`.
6. **Build lock**: `/tmp/cairn-build.lock` prevents concurrent builds.

## Env Vars

Full list from `internal/config/config.go`. 108 distinct var names (including aliases).

**Required (one of):**
- `LLM_API_KEY` — primary LLM API key
- `GLM_API_KEY` / `ZHIPU_API_KEY` — aliases (auto-sets provider=glm)
- `OPENAI_API_KEY` — alias (auto-sets provider=openai)

**LLM:**
- `LLM_PROVIDER` ("glm"|"openai"; auto-detected when using `GLM_API_KEY` or `OPENAI_API_KEY`; defaults to "glm" when only `LLM_API_KEY` is set)
- `LLM_MODEL` (default: glm-5-turbo for GLM, gpt-4o for OpenAI)
- `LLM_BASE_URL` (default: https://api.z.ai/api/coding/paas/v4 for GLM, https://api.openai.com/v1 for OpenAI)
- `LLM_FALLBACK_MODEL` — fallback model on persistent failure
- `GLM_MODEL`, `GLM_BASE_URL`, `GLM_FALLBACK_MODEL` — legacy GLM-specific aliases
- `OPENAI_BASE_URL`, `ZHIPU_BASE_URL` — provider-specific base URL aliases

**Server:**
- `PORT` (8787)
- `HOST` (0.0.0.0)
- `DATABASE_PATH` (./data/cairn.db)
- `WRITE_API_TOKEN` — required for write endpoints
- `READ_API_TOKEN` — optional, if unset read endpoints are open
- `FRONTEND_ORIGIN` — CORS allowed origin

**Signal Plane:**
- `GH_TOKEN` / `GITHUB_TOKEN` — GitHub personal access token
- `GH_ORGS` — comma-separated GitHub org names to track
- `GH_OWNER` — your GitHub login (for self-filter on activity)
- `GH_TRACKED_REPOS` — comma-separated explicit repos (empty = auto-detect)
- `GH_BOT_FILTER` — comma-separated additional bot logins to filter
- `GH_METRICS_INTERVAL` (14400) — seconds between GitHub metrics polls (4h)
- `GMAIL_ENABLED` (false) — enable Gmail poller
- `GMAIL_FILTER_QUERY` (-category:promotions -category:social -category:forums) — Gmail search filter
- `CALENDAR_ENABLED` (false) — enable Calendar poller
- `CALENDAR_LOOKAHEAD_H` (48) — calendar lookahead in hours
- `RSS_ENABLED` (false) — enable RSS/Atom poller
- `RSS_FEEDS` — comma-separated RSS/Atom feed URLs
- `SO_ENABLED` (false) — enable Stack Overflow poller
- `SO_TAGS` — comma-separated SO tags to monitor
- `SO_API_KEY` — SO API key (optional, higher rate limit)
- `SO_POLL_INTERVAL` (60) — SO poll interval in minutes
- `DEVTO_ENABLED` (false) — enable Dev.to poller
- `DEVTO_TAGS` — comma-separated Dev.to tags to monitor
- `DEVTO_USERNAME` — Dev.to username to follow
- `DEVTO_POLL_INTERVAL` (30) — Dev.to poll interval in minutes
- `HN_KEYWORDS` — comma-separated Hacker News keyword filter
- `HN_MIN_SCORE` (0) — minimum HN story score
- `POLL_INTERVAL` (300) — default poll interval in seconds (also accepts `POLL_INTERVAL_MS` in ms)
- `REDDIT_SUBS` — comma-separated subreddit names
- `NPM_PACKAGES` — comma-separated npm packages to track
- `CRATES_PACKAGES` / `CRATES` — comma-separated crates.io packages to track
- `WEBHOOK_SECRETS` — JSON map of name->HMAC secret (e.g. `{"github":"abc123"}`)

**Memory:**
- `MEMORY_CONTEXT_BUDGET` (4000) — total token budget for context builder
- `MEMORY_HARD_RULE_RESERVE` (500) — tokens reserved for hard rules
- `MEMORY_DECAY_HALF_LIFE` (30) — days, relevance decay half-life
- `MEMORY_STALE_THRESHOLD` (14) — days, penalty for unused memories
- `MEMORY_AUTO_EXTRACT` (true) — auto-extract memories from sessions

**Agent:**
- `AGENT_TICK_INTERVAL` (60) — orchestrator tick interval in seconds
- `REFLECTION_INTERVAL` (1800) — reflection cycle interval in seconds
- `TALK_MAX_ROUNDS` (40) — max tool rounds in talk mode
- `WORK_MAX_ROUNDS` (80) — max tool rounds in work mode
- `CODING_MAX_ROUNDS` (400) — max tool rounds in coding mode
- `CODING_ALLOWED_REPOS` — comma-separated absolute repo paths where coding is allowed (empty = cwd only)

**Channels:**
- `TELEGRAM_BOT_TOKEN` — Telegram bot token
- `TELEGRAM_CHAT_ID` — Telegram chat ID (int64)
- `DISCORD_BOT_TOKEN` — Discord bot token
- `DISCORD_CHANNEL_ID` — Discord channel ID
- `SLACK_BOT_TOKEN` — Slack bot token
- `SLACK_APP_TOKEN` — Slack app token (Socket Mode)
- `SLACK_CHANNEL_ID` — Slack channel ID
- `CHANNEL_SESSION_TIMEOUT` (240) — channel session idle timeout in minutes
- `PREFERRED_CHANNEL` — default outbound notification channel (e.g. "telegram")
- `QUIET_HOURS_START` (-1) — quiet hours start 0-23 (-1 = disabled)
- `QUIET_HOURS_END` (-1) — quiet hours end 0-23 (-1 = disabled)
- `QUIET_HOURS_TZ` (UTC) — IANA timezone for quiet hours
- `MUTED_SOURCES` — comma-separated source names that skip notifications
- `NOTIF_MIN_PRIORITY` (low) — minimum priority for notifications ("low"|"medium"|"high")
- `CHANNEL_ROUTING` — JSON map of source -> channel (e.g. `{"github_signal":"telegram"}`)

**MCP:**
- `MCP_SERVER_ENABLED` (false) — expose Cairn tools as MCP server
- `MCP_PORT` (3001) — MCP server port
- `MCP_TRANSPORT` (http) — MCP transport ("stdio"|"http"|"both")
- `MCP_WRITE_RATE_LIMIT` (100) — write requests per minute on MCP server
- `MCP_SERVERS` — JSON array of MCP client server configs to connect to

**Embeddings:**
- `EMBEDDING_ENABLED` (true when API key present) — enable semantic embeddings
- `EMBEDDING_MODEL` (embedding-3 for GLM, text-embedding-3-small for OpenAI) — embedding model
- `EMBEDDING_DIMENSIONS` (2048) — embedding vector dimensions
- `EMBEDDING_BASE_URL` (defaults to LLM base URL) — embedding API endpoint
- `EMBEDDING_API_KEY` (defaults to LLM API key) — embedding API key

**Session Compaction:**
- `COMPACTION_TRIGGER_TOKENS` (150000) — context length that triggers compaction
- `COMPACTION_KEEP_RECENT` (10) — number of recent turns to keep verbatim
- `COMPACTION_MAX_TOOL_OUTPUT` (32000) — max chars of tool output preserved per turn

**Voice:**
- `VOICE_ENABLED` (false) — enable voice input/output
- `WHISPER_URL` (http://127.0.0.1:8178) — Whisper STT server URL
- `TTS_VOICE` (en-US-BrianNeural) — edge-tts voice name

**Web Tools (fallback when Z.ai disabled):**
- `SEARXNG_URL` — SearXNG instance URL for web search
- `WEB_FETCH_TIMEOUT` (30) — HTTP fetch timeout in seconds
- `WEB_FETCH_MAX_SIZE` (5MB) — max response size in bytes

**Z.ai (GLM-specific):**
- `ZAI_WEB_ENABLED` (true when provider=glm) — enable Z.ai web/search tools
- `ZAI_BASE_URL` (https://api.z.ai/api/mcp) — Z.ai MCP endpoint
- `ZAI_API_KEY` — Z.ai MCP key (falls back to LLM_API_KEY)
- `ZAI_VISION_ENABLED` (true when provider=glm) — enable Z.ai vision tools

**Budget:**
- `BUDGET_DAILY_CAP` (0) — daily LLM spend cap USD (0 = unlimited)
- `BUDGET_WEEKLY_CAP` (0) — weekly LLM spend cap USD (0 = unlimited)
- Aliases: `BEDROCK_DAILY_BUDGET_USD`, `IDLE_BUDGET_CAP_USD` (daily), `BEDROCK_WEEKLY_BUDGET_USD` (weekly)

**Paths:**
- `SOUL_PATH` (./SOUL.md — production: ~/.cairn/SOUL.md)
- `DATA_DIR` (./data — production: ~/.cairn/data)
- `SKILL_DIRS` — extra skill directories appended to default search path
- Default skill scan order: `./skills` → `~/.cairn/skills` → `.cairn/skills` → `.agents/skills` → SKILL_DIRS. Last-wins on name conflict. `InstallDir()` returns last entry.

**Feature Flags:**
- `CODING_ENABLED` (false) — enable coding mode and worktree isolation
- `IDLE_MODE_ENABLED` (false) — enable always-on idle/proactive agent loop

## Design Docs

Full design specs live in `docs/design/`. Phase plans are archived - all 9 phases complete.
- `VISION.md` - architecture, differentiators, success criteria
- `PHASES.md` - implementation phases with dependency graph (all phases complete, kept as reference)
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
- **Address ALL review comments before merging — ALL means ALL.** Every high, medium, and low priority comment must be fixed. No skipping "medium" or "low" severity. No "that's just a suggestion". If a reviewer (human or bot) flags it, fix it. The only exception: if a comment is factually wrong, respond in the review explaining why — but still improve the code. This is non-negotiable.

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
