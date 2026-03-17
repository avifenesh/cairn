<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';

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
	<div class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)]">
		{@html renderedContent}
		{#if isStreaming}
			<span class="inline-block h-4 w-0.5 animate-pulse bg-[var(--pub-accent)]"></span>
		{/if}
	</div>
</div>

{#if !isStreaming && codeBlocks.length > 0}
	<div class="mt-1 flex flex-wrap gap-1">
		{#each codeBlocks as block, i}
			<button
				class="rounded bg-[var(--bg-3)] px-1.5 py-0.5 text-[10px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors"
				onclick={() => copyBlock(i)}
			>
				{copiedIndex === i ? 'Copied' : `Copy${block.lang ? ' ' + block.lang : ''}`}
			</button>
		{/each}
	</div>
{/if}
