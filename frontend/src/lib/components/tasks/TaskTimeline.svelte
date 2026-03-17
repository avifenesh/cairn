<script lang="ts">
	import type { Task } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';

	let { tasks }: { tasks: Task[] } = $props();

	const sorted = $derived([...tasks].sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()));
</script>

<div class="relative pl-4">
	<div class="absolute left-1.5 top-0 bottom-0 w-px bg-border-subtle"></div>
	{#each sorted as task (task.id)}
		<div class="relative mb-3 pl-4">
			<span
				class="absolute -left-[5px] top-1.5 h-2.5 w-2.5 rounded-full border-2 border-[var(--bg-1)]"
				class:bg-[var(--color-success)]={task.status === 'completed'}
				class:bg-[var(--color-error)]={task.status === 'failed'}
				class:bg-[var(--color-warning)]={task.status === 'pending'}
				class:bg-[var(--pub-accent)]={task.status === 'running'}
				class:bg-[var(--text-tertiary)]={task.status === 'cancelled'}
			></span>
			<p class="text-sm text-[var(--text-primary)]">{task.title}</p>
			<p class="text-xs text-[var(--text-tertiary)]">
				{task.status} &middot; {relativeTime(task.updatedAt)}
			</p>
		</div>
	{/each}
</div>
