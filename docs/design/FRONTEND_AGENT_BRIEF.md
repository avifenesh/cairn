# Frontend Agent Brief — Cairn

> Everything a frontend developer needs to build the Svelte 5 dashboard from scratch.
> Read this file first. No other context required to start.

## What You're Building

A **Svelte 5 + SvelteKit** dashboard for Cairn — a personal agent operating system. The dashboard replaces a 13k-line vanilla JS monolith (`app.js`) with a modern component architecture. It will be embedded in a Go binary via `embed.FS`.

**Location:** `/home/ubuntu/cairn/frontend/`

## Stack

| Layer | Choice | Why |
|-------|--------|-----|
| Framework | Svelte 5 (runes) | Compile-time reactivity, smallest bundle, best SSE ergonomics |
| Meta-framework | SvelteKit (static adapter) | File-based routing, SSR not needed — static output only |
| Components | shadcn-svelte | Copy-paste Tailwind components, accessible, Svelte 5 compatible |
| Styling | Tailwind CSS v4 | CSS-first config, design tokens via `@theme` |
| Icons | Lucide Svelte | Tree-shakeable, used by shadcn |
| Fonts | Geist (headings) + Inter (body) + Geist Mono (code) | Upgraded from v1 |
| Markdown | marked + DOMPurify | Same libs as v1, proven |

## Design Tokens

Source of truth: `/home/ubuntu/pub/design-tokens.json` (copy into the frontend project).

The token file defines:
- **Backgrounds**: `bg-0` through `bg-4` (depth layers, dark-first)
- **Text**: `primary`, `secondary`, `tertiary`
- **Accent**: `#E87B9A` (pink-rose) + dim/glow opacities
- **Source colors**: per-source (github purple, reddit orange, gmail red, etc.)
- **Status**: success green, warning yellow, error red
- **Border**: base + 3 opacity levels (subtle/default/strong)
- **Radius**: sm(6) md(8) lg(10) xl(16) full(9999)
- **Font stacks**: Geist (headings), Inter (body), Geist Mono (code)
- **Shadows**: sm/md/lg
- **Motion**: easing (out/spring), duration (fast 120ms, normal 200ms, slow 350ms)
- **Layout**: sidebar 240px, header 56px, toolbar 48px, touch target 44px
- **Light theme**: full override set

Map these to Tailwind's `@theme` in CSS. Support dark (default) and light via `[data-theme="light"]`.

## Views (Routes)

| Route | View | Description |
|-------|------|-------------|
| `/` or `/today` | Today | Dashboard: greeting, stats, activity, quick actions |
| `/ops` | Ops Inbox | Task cards, approval cards, bulk actions |
| `/chat` | Chat | Conversational AI with streaming, modes, voice |
| `/memory` | Memory | Memory browser, search, accept/reject/edit |
| `/agents` | Agents | Fleet status, agent cards, session viewer |
| `/skills` | Skills | Skill catalog, activation status |
| `/soul` | Soul | SOUL.md editor with git history, patches |
| `/settings` | Settings | Theme, density, mood, notifications |

**Mobile (< 768px):** Bottom nav with 4 items (Today, Ops, Chat, More). More menu shows remaining views. Sidebar hidden.

## Layout Architecture

```
Desktop (>= 768px):
+--------------------------------------------------+
| Header: Health dot | Cmd+K | Theme | Budget       |
+----------+---------------------+-----------------+
| Sidebar  | Main Content        | Context Panel   |
| (240px)  | (route-dependent)   | (collapsible)   |
+----------+---------------------+-----------------+
| Status Bar: SSE status | agents | last poll       |
+--------------------------------------------------+

Mobile (< 768px):
+------------------------+
| Header                 |
+------------------------+
| Main Content           |
| (full width)           |
+------------------------+
| Today|Ops|Chat|More    |  <- Bottom nav
+------------------------+
```

## API Contract (The Shared Interface)

The Go backend reimplements the same REST API. Here's what the frontend consumes:

### Base URL
Default: `http://localhost:8787`. Configurable via env or query param.

