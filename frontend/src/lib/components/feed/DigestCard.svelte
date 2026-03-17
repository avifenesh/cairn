<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';
	import { FileText } from '@lucide/svelte';

	let { title = 'Daily Digest', content }: { title?: string; content: string } = $props();
</script>

<div class="rounded-lg border border-[var(--pub-accent)]/20 bg-[var(--accent-dim)] p-4">
	<div class="mb-2 flex items-center gap-2 text-[var(--pub-accent)]">
		<FileText class="h-4 w-4" />
		<span class="text-sm font-medium">{title}</span>
	</div>
	<div class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)]">
		{@html renderMarkdown(content)}
	</div>
</div>
