# Piece 10: Frontend (Svelte 5 Dashboard)

> Real-time agent dashboard with chat streaming, task boards, memory management, and feed views.
> Embedded in Go binary via `embed.FS`. No separate server.

## Why Svelte 5

| Factor | Decision |
|--------|----------|
| **Reactivity** | Runes ($state, $derived, $effect) — compile-time fine-grained, no VDOM |
| **Streaming** | $effect + EventSource + direct mutation = most natural SSE pattern |
| **Size** | 15-20KB compiled output |
| **Embedding** | Vite build → dist/ → Go embed.FS. No Node server needed |
| **Components** | shadcn-svelte (Svelte 5 supported) + Tailwind |
| **TypeScript** | First-class, compile-time checked |
| **DX** | Highest satisfaction scores, explicit without verbose |

## Layout: Three-Zone Architecture

```
┌──────────────────────────────────────────────────────┐
│  Header: Agent Health ● | Cmd+K | Theme | Budget     │
├────────────┬─────────────────────────┬───────────────┤
│            │                         │               │
│  Sidebar   │     Main Content        │   Context     │
│            │                         │   Panel       │
│  Navigation│  (switches by route)    │               │
│  • Today   │  - Today: Dashboard     │  • Tool calls │
│  • Ops     │  - Ops: Task triage     │  • Artifacts  │
│  • Chat    │  - Chat: Conversation   │  • Sources    │
│  • Memory  │  - Memory: RAG browser  │  • Reasoning  │
│  • Agents  │  - Feed: Signal stream  │               │
│  • Skills  │  - Soul: Editor         │  (collapses   │
│  • Soul    │  - Settings             │   on mobile)  │
│            │                         │               │
├────────────┴─────────────────────────┴───────────────┤
│  Status Bar: SSE ● | 6 agents | Polled 2m ago        │
└──────────────────────────────────────────────────────┘

Mobile (< 768px):
┌──────────────────────┐
│ Header               │
├──────────────────────┤
│                      │
│   Main Content       │
│   (full width)       │
│                      │
├──────────────────────┤
│ Today|Ops|Chat|More  │  ← Bottom nav (4 items + overflow)
└──────────────────────┘
```

## Component Architecture

```
src/
├── lib/
│   ├── stores/
│   │   ├── app.svelte.ts       # Global reactive state
│   │   ├── chat.svelte.ts      # Chat messages, streaming, sessions
│   │   ├── tasks.svelte.ts     # Task queue, approvals
│   │   ├── feed.svelte.ts      # Signal feed, unread counts
│   │   ├── memory.svelte.ts    # Memories, search results
│   │   └── sse.svelte.ts       # SSE connection, reconnect logic
│   ├── components/
│   │   ├── chat/
│   │   │   ├── ChatPanel.svelte
│   │   │   ├── MessageBubble.svelte
│   │   │   ├── StreamingText.svelte     # Markdown + cursor
│   │   │   ├── ToolCallChip.svelte
│   │   │   ├── ReasoningBlock.svelte
│   │   │   ├── ModeSelector.svelte
│   │   │   ├── SessionPicker.svelte
│   │   │   └── VoiceButton.svelte
│   │   ├── tasks/
│   │   │   ├── TaskCard.svelte
│   │   │   ├── ApprovalCard.svelte
│   │   │   └── TaskTimeline.svelte
│   │   ├── feed/
│   │   │   ├── FeedItem.svelte
│   │   │   └── DigestCard.svelte
│   │   ├── memory/
│   │   │   ├── MemoryCard.svelte
│   │   │   ├── MemorySearch.svelte
│   │   │   └── MemoryEditor.svelte
│   │   ├── layout/
│   │   │   ├── Sidebar.svelte
│   │   │   ├── Header.svelte
│   │   │   ├── StatusBar.svelte
│   │   │   ├── BottomNav.svelte        # Mobile
│   │   │   └── CommandPalette.svelte   # Cmd+K
│   │   └── shared/
│   │       ├── Button.svelte
│   │       ├── Badge.svelte
│   │       ├── Toast.svelte
│   │       ├── Modal.svelte
│   │       └── CodeBlock.svelte
│   ├── api/
│   │   └── client.ts           # REST + SSE client
│   └── utils/
│       ├── markdown.ts         # marked.js wrapper
│       └── time.ts             # Relative time formatting
├── routes/
│   ├── +layout.svelte          # Shell layout
│   ├── today/+page.svelte
│   ├── ops/+page.svelte
│   ├── chat/+page.svelte
│   ├── memory/+page.svelte
│   ├── agents/+page.svelte
│   ├── skills/+page.svelte
│   ├── soul/+page.svelte
│   └── settings/+page.svelte
└── app.html
```

## SSE Streaming (the core pattern)

