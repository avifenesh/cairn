<script lang="ts">
	import type { Memory } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Check, X, Pencil, Trash2 } from '@lucide/svelte';

	let {
		memory,
		onaccept,
		onreject,
		ondelete,
		onupdate,
	}: {
		memory: Memory;
		onaccept?: (id: string) => void;
		onreject?: (id: string) => void;
		ondelete?: (id: string) => void;
		onupdate?: (id: string, content: string) => void;
	} = $props();

	let editing = $state(false);
	let editContent = $state('');

	function startEdit() {
		editContent = memory.content;
		editing = true;
	}

	function cancelEdit() {
		editing = false;
	}

	function saveEdit() {
		const trimmed = editContent.trim();
		if (!trimmed || trimmed === memory.content) {
			editing = false;
			return;
		}
		onupdate?.(memory.id, trimmed);
		editing = false;
	}

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

<div class="group rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover">
	<div class="mb-2 flex items-start justify-between">
		<div class="flex items-center gap-2">
			<span class="h-2 w-2 rounded-full" style="background: {statusColor[memory.status]}"></span>
			<Badge variant="secondary" class="h-4 px-1 text-[10px]">{memory.category}</Badge>
			<Badge variant={statusVariant[memory.status] ?? 'outline'} class="h-4 px-1 text-[10px]">{memory.status}</Badge>
		</div>
		<div class="flex items-center gap-1">
			<div class="flex gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
				{#if onupdate}
					<button
						class="rounded p-1 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-2)]"
						title="Edit"
						onclick={startEdit}
					>
						<Pencil class="h-3 w-3" />
					</button>
				{/if}
				{#if ondelete}
					<button
						class="rounded p-1 text-[var(--text-tertiary)] hover:text-[var(--color-error)] hover:bg-[var(--color-error)]/10"
						title="Delete"
						onclick={() => ondelete?.(memory.id)}
					>
						<Trash2 class="h-3 w-3" />
					</button>
				{/if}
			</div>
			<time class="text-[11px] text-[var(--text-tertiary)] tabular-nums font-mono ml-1" datetime={memory.createdAt}>
				{relativeTime(memory.createdAt)}
			</time>
		</div>
	</div>
	{#if editing}
		<div class="space-y-2">
			<textarea
				bind:value={editContent}
				rows="3"
				class="w-full resize-none rounded-md border border-border-subtle bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none"
			></textarea>
			<div class="flex gap-1.5">
				<Button size="sm" class="h-6 text-[10px] px-2" onclick={saveEdit}>Save</Button>
				<Button variant="ghost" size="sm" class="h-6 text-[10px] px-2" onclick={cancelEdit}>Cancel</Button>
			</div>
		</div>
	{:else}
		<p class="text-sm text-[var(--text-primary)] leading-relaxed">{memory.content}</p>
	{/if}
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
