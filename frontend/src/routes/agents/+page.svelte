<script lang="ts">
	import { onMount } from 'svelte';
	import { getFleet } from '$lib/api/client';
	import { relativeTime } from '$lib/utils/time';
	import type { Agent } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
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
		busy: 'var(--cairn-accent)',
		offline: 'var(--text-tertiary)',
	};

	const statusVariant: Record<string, 'default' | 'secondary' | 'outline'> = {
		idle: 'default',
		busy: 'secondary',
		offline: 'outline',
	};
</script>

<div class="mx-auto max-w-5xl p-6">
	<h1 class="mb-6 text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Agents</h1>

	{#if loading}
		<div class="grid gap-3 md:grid-cols-2">
			{#each Array(4) as _, i}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 animate-in" style="animation-delay: {i * 50}ms">
					<div class="flex items-center gap-3 mb-3">
						<Skeleton class="h-9 w-9 rounded-lg" />
						<div class="flex-1">
							<Skeleton class="h-4 w-24 mb-1" />
							<Skeleton class="h-3 w-16" />
						</div>
					</div>
				</div>
			{/each}
		</div>
	{:else if agents.length === 0}
		<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
			<Bot class="mb-3 h-10 w-10 opacity-30" />
			<p class="text-sm">No agents registered</p>
			<p class="mt-1 text-xs opacity-60">Agents appear when connected via A2A or spawned by tasks</p>
		</div>
	{:else}
		<div class="grid gap-3 md:grid-cols-2">
			{#each agents as agent, i (agent.id)}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover animate-in" style="animation-delay: {i * 50}ms">
					<div class="mb-3 flex items-center gap-3">
						<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--bg-2)]">
							<Bot class="h-4 w-4 text-[var(--text-secondary)]" />
						</div>
						<div class="flex-1 min-w-0">
							<p class="text-sm font-medium text-[var(--text-primary)] truncate">{agent.name}</p>
							<p class="text-[11px] text-[var(--text-tertiary)]">{agent.type}</p>
						</div>
						<Badge variant={statusVariant[agent.status] ?? 'outline'} class="h-5 text-[10px] gap-1">
							<Circle class="h-1.5 w-1.5 fill-current" style="color: {statusColor[agent.status]}" />
							{agent.status}
						</Badge>
					</div>
					{#if agent.currentTask}
						<p class="text-xs text-[var(--text-secondary)] truncate">
							Working on: <span class="text-[var(--text-primary)]">{agent.currentTask}</span>
						</p>
					{/if}
					{#if agent.lastHeartbeat}
						<p class="mt-1 text-[10px] text-[var(--text-tertiary)] font-mono tabular-nums">
							Last seen {relativeTime(agent.lastHeartbeat)}
						</p>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
