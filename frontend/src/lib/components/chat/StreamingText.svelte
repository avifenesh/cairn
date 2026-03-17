<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';

	let { content = '', isStreaming = false }: { content: string; isStreaming: boolean } = $props();

	// Plan: 80ms debounce on re-render during streaming
	let renderedContent = $state('');
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let contentEl: HTMLDivElement;

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

	// Plan: "Code blocks get copy buttons after stream ends"
	$effect(() => {
		if (isStreaming || !contentEl) return;
		const blocks = contentEl.querySelectorAll('pre code');
		blocks.forEach((block) => {
			const pre = block.parentElement;
			if (!pre || pre.querySelector('.copy-btn')) return;
			const btn = document.createElement('button');
			btn.className = 'copy-btn absolute right-2 top-2 rounded-md bg-[var(--bg-3)] p-1.5 text-[var(--text-tertiary)] opacity-0 transition-opacity hover:text-[var(--text-primary)]';
			btn.textContent = 'Copy';
			btn.style.fontSize = '10px';
			btn.onclick = async () => {
				await navigator.clipboard.writeText(block.textContent ?? '');
				btn.textContent = 'Copied';
				btn.style.color = 'var(--color-success)';
				setTimeout(() => {
					btn.textContent = 'Copy';
					btn.style.color = '';
				}, 2000);
			};
			pre.style.position = 'relative';
			pre.classList.add('group');
			pre.appendChild(btn);
		});
	});
</script>

<div
	bind:this={contentEl}
	class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)] [&_pre]:relative [&_.copy-btn]:group-hover:opacity-100"
>
	{@html renderedContent}
	{#if isStreaming}
		<span class="inline-block h-4 w-0.5 animate-pulse bg-[var(--pub-accent)]"></span>
	{/if}
</div>
