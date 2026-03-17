<script lang="ts">
	import type { Approval } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';

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

<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
	<div class="mb-2 flex items-start justify-between">
		<div class="flex items-start gap-2">
			{#if onselect}
				<input
					type="checkbox"
					checked={selected}
					onchange={() => onselect?.(approval.id)}
					class="mt-0.5 h-4 w-4 rounded border-border-default accent-[var(--pub-accent)]"
				/>
			{/if}
			<div>
				<p class="text-sm font-medium text-[var(--text-primary)]">{approval.title}</p>
				{#if approval.description}
					<p class="mt-1 text-xs text-[var(--text-secondary)]">{approval.description}</p>
				{/if}
			</div>
		</div>
		<span class="text-xs text-[var(--text-tertiary)]">
			{relativeTime(approval.createdAt)}
		</span>
	</div>
	<div class="flex gap-2">
		<button
			class="rounded-md bg-[var(--color-success)]/10 px-3 py-1.5 text-xs font-medium text-[var(--color-success)] hover:bg-[var(--color-success)]/20 transition-colors"
			onclick={() => onapprove(approval.id)}
		>
			Approve
		</button>
		<button
			class="rounded-md bg-[var(--color-error)]/10 px-3 py-1.5 text-xs font-medium text-[var(--color-error)] hover:bg-[var(--color-error)]/20 transition-colors"
			onclick={() => ondeny(approval.id)}
		>
			Deny
		</button>
	</div>
</div>
