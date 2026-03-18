<script lang="ts">
	import type { Task } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { Badge } from '$lib/components/ui/badge';

	let { tasks }: { tasks: Task[] } = $props();

	const sorted = $derived([...tasks].sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()));

	const statusColors: Record<string, string> = {
		completed: 'var(--color-success)',
		failed: 'var(--color-error)',
		pending: 'var(--color-warning)',
		running: 'var(--cairn-accent)',
		cancelled: 'var(--text-tertiary)',
	};
</script>

<div class="relative pl-4">
	<div class="absolute left-1.5 top-0 bottom-0 w-px bg-border-subtle"></div>
	{#each sorted as task, i (task.id)}
		<div class="relative mb-3 pl-4 animate-in" style="animation-delay: {i * 30}ms">
			<span
				class="absolute -left-[5px] top-1.5 h-2.5 w-2.5 rounded-full border-2 border-[var(--bg-0)]
					{task.status === 'running' ? 'animate-pulse-dot' : ''}"
				style="background: {statusColors[task.status] ?? 'var(--text-tertiary)'}"
			></span>
			<div class="flex items-center gap-2">
				<p class="text-sm text-[var(--text-primary)] truncate">{task.title}</p>
				<Badge variant="outline" class="h-4 px-1 text-[10px] flex-shrink-0">{task.status}</Badge>
			</div>
			<time class="text-[11px] text-[var(--text-tertiary)] font-mono tabular-nums" datetime={task.updatedAt}>
				{relativeTime(task.updatedAt)}
			</time>
		</div>
	{/each}
</div>
