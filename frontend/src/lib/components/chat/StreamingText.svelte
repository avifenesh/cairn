<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';
	import { Copy, Check } from '@lucide/svelte';

	let { content = '', isStreaming = false }: { content: string; isStreaming: boolean } = $props();

	let renderedContent = $state('');
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let codeBlocks = $state<{ code: string; lang: string }[]>([]);
	let copiedIndex = $state<number | null>(null);

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

	$effect(() => {
		if (isStreaming) {
			codeBlocks = [];
			return;
		}
		const matches: { code: string; lang: string }[] = [];
		const pattern = /```(\w*)\n([\s\S]*?)```/g;
		let m;
		while ((m = pattern.exec(content)) !== null) {
			matches.push({ lang: m[1], code: m[2] });
		}
		codeBlocks = matches;
	});

	async function copyBlock(index: number) {
		await navigator.clipboard.writeText(codeBlocks[index].code);
		copiedIndex = index;
		setTimeout(() => { copiedIndex = null; }, 2000);
	}
</script>

<div class="streaming-text">
	<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">
		{@html renderedContent}
		{#if isStreaming}
			<span class="inline-block h-4 w-0.5 animate-pulse bg-[var(--cairn-accent)] ml-0.5 rounded-full"></span>
		{/if}
	</div>
</div>

{#if !isStreaming && codeBlocks.length > 0}
	<div class="mt-2 flex flex-wrap gap-1">
		{#each codeBlocks as block, i}
			<button
				class="inline-flex items-center gap-1 rounded-md border border-border-subtle bg-[var(--bg-2)] px-2 py-0.5 text-[10px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:border-border-default transition-colors"
				onclick={() => copyBlock(i)}
			>
				{#if copiedIndex === i}
					<Check class="h-2.5 w-2.5 text-[var(--color-success)]" />
					Copied
				{:else}
					<Copy class="h-2.5 w-2.5" />
					{block.lang || 'code'}
				{/if}
			</button>
		{/each}
	</div>
{/if}

<style>
	.cairn-prose :global(p) { margin: 0.5em 0; }
	.cairn-prose :global(p:first-child) { margin-top: 0; }
	.cairn-prose :global(p:last-child) { margin-bottom: 0; }
	.cairn-prose :global(code) {
		background: var(--bg-2);
		color: var(--cairn-accent);
		padding: 0.15em 0.4em;
		border-radius: 4px;
		font-size: 0.85em;
		font-family: 'Geist Mono', monospace;
	}
	.cairn-prose :global(pre) {
		background: var(--bg-2);
		border: 1px solid var(--border-subtle);
		border-radius: 8px;
		padding: 0.75em 1em;
		overflow-x: auto;
		margin: 0.75em 0;
	}
	.cairn-prose :global(pre code) {
		background: none;
		color: var(--text-primary);
		padding: 0;
		font-size: 0.8em;
	}
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
</style>
