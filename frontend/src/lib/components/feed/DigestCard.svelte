<script lang="ts">
	import { renderMarkdown } from '$lib/utils/markdown';
	import { Badge } from '$lib/components/ui/badge';
	import { FileText } from '@lucide/svelte';

	let { title = 'Daily Digest', content }: { title?: string; content: string } = $props();
</script>

<div class="rounded-lg border border-[var(--cairn-accent)]/20 bg-[var(--accent-dim)] p-4 card-hover">
	<div class="mb-3 flex items-center gap-2">
		<div class="flex h-7 w-7 items-center justify-center rounded-md bg-[var(--cairn-accent)]/10">
			<FileText class="h-3.5 w-3.5 text-[var(--cairn-accent)]" />
		</div>
		<span class="text-sm font-medium text-[var(--cairn-accent)]">{title}</span>
		<Badge variant="outline" class="h-4 px-1 text-[10px] border-[var(--cairn-accent)]/30 text-[var(--cairn-accent)]">digest</Badge>
	</div>
	<div class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)] [&_code]:text-[var(--cairn-accent)] [&_code]:bg-[var(--bg-2)] [&_code]:px-1 [&_code]:rounded [&_code]:text-xs">
		{@html renderMarkdown(content)}
	</div>
</div>
