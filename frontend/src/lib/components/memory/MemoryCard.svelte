<script lang="ts">
	import type { Memory } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
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

	const statusVariant: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
		proposed: 'outline',
		accepted: 'default',
		rejected: 'destructive',
	};

	const statusColor: Record<string, string> = {
		proposed: 'var(--color-warning)',
		accepted: 'var(--color-success)',
		rejected: 'var(--color-error)',
	};
</script>

<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover">
	<div class="mb-2 flex items-start justify-between">
		<div class="flex items-center gap-2">
			<span class="h-2 w-2 rounded-full" style="background: {statusColor[memory.status]}"></span>
			<Badge variant="secondary" class="h-4 px-1 text-[10px]">{memory.category}</Badge>
			<Badge variant={statusVariant[memory.status] ?? 'outline'} class="h-4 px-1 text-[10px]">{memory.status}</Badge>
		</div>
		<time class="text-[11px] text-[var(--text-tertiary)] tabular-nums font-mono" datetime={memory.createdAt}>
			{relativeTime(memory.createdAt)}
		</time>
	</div>
	<p class="text-sm text-[var(--text-primary)] leading-relaxed">{memory.content}</p>
	{#if memory.confidence != null}
		<div class="mt-2 flex items-center gap-2">
			<div class="h-1 flex-1 rounded-full bg-[var(--bg-3)] overflow-hidden">
				<div class="h-full rounded-full bg-[var(--cairn-accent)] transition-all" style="width: {memory.confidence * 100}%"></div>
			</div>
			<span class="text-[10px] text-[var(--text-tertiary)] font-mono tabular-nums">{Math.round(memory.confidence * 100)}%</span>
		</div>
	{/if}
	{#if memory.status === 'proposed' && onaccept && onreject}
		<div class="mt-3 flex gap-2">
			<Button
				variant="outline"
				size="sm"
				class="h-7 text-xs gap-1 border-[var(--color-success)]/30 text-[var(--color-success)] hover:bg-[var(--color-success)]/10"
				onclick={() => onaccept?.(memory.id)}
			>
				<Check class="h-3 w-3" /> Accept
			</Button>
			<Button
				variant="outline"
				size="sm"
				class="h-7 text-xs gap-1 border-[var(--color-error)]/30 text-[var(--color-error)] hover:bg-[var(--color-error)]/10"
				onclick={() => onreject?.(memory.id)}
			>
				<X class="h-3 w-3" /> Reject
			</Button>
		</div>
	{/if}
</div>
