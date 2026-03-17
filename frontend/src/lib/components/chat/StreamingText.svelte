<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';

	let { content = '', isStreaming = false }: { content: string; isStreaming: boolean } = $props();

	// Plan: 80ms debounce on re-render during streaming
	let renderedContent = $state('');
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		if (!isStreaming) {
			// Not streaming — render immediately
			renderedContent = renderMarkdown(content);
			return;
		}
		// Streaming — debounce at 80ms
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => {
			renderedContent = renderMarkdown(content);
		}, 80);
	});
</script>

<div class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)]">
	{@html renderedContent}
	{#if isStreaming}
		<span class="inline-block h-4 w-0.5 animate-pulse bg-[var(--pub-accent)]"></span>
	{/if}
</div>
