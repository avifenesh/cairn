---
name: svelte5-dev
description: "Use for Svelte 5 and SvelteKit development tasks: components, runes, stores, routing, forms. Keywords: svelte, sveltekit, $state, $derived, $effect, $props, runes, component, route, +page, +layout, +server"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.readFile,cairn.editFile,cairn.searchFiles,cairn.writeFile"
---

# Svelte 5 + SvelteKit Development

## Svelte 5 Runes (NOT Svelte 4 stores)

This project uses Svelte 5 runes exclusively. Never use Svelte 4 patterns ($: reactive, stores with $ prefix, onMount callbacks for state).

### State Management
- `$state<T>(initial)` for reactive state
- `$derived(() => expr)` for computed values (always wrap in arrow function)
- `$effect(() => { ... })` for side effects
- `$props()` for component props (replaces `export let`)
- `$bindable()` for two-way binding props

### Reactive Stores (.svelte.ts files)
```typescript
// src/lib/stores/example.svelte.ts
let items = $state<Item[]>([]);
let loading = $state(false);
const filtered = $derived(() => items.filter(i => i.active));

export const store = {
  get items() { return items; },
  get loading() { return loading; },
  get filtered() { return filtered(); },
  setItems(v: Item[]) { items = v; },
};
```

### Component Patterns
- Use `{#snippet name()}...{/snippet}` for reusable markup (NOT slots)
- Use `{@render snippet()}` to render snippets
- Event handlers: `onclick`, `oninput` (NOT on:click, on:input)
- Use `{@const x = expr}` inside `{#each}` or `{#if}` blocks only

## SvelteKit Patterns
- File-based routing: `src/routes/path/+page.svelte`
- Load functions: `+page.ts` or `+page.server.ts`
- Layouts: `+layout.svelte`, `+layout.ts`
- API routes: `+server.ts` with GET, POST, PUT, DELETE exports
- Static adapter: `@sveltejs/adapter-static` (embedded in Go binary)

## Styling
- Tailwind CSS v4 (CSS-first config)
- Use `tailwind-variants` for component styling
- Dark theme via CSS custom properties (--bg-0, --bg-1, --text-primary, etc.)
- Component library: shadcn-svelte (bits-ui based)

## Commands
```bash
pnpm dev          # Dev server
pnpm build        # Static build to dist/
pnpm check        # Svelte + TypeScript check
pnpm test         # Vitest
```

## Key Conventions
- All stores use `.svelte.ts` extension (NOT `.ts`)
- API client at `$lib/api/client.ts` with get/post/put/del helpers
- Types at `$lib/types.ts`
- Components in `$lib/components/` organized by domain
- Use `$lib/` import alias, never relative paths from routes
