<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';

	let { content = '', isStreaming = false }: { content: string; isStreaming: boolean } = $props();

	let renderedContent = $state('');
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		if (!isStreaming) {
			renderedContent = renderMarkdown(content);
			return;
		}
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => {
			renderedContent = renderMarkdown(content);
		}, 80);
	});

	// Event delegation for code block copy buttons (no inline onclick)
	function handleClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (target.dataset.copy !== 'true') return;
		const block = target.closest('.cairn-code-block');
		const code = block?.querySelector('code')?.textContent ?? '';
		navigator.clipboard.writeText(code);
		target.textContent = 'Copied';
		setTimeout(() => { target.textContent = 'Copy'; }, 2000);
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="streaming-text" role="region" onclick={handleClick}>
	<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">
		{@html renderedContent}
		{#if isStreaming}
			<span class="inline-block h-4 w-0.5 animate-pulse bg-[var(--cairn-accent)] ml-0.5 rounded-full"></span>
		{/if}
	</div>
</div>

<style>
	/* --- Prose base --- */
	.cairn-prose :global(p) { margin: 0.5em 0; }
	.cairn-prose :global(p:first-child) { margin-top: 0; }
	.cairn-prose :global(p:last-child) { margin-bottom: 0; }
	.cairn-prose :global(strong) { color: var(--text-primary); font-weight: 600; }
	.cairn-prose :global(em) { color: var(--text-secondary); }
	.cairn-prose :global(ul), .cairn-prose :global(ol) { padding-left: 1.25em; margin: 0.5em 0; }
	.cairn-prose :global(li) { margin: 0.2em 0; }
	.cairn-prose :global(blockquote) {
		border-left: 2px solid var(--cairn-accent);
		padding-left: 0.75em;
		color: var(--text-secondary);
		margin: 0.75em 0;
	}
	.cairn-prose :global(a) {
		color: var(--cairn-accent);
		text-decoration: underline;
		text-underline-offset: 2px;
	}
	.cairn-prose :global(h1), .cairn-prose :global(h2), .cairn-prose :global(h3) {
		font-weight: 600;
		margin: 0.75em 0 0.25em;
		color: var(--text-primary);
	}
	.cairn-prose :global(h1) { font-size: 1.1em; }
	.cairn-prose :global(h2) { font-size: 1em; }
	.cairn-prose :global(h3) { font-size: 0.95em; }
	.cairn-prose :global(hr) {
		border: none;
		border-top: 1px solid var(--border-subtle);
		margin: 1em 0;
	}
	.cairn-prose :global(table) {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85em;
		margin: 0.75em 0;
	}
	.cairn-prose :global(th), .cairn-prose :global(td) {
		border: 1px solid var(--border-subtle);
		padding: 0.4em 0.6em;
		text-align: left;
	}
	.cairn-prose :global(th) { background: var(--bg-2); font-weight: 500; }

	/* --- Inline code --- */
	.cairn-prose :global(code) {
		background: var(--bg-2);
		color: var(--cairn-accent);
		padding: 0.15em 0.4em;
		border-radius: 4px;
		font-size: 0.85em;
		font-family: 'Geist Mono', monospace;
	}

	/* --- Code blocks with header bar --- */
	.cairn-prose :global(.cairn-code-block) {
		border: 1px solid var(--border-subtle);
		border-radius: 8px;
		overflow: hidden;
		margin: 0.75em 0;
	}
	.cairn-prose :global(.cairn-code-header) {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.35em 0.75em;
		background: var(--bg-3);
		border-bottom: 1px solid var(--border-subtle);
		font-size: 0.7em;
	}
	.cairn-prose :global(.cairn-code-lang) {
		color: var(--text-tertiary);
		font-family: 'Geist Mono', monospace;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.cairn-prose :global(.cairn-code-copy) {
		color: var(--text-tertiary);
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.2em 0.5em;
		border-radius: 4px;
		font-size: 1em;
		font-family: inherit;
		transition: color 0.15s, background 0.15s;
	}
	.cairn-prose :global(.cairn-code-copy:hover) {
		color: var(--text-primary);
		background: var(--bg-4);
	}
	.cairn-prose :global(.cairn-code-block pre) {
		margin: 0;
		border: none;
		border-radius: 0;
		background: var(--bg-2);
		padding: 0.75em 1em;
		overflow-x: auto;
	}
	.cairn-prose :global(.cairn-code-block pre code) {
		background: none;
		color: var(--text-primary);
		padding: 0;
		font-size: 0.8em;
	}
	/* Fallback pre without header */
	.cairn-prose :global(pre:not(.cairn-code-block pre)) {
		background: var(--bg-2);
		border: 1px solid var(--border-subtle);
		border-radius: 8px;
		padding: 0.75em 1em;
		overflow-x: auto;
		margin: 0.75em 0;
	}
	.cairn-prose :global(pre:not(.cairn-code-block pre) code) {
		background: none;
		color: var(--text-primary);
		padding: 0;
		font-size: 0.8em;
	}

	/* --- Syntax highlighting --- */
	.cairn-prose :global(.hl-keyword) { color: #C084FC; } /* purple-400 */
	.cairn-prose :global(.hl-string) { color: #34D399; } /* emerald-400 */
	.cairn-prose :global(.hl-comment) { color: var(--text-tertiary); font-style: italic; }
	.cairn-prose :global(.hl-number) { color: #FB923C; } /* orange-400 */
</style>
