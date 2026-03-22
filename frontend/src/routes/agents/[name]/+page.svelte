<script lang="ts">
	import { onMount } from 'svelte';
	import { getAgentType, deleteAgentType, type AgentTypeDetail } from '$lib/api/client';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { ArrowLeft, Trash2 } from '@lucide/svelte';

	let { data } = $props<{ data: { name: string } }>();
	let agentType = $state<AgentTypeDetail | null>(null);
	let loading = $state(true);
	let error = $state('');
	let deleting = $state(false);

	onMount(async () => {
		try {
			agentType = await getAgentType(data.name);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load agent type';
		} finally {
			loading = false;
		}
	});

	const modeColors: Record<string, string> = {
		talk: 'var(--color-info)',
		work: 'var(--color-warning)',
		coding: 'var(--color-success)',
	};

	async function handleDelete() {
		if (!agentType || !confirm(`Delete agent type "${agentType.name}"? This cannot be undone.`)) return;
		deleting = true;
		try {
			await deleteAgentType(agentType.name);
			window.location.href = '/agents';
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			deleting = false;
		}
	}
</script>

<div class="mx-auto max-w-4xl px-4 py-4 sm:p-6">
	<a href="/agents" class="mb-4 inline-flex items-center gap-1.5 text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] transition-colors">
		<ArrowLeft class="h-3 w-3" />
		Back to Agents
	</a>

	{#if loading}
		<div class="mt-4">
			<Skeleton class="h-8 w-48 mb-2" />
			<Skeleton class="h-4 w-96 mb-6" />
			<Skeleton class="h-64 w-full rounded-lg" />
		</div>
	{:else if error}
		<div class="mt-4 rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-sm text-red-400">
			{error}
		</div>
	{:else if agentType}
		<div class="mt-2">
			<!-- Header -->
			<div class="flex items-start justify-between mb-6">
				<div>
					<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">{agentType.name}</h1>
					<p class="mt-1 text-sm text-[var(--text-secondary)]">{agentType.description}</p>
					<div class="mt-2 flex items-center gap-2">
						<Badge variant="outline" class="text-[10px]" style="border-color: {modeColors[agentType.mode] ?? 'var(--border-subtle)'}; color: {modeColors[agentType.mode] ?? 'var(--text-tertiary)'}">
							{agentType.mode} mode
						</Badge>
						<Badge variant="outline" class="text-[10px]">{agentType.maxRounds} rounds</Badge>
						{#if agentType.worktree}
							<Badge variant="outline" class="text-[10px]">worktree</Badge>
						{/if}
						{#if agentType.model && agentType.model !== 'default'}
							<Badge variant="outline" class="text-[10px]">model: {agentType.model}</Badge>
						{/if}
					</div>
				</div>
				<button
					class="rounded-md p-2 text-[var(--text-tertiary)] hover:text-red-400 hover:bg-red-500/10 transition-colors"
					onclick={handleDelete}
					disabled={deleting}
					title="Delete agent type"
				>
					<Trash2 class="h-4 w-4" />
				</button>
			</div>

			<!-- Tool access -->
			{#if agentType.deniedTools && agentType.deniedTools.length > 0}
				<div class="mb-4 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
					<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1.5">Denied Tools</p>
					<div class="flex flex-wrap gap-1">
						{#each agentType.deniedTools as t}
							<span class="rounded bg-red-500/10 px-1.5 py-0.5 text-[10px] font-mono text-red-400">{t}</span>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Content / System Prompt -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-3">System Prompt</p>
				<div class="prose prose-sm prose-invert max-w-none text-[var(--text-secondary)]">
					{@html renderMarkdown(agentType.content)}
				</div>
			</div>
		</div>
	{/if}
</div>
