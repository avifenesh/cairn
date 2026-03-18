<script lang="ts">
	import type { Task } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { CheckCircle, XCircle, Clock, Ban, Loader2 } from '@lucide/svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';

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
		running: 'var(--cairn-accent)',
		cancelled: 'var(--text-tertiary)',
	};

	const statusVariant: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
		completed: 'default',
		failed: 'destructive',
		pending: 'outline',
		running: 'secondary',
		cancelled: 'outline',
	};

	const Icon = $derived(icons[task.status] ?? Clock);
</script>

<div class="flex items-center gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 card-hover">
	<div class="flex items-center justify-center h-8 w-8 rounded-md bg-[var(--bg-2)] flex-shrink-0">
		<Icon
			class="h-4 w-4 {task.status === 'running' ? 'animate-spin' : ''}"
			style="color: {colors[task.status]}"
		/>
	</div>
	<div class="min-w-0 flex-1">
		<p class="truncate text-sm font-medium text-[var(--text-primary)]">{task.title}</p>
		<div class="mt-1 flex items-center gap-1.5">
			<Badge variant={statusVariant[task.status] ?? 'outline'} class="h-4 px-1 text-[10px]">
				{task.status}
			</Badge>
			{#if task.type === 'a2a'}
				<Badge variant="outline" class="h-4 px-1 text-[10px] text-[var(--cairn-accent)]">a2a</Badge>
			{/if}
			<span class="text-[11px] text-[var(--text-tertiary)]">{task.type}</span>
			<span class="text-[11px] text-[var(--text-tertiary)]">&middot;</span>
			<time class="text-[11px] text-[var(--text-tertiary)]" datetime={task.createdAt}>{relativeTime(task.createdAt)}</time>
		</div>
		{#if task.progress != null}
			<div class="mt-2 h-1 w-full rounded-full bg-[var(--bg-3)] overflow-hidden">
				<div class="h-full rounded-full bg-[var(--cairn-accent)] transition-all" style="width: {task.progress}%"></div>
			</div>
		{/if}
	</div>
	{#if oncancel && (task.status === 'running' || task.status === 'pending')}
		<Button
			variant="ghost"
			size="sm"
			class="h-7 text-xs text-[var(--text-tertiary)] hover:text-[var(--color-error)]"
			onclick={() => oncancel?.(task.id)}
		>
			Cancel
		</Button>
	{/if}
</div>