```svelte
<!-- src/lib/stores/sse.svelte.ts -->
<script context="module" lang="ts">
  let connected = $state(false);
  let reconnectAttempt = $state(0);

  export function connect(baseUrl: string, token: string) {
    const url = `${baseUrl}/v1/stream?token=${encodeURIComponent(token)}`;
    const source = new EventSource(url, { withCredentials: true });

    source.onopen = () => { connected = true; reconnectAttempt = 0; };
    source.onerror = () => { connected = false; reconnectAttempt++; };

    // Route events to stores
    source.addEventListener('assistant_delta', (e) => {
      const data = JSON.parse(e.data);
      chat.appendDelta(data.taskId, data.deltaText);
    });
    source.addEventListener('assistant_end', (e) => {
      const data = JSON.parse(e.data);
      chat.completeMessage(data.taskId, data.messageText);
    });
    source.addEventListener('assistant_reasoning', (e) => {
      const data = JSON.parse(e.data);
      chat.appendReasoning(data.taskId, data.round, data.thought);
    });
    source.addEventListener('task_update', (e) => {
      const data = JSON.parse(e.data);
      tasks.update(data);
    });
    // ... more event handlers

    return source;
  }
</script>
```

```svelte
<!-- src/lib/components/chat/StreamingText.svelte -->
<script lang="ts">
  import { marked } from 'marked';

  let { content = '', isStreaming = false }: { content: string; isStreaming: boolean } = $props();
  let rendered = $derived(marked.parse(content, { breaks: true, gfm: true }));
</script>

<div class="prose prose-sm dark:prose-invert">
  {@html rendered}
  {#if isStreaming}
    <span class="animate-pulse inline-block w-0.5 h-4 bg-accent ml-0.5"></span>
  {/if}
</div>
```

## Design System

| Element | Choice | Reason |
|---------|--------|--------|
| **Components** | shadcn-svelte | Copy-paste, Tailwind-based, accessible |
| **Styling** | Tailwind CSS v4 | Utility-first, design tokens via CSS vars |
| **Icons** | Lucide | Used by shadcn, tree-shakeable SVGs |
| **Fonts** | Inter (UI) + JetBrains Mono (code) | Clean, widely available |
| **Dark mode** | CSS `prefers-color-scheme` + manual toggle | `[data-theme]` attribute on root |
| **Animations** | CSS transitions + Svelte `transition:` | No JS animation library needed |

## Key UI Patterns

1. **Cmd+K command palette** — fuzzy search to any view, session, memory, or action
2. **Keyboard triage** — `a`=approve, `d`=deny, `j/k`=navigate, `o`=open, `?`=help
3. **Ambient health dot** — green/yellow/red in header, no distraction
4. **Toast for disconnects** — after 30s SSE drop, write queue with 50-entry cap
5. **Skeleton loading** — for every async panel
6. **Optimistic mutations** — approve/deny instant, server confirms async
7. **Responsive breakpoints** — 1280px (desktop), 768px (tablet), 375px (mobile)

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 10.1 | Svelte + Vite + Tailwind scaffold | Nothing |
| 10.2 | SSE store + API client | 10.1, backend SSE |
| 10.3 | Layout shell (sidebar, header, bottom nav, routes) | 10.1 |
| 10.4 | Chat panel (streaming, markdown, tool chips, reasoning) | 10.2, 10.3 |
| 10.5 | Today dashboard (stats, activity, quick actions) | 10.2, 10.3 |
| 10.6 | Ops view (task cards, approvals, triage) | 10.2, 10.3 |
| 10.7 | Memory view (search, cards, edit, propose) | 10.2, 10.3 |
| 10.8 | Feed, Agents, Skills, Soul, Settings views | 10.2, 10.3 |
| 10.9 | Command palette (Cmd+K) | 10.3 |
| 10.10 | Unit tests (vitest): stores, utilities, API client | 10.2-10.9 |
| 10.11 | Mobile: pull-to-refresh, swipe-to-dismiss | 10.3-10.9 |
| 10.12 | Embed in Go binary (embed.FS) | 10.1, backend |

## Status

All subphases 10.1-10.11 complete. 84 tests passing. 10.12 (Go embed) depends on backend server (Phase 4a).

## Phase 6: Hardening

| # | Item | Status |
|---|------|--------|
| 6.1 | Component tests (ErrorBoundary, FeedItem, ApprovalCard, ToolCallChip) | Done |
| 6.2 | Error boundaries (shared component, replace bare catches) | Done |
| 6.3 | Consistent loading (shared SkeletonList) | Done |
| 6.4 | Feed pagination (infinite scroll / load more) | Done |
| 6.5 | Session persistence (localStorage + restore) | Done |
| 6.6 | Accessibility (semantic elements, tab order, a11y fixes) | Done |
| 6.7 | Auto-mood timer (setInterval + Settings toggle) | Done |
| 6.8 | Settings notification preferences | Done |
| 6.9 | Debounced memory search (300ms) | Done |
| 6.10 | Type-safe SSE parsing (try/catch all handlers) | Done |
