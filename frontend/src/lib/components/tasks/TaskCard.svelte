<script lang="ts">
	import type { Task } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { CheckCircle, XCircle, Clock, Ban, Loader2 } from '@lucide/svelte';

	let { task, oncancel }: { task: Task; oncancel?: (id: string) => void } = $props();

	const icons: Record<string, typeof CheckCircle> = {
		completed: CheckCircle,
		failed: XCircle,
		pending: Clock,
		running: Loader2,
		cancelled: Ban,
	};

	const colors: Record<string, string> = {
		completed: 'var(--color-success)',
		failed: 'var(--color-error)',
		pending: 'var(--color-warning)',
		running: 'var(--pub-accent)',
		cancelled: 'var(--text-tertiary)',
	};

	const Icon = $derived(icons[task.status] ?? Clock);
</script>

<div class="flex items-center gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
	<Icon class="h-4 w-4 flex-shrink-0" style="color: {colors[task.status]}" />
	<div class="min-w-0 flex-1">
		<p class="truncate text-sm text-[var(--text-primary)]">{task.title}</p>
		<p class="text-xs text-[var(--text-tertiary)]">
			{task.type} &middot; {task.status} &middot; {relativeTime(task.createdAt)}
		</p>
	</div>
	{#if oncancel && (task.status === 'running' || task.status === 'pending')}
		<button
			class="text-xs text-[var(--text-tertiary)] hover:text-[var(--color-error)] transition-colors"
			onclick={() => oncancel?.(task.id)}
		>
			Cancel
		</button>
	{/if}
</div>
