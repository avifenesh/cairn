<script lang="ts">
	import type { Memory } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { Check, X } from '@lucide/svelte';

	let {
		memory,
		onaccept,
		onreject,
	}: {
		memory: Memory;
		onaccept?: (id: string) => void;
		onreject?: (id: string) => void;
	} = $props();

	const statusColor: Record<string, string> = {
		proposed: 'var(--color-warning)',
		accepted: 'var(--color-success)',
		rejected: 'var(--color-error)',
	};
</script>

<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
	<div class="mb-2 flex items-start justify-between">
		<div class="flex items-center gap-2">
			<span class="h-2 w-2 rounded-full" style="background: {statusColor[memory.status]}"></span>
			<span class="text-xs text-[var(--text-tertiary)]">{memory.category}</span>
			<span class="text-xs text-[var(--text-tertiary)]">&middot;</span>
			<span class="text-xs text-[var(--text-tertiary)]">{memory.status}</span>
		</div>
		<span class="text-xs text-[var(--text-tertiary)]">
			{relativeTime(memory.createdAt)}
		</span>
	</div>
	<p class="text-sm text-[var(--text-primary)]">{memory.content}</p>
	{#if memory.status === 'proposed' && onaccept && onreject}
		<div class="mt-3 flex gap-2">
			<button
				class="flex items-center gap-1 rounded-md bg-[var(--color-success)]/10 px-3 py-1 text-xs font-medium text-[var(--color-success)] hover:bg-[var(--color-success)]/20 transition-colors"
				onclick={() => onaccept?.(memory.id)}
			>
				<Check class="h-3 w-3" /> Accept
			</button>
			<button
				class="flex items-center gap-1 rounded-md bg-[var(--color-error)]/10 px-3 py-1 text-xs font-medium text-[var(--color-error)] hover:bg-[var(--color-error)]/20 transition-colors"
				onclick={() => onreject?.(memory.id)}
			>
				<X class="h-3 w-3" /> Reject
			</button>
		</div>
	{/if}
</div>
