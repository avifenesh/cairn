<script lang="ts">
	import type { Approval } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { ShieldCheck, ShieldX } from '@lucide/svelte';

	let {
		approval,
		onapprove,
		ondeny,
		selected = false,
		onselect,
	}: {
		approval: Approval;
		onapprove: (id: string) => void;
		ondeny: (id: string) => void;
		selected?: boolean;
		onselect?: (id: string) => void;
	} = $props();
</script>

<div class="rounded-lg border border-[var(--color-warning)]/20 bg-[var(--bg-1)] p-4 card-hover">
	<div class="mb-3 flex items-start justify-between gap-3">
		<div class="flex items-start gap-2.5 min-w-0">
			{#if onselect}
				<input
					type="checkbox"
					checked={selected}
					onchange={() => onselect?.(approval.id)}
					class="mt-0.5 h-4 w-4 rounded border-border-default accent-[var(--cairn-accent)]"
				/>
			{/if}
			<div class="min-w-0">
				<div class="flex items-center gap-2">
					<p class="text-sm font-medium text-[var(--text-primary)] truncate">{approval.title}</p>
					<Badge variant="outline" class="h-4 px-1 text-[10px] border-[var(--color-warning)]/30 text-[var(--color-warning)] flex-shrink-0">
						{approval.type}
					</Badge>
				</div>
				{#if approval.description}
					<p class="mt-1 text-xs text-[var(--text-secondary)] line-clamp-2">{approval.description}</p>
				{/if}
			</div>
		</div>
		<time class="text-[11px] text-[var(--text-tertiary)] flex-shrink-0 tabular-nums" datetime={approval.createdAt}>
			{relativeTime(approval.createdAt)}
		</time>
	</div>
	<div class="flex gap-2">
		<Button
			variant="outline"
			size="sm"
			class="h-7 text-xs gap-1.5 border-[var(--color-success)]/30 text-[var(--color-success)] hover:bg-[var(--color-success)]/10"
			onclick={() => onapprove(approval.id)}
		>
			<ShieldCheck class="h-3 w-3" />
			Approve
		</Button>
		<Button
			variant="outline"
			size="sm"
			class="h-7 text-xs gap-1.5 border-[var(--color-error)]/30 text-[var(--color-error)] hover:bg-[var(--color-error)]/10"
			onclick={() => ondeny(approval.id)}
		>
			<ShieldX class="h-3 w-3" />
			Deny
		</Button>
	</div>
</div>
