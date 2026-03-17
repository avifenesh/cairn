<script lang="ts">
	import { onMount } from 'svelte';
	import { getFleet } from '$lib/api/client';
	import { relativeTime } from '$lib/utils/time';
	import type { Agent } from '$lib/types';
	import { Bot, Circle } from '@lucide/svelte';

	let agents = $state<Agent[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await getFleet();
			agents = res.agents;
		} catch {
			// handled
		} finally {
			loading = false;
		}
	});

	const statusColor: Record<string, string> = {
		idle: 'var(--color-success)',
		busy: 'var(--pub-accent)',
		offline: 'var(--text-tertiary)',
	};
</script>

<div class="mx-auto max-w-4xl p-6">
	<h1 class="mb-6 text-2xl font-semibold text-[var(--text-primary)]">Agents</h1>

	{#if loading}
		<div class="grid gap-4 md:grid-cols-2">
			{#each Array(4) as _}
				<div class="h-32 animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
			{/each}
		</div>
	{:else if agents.length === 0}
		<div class="flex flex-col items-center justify-center py-16 text-[var(--text-tertiary)]">
			<Bot class="mb-3 h-10 w-10 opacity-40" />
			<p class="text-sm">No agents registered</p>
		</div>
	{:else}
		<div class="grid gap-4 md:grid-cols-2">
			{#each agents as agent (agent.id)}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
					<div class="mb-3 flex items-center gap-3">
						<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--bg-3)]">
							<Bot class="h-5 w-5 text-[var(--text-secondary)]" />
						</div>
						<div class="flex-1">
							<p class="text-sm font-medium text-[var(--text-primary)]">{agent.name}</p>
							<p class="text-xs text-[var(--text-tertiary)]">{agent.type}</p>
						</div>
						<span class="flex items-center gap-1.5 text-xs">
							<Circle class="h-2 w-2 fill-current" style="color: {statusColor[agent.status]}" />
							{agent.status}
						</span>
					</div>
					{#if agent.currentTask}
						<p class="text-xs text-[var(--text-secondary)]">
							Working on: {agent.currentTask}
						</p>
					{/if}
					{#if agent.lastHeartbeat}
						<p class="mt-1 text-[10px] text-[var(--text-tertiary)]">
							Last seen {relativeTime(agent.lastHeartbeat)}
						</p>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
