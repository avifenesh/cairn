<script lang="ts">
	import { onMount } from 'svelte';
	import { getFleet, getAgentTypes, type AgentTypeItem } from '$lib/api/client';
	import { relativeTime } from '$lib/utils/time';
	import type { Agent } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Bot, Circle, Cpu, Code, Search, Eye, Wrench, FileText, Database, Layout, Server, PenTool, Compass, Shield } from '@lucide/svelte';

	let agents = $state<Agent[]>([]);
	let agentTypes = $state<AgentTypeItem[]>([]);
	let loading = $state(true);
	let tab = $state<'types' | 'fleet'>('types');

	onMount(async () => {
		try {
			const [fleet, types] = await Promise.all([
				getFleet().catch(() => ({ agents: [] })),
				getAgentTypes().catch(() => []),
			]);
			agents = fleet.agents;
			agentTypes = types;
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

	const modeColors: Record<string, string> = {
		talk: 'var(--color-info)',
		work: 'var(--color-warning)',
		coding: 'var(--color-success)',
	};

	const typeIcons: Record<string, typeof Bot> = {
		researcher: Search,
		coder: Code,
		reviewer: Shield,
		executor: Wrench,
		planner: PenTool,
		explorer: Compass,
		frontend: Layout,
		backend: Server,
		database: Database,
		architect: Cpu,
		observer: Eye,
		fixer: Wrench,
		'docs-writer': FileText,
	};
</script>

<div class="mx-auto max-w-5xl px-4 py-4 sm:p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Agents</h1>
		<div class="flex gap-1 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-0.5">
			<button
				class="rounded-md px-3 py-1 text-xs font-medium transition-colors {tab === 'types' ? 'bg-[var(--bg-2)] text-[var(--text-primary)]' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
				onclick={() => tab = 'types'}
			>
				Types ({agentTypes.length})
			</button>
			<button
				class="rounded-md px-3 py-1 text-xs font-medium transition-colors {tab === 'fleet' ? 'bg-[var(--bg-2)] text-[var(--text-primary)]' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
				onclick={() => tab = 'fleet'}
			>
				Fleet ({agents.length})
			</button>
		</div>
	</div>

	{#if loading}
		<div class="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _, i}
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

	{:else if tab === 'types'}
		{#if agentTypes.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
				<Cpu class="mb-3 h-10 w-10 opacity-30" />
				<p class="text-sm">No agent types discovered</p>
				<p class="mt-1 text-xs opacity-60">Add AGENT.md files to agents/ or ~/.cairn/agents/</p>
			</div>
		{:else}
			<div class="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
				{#each agentTypes as at, i (at.name)}
					{@const IconComponent = typeIcons[at.name] ?? Bot}
					<a
						href="/agents/{at.name}"
						class="group rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover animate-in block"
						style="animation-delay: {i * 40}ms"
					>
						<div class="mb-3 flex items-center gap-3">
							<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--bg-2)] group-hover:bg-[var(--bg-3)] transition-colors">
								<IconComponent class="h-4 w-4 text-[var(--text-secondary)]" />
							</div>
							<div class="flex-1 min-w-0">
								<p class="text-sm font-medium text-[var(--text-primary)] truncate">{at.name}</p>
								<div class="flex items-center gap-1.5 mt-0.5">
									<Badge variant="outline" class="h-4 text-[9px] px-1.5" style="border-color: {modeColors[at.mode] ?? 'var(--border-subtle)'}; color: {modeColors[at.mode] ?? 'var(--text-tertiary)'}">
										{at.mode}
									</Badge>
									<span class="text-[10px] text-[var(--text-tertiary)] tabular-nums">{at.maxRounds}r</span>
									{#if at.worktree}
										<span class="text-[10px] text-[var(--text-tertiary)]">worktree</span>
									{/if}
								</div>
							</div>
						</div>
						<p class="text-xs text-[var(--text-secondary)] line-clamp-2">{at.description}</p>
					</a>
				{/each}
			</div>
		{/if}

	{:else}
		<!-- Fleet tab (existing A2A agents) -->
		{#if agents.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
				<Bot class="mb-3 h-10 w-10 opacity-30" />
				<p class="text-sm">No agents connected</p>
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
	{/if}
</div>