### Auth
- **WebAuthn cookie**: `pub_session` HttpOnly cookie (primary auth after biometric login)
- **Token header**: `x-api-token: <token>` for API token auth
- **SSE auth**: append `?token=<token>` as query param (EventSource can't send headers)
- **Read endpoints**: optional `READ_API_TOKEN` (open when not configured)
- **Write endpoints**: require `WRITE_API_TOKEN`

### Key Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Health check (always open) |
| GET | `/v1/dashboard` | Stats + feed + poller + readiness |
| GET | `/v1/feed?limit=50&before=&source=&unread=` | Paginated feed |
| POST | `/v1/feed/:id/read` | Mark read |
| POST | `/v1/feed/read-all` | Mark all read |
| GET | `/v1/stream` | SSE stream (see events below) |
| GET | `/v1/tasks?status=&type=` | Task list |
| POST | `/v1/tasks/:id/cancel` | Cancel task |
| GET | `/v1/approvals?status=pending` | Pending approvals |
| POST | `/v1/approvals/:id/approve` | Approve |
| POST | `/v1/approvals/:id/deny` | Deny |
| GET | `/v1/assistant/sessions` | Chat session list |
| GET | `/v1/assistant/sessions/:id` | Session messages |
| POST | `/v1/assistant/message` | Send chat (returns 202 + taskId) |
| POST | `/v1/assistant/voice` | Upload audio (multipart) |
| GET | `/v1/assistant/voice/tts?text=&format=` | TTS audio |
| GET | `/v1/memories?status=&category=` | Memory list |
| GET | `/v1/memories/search?q=&limit=10` | Semantic search |
| POST | `/v1/memories` | Create memory |
| POST | `/v1/memories/:id/accept` | Accept proposed memory |
| POST | `/v1/memories/:id/reject` | Reject proposed memory |
| GET | `/v1/fleet` | Agent fleet status |
| GET | `/v1/skills` | Skill catalog |
| GET | `/v1/soul` | Read SOUL.md |
| PUT | `/v1/soul` | Write SOUL.md |
| GET | `/v1/soul/history` | Git history |
| GET | `/v1/soul/patches` | Proposed patches |
| GET | `/v1/metrics` | Runtime metrics (for budget display) |
| GET | `/v1/costs` | Cost data |
| GET | `/v1/status` | System status |
| POST | `/v1/auth/login/start` | WebAuthn challenge |
| POST | `/v1/auth/login/complete` | WebAuthn verify |
| POST | `/v1/auth/register/start` | Register passkey |
| POST | `/v1/auth/register/complete` | Complete registration |
| POST | `/v1/auth/logout` | Logout |

### SSE Events (`GET /v1/stream`)

| Event | Payload | Use |
|-------|---------|-----|
| `ready` | `{ clientId }` | Connection established |
| `feed_update` | `{ item }` | New feed item |
| `poll_completed` | `{ source, newCount }` | Poll cycle done |
| `task_update` | `{ task }` | Task status change |
| `approval_required` | `{ approval }` | New approval needed |
| `assistant_delta` | `{ taskId, deltaText }` | Streaming LLM text |
| `assistant_end` | `{ taskId, messageText }` | LLM response complete |
| `assistant_reasoning` | `{ taskId, round, thought }` | ReAct reasoning trace |
| `assistant_tool_call` | `{ taskId, toolName, phase, args?, result? }` | Tool execution |
| `memory_proposed` | `{ memory }` | New memory proposed |
| `memory_accepted` | `{ memoryId }` | Memory accepted |
| `soul_updated` | `{ sha }` | SOUL.md changed |
| `digest_ready` | `{ digest }` | New digest available |
| `coding_session_event` | `{ sessionId, type, data }` | Coding session lifecycle |
| `agent_progress` | `{ agentId, message }` | Agent status update |
| `skill_activated` | `{ skillName }` | Skill activated |

SSE features:
- `Last-Event-ID` header for reconnection replay (1000-entry buffer)
- `retry` directive sent on connect
- Reconnect with exponential backoff (base 5s, max 60s, random jitter)

## Chat System

### Three modes: `talk`, `work`, `coding`
- **talk**: conversational, 10 tool rounds
- **work**: task-oriented, 10 tool rounds
- **coding**: file operations, 100 tool rounds

### Flow
1. User sends message via `POST /v1/assistant/message` with `{ message, mode, sessionId }`
2. Server returns `202 { taskId }`
3. LLM response streams via SSE `assistant_delta` events (keyed by `taskId`)
4. `assistant_end` marks completion with final text
5. Tool calls appear via `assistant_tool_call` events during streaming
6. Reasoning traces via `assistant_reasoning` events

### Streaming rendering
- Always render through markdown parser (marked.js) from first token
- 80ms debounce on re-render during streaming
- Blinking cursor during active stream
- Code blocks get copy buttons after stream ends

### Voice
- **Input**: MediaRecorder → FormData upload to `/v1/assistant/voice`
- **Output**: fetch `/v1/assistant/voice/tts?text=...` → play Audio element

### Custom modes
Users can define custom modes with: name, description, promptInjection (custom system prompt). Stored in localStorage.

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `1-9, 0` | Switch view |
| `j/k` | Navigate items |
| `o` | Open item URL |
| `r` | Mark read |
| `x` | Toggle selection |
| `s` | Manual sync |
| `a` (ops) | Approve |
| `d` (ops) | Deny |
| `Cmd+K` | Command palette |
| `?` | Help modal |
| `t` | Toggle theme |
| `Escape` | Close modals |

## Theming

- **Dark** (default) / **Light** via `[data-theme="light"]` on `<html>`
- **Mood packs**: default, dawn, ocean, night — override accent colors
- **Density**: comfortable (default), balanced, dense — adjust spacing/font-size
- **Auto-mood**: time-based (dawn 6-10, default 10-18, ocean 18-22, night 22-6)

## Key UI Patterns

1. **Command palette** (Cmd+K) — fuzzy search to any view, session, memory, action
2. **Keyboard triage** — j/k navigate, a/d approve/deny, o open
3. **Health dot** — green/yellow/red in header
4. **Toast notifications** — disconnects, errors, confirmations
5. **Skeleton loading** — for every async panel
6. **Optimistic mutations** — approve/deny instant, server confirms async
7. **Offline queue** — write actions queued when SSE disconnected (50 cap, 10min expiry)
8. **Pull-to-refresh** — mobile touch gesture
9. **Swipe-to-dismiss** — feed cards on mobile

## Build Output

```bash
cd /home/ubuntu/cairn/frontend
npm run build   # or pnpm build
# Output: dist/ directory (static files)
```

The Go backend embeds `frontend/dist/` via `embed.FS` and serves it at `/`.

## Getting Started

### Phase 1: Scaffold + Layout
1. `npx sv create frontend` (Svelte 5 + SvelteKit + TypeScript)
2. Install: `@sveltejs/adapter-static`, `tailwindcss`, `shadcn-svelte`
3. Configure Tailwind with design tokens
4. Build layout shell: sidebar, header, bottom nav, status bar
5. Set up file-based routes for all 8 views
6. Mock data for development (no backend needed yet)

### Phase 2: SSE + API Client
1. Create SSE store (`$state`-based, auto-reconnect)
2. Create typed API client (REST endpoints)
3. Wire stores to components

### Phase 3: Core Views
1. Chat panel (streaming, markdown, tool chips, reasoning)
2. Today dashboard (stats, activity, quick actions)
3. Ops inbox (task cards, approval cards, bulk actions)
4. Memory browser (search, cards, accept/reject)

### Phase 4: Remaining Views + Polish
1. Agents, Skills, Soul, Settings
2. Command palette
3. Mobile optimization
4. Voice input/output

### Phase 5: Testing
1. Set up vitest with jsdom environment + Svelte compiler
2. Unit tests for stores: chat (streaming lifecycle, tool calls, reasoning), feed (dedup, unread), tasks (upsert, optimistic approvals), memory (resolve, proposed count), offline-queue (cap, expiry, drain), keyboard-nav (movement, selection)
3. Unit tests for utilities: markdown (rendering, XSS sanitization), time (relative formatting), touch (pull-to-refresh, swipe-to-dismiss)
4. API client tests: ApiError, fetch behavior, credentials
5. Test script: `pnpm test` (vitest run), `pnpm test:watch` (vitest)
6. 221 tests across 27 test files (stores, utils, API, components)

## Completion Status

All frontend phases (1-5), subphases (10.1-10.12), Phase 6 hardening, full redesign, and Phase 6 frontend near complete (13/15 items). 221 tests across 27 files. 14 shadcn-svelte components. Cairn design system (emerald accent, zinc backgrounds, Geist font). 10 reactive stores (app, chat, feed, memory, tasks, sse, offline-queue, keyboard-nav, skills, status). Shared constants in `$lib/constants.ts`.

### Full Redesign (merged)

Design system rewrite: `--pub-*` → `--cairn-*`, emerald `#10B981` accent, zinc backgrounds, Geist + Geist Mono fonts. 14 shadcn-svelte components (button, card, badge, tooltip, separator, scroll-area, dialog, dropdown-menu, input, toggle, avatar, skeleton, alert, tabs). All 26 custom components and 8 pages redesigned. Tabs use underline style, buttons are softer rounded-lg, density scales root font-size. Token gate for auth, SSE integration working with GLM backend.

### Next: Chat Improvements (frontend-only, no backend dependency)

| # | Item | Priority | Description |
|---|------|----------|-------------|
| C.1 | Copy button on messages | high | Copy message content to clipboard, "Copied" feedback |
| C.2 | Better message styling | high | Markdown rendering polish, code block headers, proper prose styling |
| C.3 | Context panel wiring | high | Wire tool calls and reasoning from SSE to context panel in real-time |
| C.4 | Old conversation loading | high | Session picker loads message history, scroll to bottom, continue chat |
| C.5 | Mode-specific styling | medium | Visual distinction per mode (talk/work/coding) — accent color, icon, border |
| C.6 | File attachment | medium | Attach files to messages, preview, send as multipart |
| C.7 | Voice input/output | medium | Wire VoiceButton to backend whisper endpoint, TTS playback |
| C.8 | Message actions bar | medium | **DONE PR #34** — hover bar: copy, remember this, create task |
| C.9 | Streaming improvements | medium | **DONE PR #28** — completedTaskIds guard, appendDelta no auto-create, split bug fixed |
| C.10 | Chat empty state | low | **DONE PR #30** — mode-aware suggestion chips, textarea focus on click |

### Frontend Tool/Skill UI (from PHASE6_PLAN.md items 6f.1-6f.22 — UNBLOCKED, backend done)

**Done (13/15):** 6f.1 (#29), 6f.2 (#34), 6f.4 (#38), 6f.5 (#34), 6f.6 (#38), 6f.7 (#35), 6f.8 (#32), 6f.10 (#40), 6f.11 (#29), 6f.12 (#38), 6f.13 (#36), 6f.14-17 (#40). Also: C.8 (#34), C.9 (#28), C.10 (#30).
**Remaining:** 6f.3 (feed actions — low priority), 6f.9 (skill detail — needs backend to wire handleListSkills + GET /v1/skills/:name). 6f.18 (SSE tool_executed) pending backend event.

### Phase 6: Hardening (approved improvements, independent of backend)

| # | Item | Priority |
|---|------|----------|
| 6.1 | Component tests — ErrorBoundary, FeedItem, ApprovalCard, ToolCallChip with @testing-library/svelte | high |
| 6.2 | Error boundaries — shared error boundary component, replace bare catch {} blocks | high |
| 6.3 | Consistent loading states — shared SkeletonList component across all views | medium |
| 6.4 | Feed pagination — infinite scroll or "Load more" using cursor-based before param | medium |
| 6.5 | Session persistence — persist current session ID in localStorage, restore on reload | medium |
| 6.6 | Accessibility — fix interactive span/div elements, manage tab order, remove a11y ignores | high |
| 6.7 | Auto-mood timer — setInterval in layout, toggle in Settings | low |
| 6.8 | Settings notifications — notification preferences (types, duration) | low |
| 6.9 | Debounced memory search — 300ms debounce on MemorySearch oninput | medium |
| 6.10 | Type-safe SSE parsing — try/catch around JSON.parse in every SSE handler | high |

## Reference Files

| File | Purpose |
|------|---------|
| `docs/design/pieces/10-frontend.md` | Full frontend design spec |
| `docs/design/pieces/11-channel-adapters.md` | Multi-channel architecture |
| `docs/design/PHASES.md` | Phase dependency graph |
| `/home/ubuntu/pub/design-tokens.json` | Design token source of truth (v1 repo) |
| `/home/ubuntu/pub/backend/FRONTEND_API_CONTRACT.md` | Full API spec, 66KB (v1 repo) |
| `/home/ubuntu/pub/docs/rag/frontend.md` | Current frontend architecture (v1 repo) |
| `/home/ubuntu/pub/app.js` | Current vanilla JS, study for behavior reference (v1 repo) |

## Non-Goals (Backend Agent Handles These)

- Go backend implementation
- SSE broadcaster implementation
- LLM streaming
- Tool execution
- Database schema
- Auth middleware
- Signal plane (polling)

The frontend agent builds against the API contract. The backend agent implements that contract in Go. They meet at the HTTP boundary.
