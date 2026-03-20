<script lang="ts">
	import { onMount } from 'svelte';
	import { getAgentActivity } from '$lib/api/client';
	import { activityStore } from '$lib/stores/activity.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Brain, AlertTriangle, Clock, Zap, RefreshCw, ChevronDown, ChevronUp, Loader2, Wrench, BarChart3 } from '@lucide/svelte';

	let expandedId = $state<string | null>(null);
	let filterType = $state<string>('all');

	const TYPE_CONFIG: Record<string, { color: string; label: string }> = {
		task: { color: 'var(--cairn-accent)', label: 'Task' },
		idle: { color: 'var(--text-tertiary)', label: 'Idle' },
		reflection: { color: '#818CF8', label: 'Reflect' },
		cron: { color: '#F59E0B', label: 'Cron' },
		error: { color: 'var(--color-error)', label: 'Error' },
	};

	const TYPES = ['all', 'task', 'idle', 'reflection', 'cron', 'error'];

	onMount(async () => {
		try {
			const res = await getAgentActivity({ limit: 100 });
			activityStore.setEntries(res.items ?? []);
			if (res.stats) activityStore.setToolStats(res.stats);
		} catch (e) {
			console.error('Failed to load activity:', e);
		} finally {
			activityStore.setLoading(false);
		}
	});

	async function refresh() {
		activityStore.setLoading(true);
		try {
			const res = await getAgentActivity({ limit: 100, type: filterType === 'all' ? undefined : filterType });
			activityStore.setEntries(res.items ?? []);
			if (res.stats) activityStore.setToolStats(res.stats);
		} catch (e) {
			console.error('Failed to refresh activity:', e);
		} finally {
			activityStore.setLoading(false);
		}
	}

	const filteredEntries = $derived(
		filterType === 'all'
			? activityStore.entries
			: activityStore.entries.filter((e) => e.type === filterType)
	);
</script>

<div class="mx-auto max-w-5xl p-6 overflow-y-auto h-full">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Activity</h1>
			<p class="text-xs text-[var(--text-tertiary)] mt-0.5">What cairn is doing — idle reasoning, tasks, reflections, errors</p>
		</div>
		{#if activityStore.errorCount > 0}
			<Badge variant="outline" class="text-[var(--color-error)] border-[var(--color-error)]/30 gap-1">
				<AlertTriangle class="h-3 w-3" /> {activityStore.errorCount} errors
			</Badge>
		{/if}
	</div>

	<!-- Tool Stats -->
	{#if activityStore.toolStats}
		{@const stats = activityStore.toolStats}
		<div class="mb-6 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center gap-2 mb-3">
				<BarChart3 class="h-4 w-4 text-[var(--cairn-accent)]" />
				<span class="text-sm font-medium text-[var(--text-primary)]">Tool Stats</span>
				<span class="text-[11px] text-[var(--text-tertiary)] tabular-nums ml-auto">
					{stats.totalCalls} calls · {stats.totalErrors} errors
				</span>
			</div>
			{#if stats.tools && stats.tools.length > 0}
				<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
					{#each stats.tools.slice(0, 12) as tool}
						<div class="flex items-center gap-2 rounded-md bg-[var(--bg-0)] px-2.5 py-1.5">
							<Wrench class="h-3 w-3 text-[var(--text-tertiary)] flex-shrink-0" />
							<span class="text-[10px] text-[var(--text-primary)] truncate flex-1">{tool.toolName.replace('cairn.', '')}</span>
							<span class="text-[10px] tabular-nums text-[var(--text-tertiary)]">{tool.calls}</span>
							{#if tool.errors > 0}
								<span class="text-[10px] tabular-nums text-[var(--color-error)]">{tool.errors}err</span>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-[10px] text-[var(--text-tertiary)]">No tool executions recorded yet</p>
			{/if}
		</div>
	{/if}

	<!-- Filter chips -->
	<div class="mb-4 flex items-center gap-2">
		{#each TYPES as t}
			<button
				class="rounded-full px-2.5 py-0.5 text-[11px] font-medium transition-colors
					{filterType === t
						? 'bg-[var(--cairn-accent)] text-white'
						: 'bg-[var(--bg-2)] text-[var(--text-secondary)] hover:bg-[var(--bg-3)]'}"
				onclick={() => { filterType = t; }}
			>
				{t === 'all' ? 'All' : t}
			</button>
		{/each}
		<span class="flex-1"></span>
		<Button variant="outline" size="sm" class="h-7 text-xs gap-1" onclick={refresh} disabled={activityStore.loading}>
			{#if activityStore.loading}<Loader2 class="h-3 w-3 animate-spin" />{:else}<RefreshCw class="h-3 w-3" />{/if}
			Refresh
		</Button>
	</div>

	<!-- Activity stream -->
	{#if activityStore.loading && activityStore.entries.length === 0}
		<div class="space-y-3">
			{#each Array(5) as _}
				<Skeleton class="h-16 w-full rounded-lg" />
			{/each}
		</div>
	{:else if filteredEntries.length === 0}
		<div class="flex flex-col items-center py-16 text-[var(--text-tertiary)]">
			<Brain class="h-8 w-8 mb-3 opacity-40" />
			<p class="text-sm">No activity yet</p>
			<p class="text-xs mt-1 opacity-60">Activity will appear as cairn runs idle ticks, tasks, and reflections</p>
		</div>
	{:else}
		<div class="space-y-2">
			{#each filteredEntries as entry (entry.id)}
				{@const cfg = TYPE_CONFIG[entry.type] ?? TYPE_CONFIG.idle}
				{@const hasErrors = entry.errors && entry.errors.length > 0}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="rounded-lg border bg-[var(--bg-0)] transition-colors
						{hasErrors ? 'border-[var(--color-error)]/30' : 'border-border-subtle'}
						hover:bg-[var(--bg-1)] cursor-pointer"
					onclick={() => expandedId = expandedId === entry.id ? null : entry.id}
					role="button"
					tabindex="0"
					onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') expandedId = expandedId === entry.id ? null : entry.id; }}
				>
					<div class="flex items-center gap-3 px-4 py-3">
						<!-- Type indicator -->
						<span
							class="h-2 w-2 flex-shrink-0 rounded-full"
							style="background: {cfg.color}"
						></span>

						<!-- Content -->
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<Badge variant="outline" class="h-4 px-1 text-[9px] font-medium border-border-subtle" style="color: {cfg.color}">
									{cfg.label}
								</Badge>
								<p class="text-sm text-[var(--text-primary)] truncate">{entry.summary}</p>
							</div>
							<div class="flex items-center gap-2 mt-0.5 text-[10px] text-[var(--text-tertiary)]">
								<time datetime={entry.createdAt}>{relativeTime(entry.createdAt)}</time>
								{#if entry.toolCount > 0}
									<span>&middot; {entry.toolCount} tools</span>
								{/if}
								{#if entry.durationMs > 0}
									<span>&middot; {entry.durationMs < 1000 ? entry.durationMs + 'ms' : (entry.durationMs / 1000).toFixed(1) + 's'}</span>
								{/if}
							</div>
						</div>

						<!-- Error badge -->
						{#if hasErrors}
							<Badge variant="outline" class="h-5 px-1.5 text-[9px] text-[var(--color-error)] border-[var(--color-error)]/30">
								{entry.errors.length} err
							</Badge>
						{/if}

						{#if expandedId === entry.id}
							<ChevronUp class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
						{:else}
							<ChevronDown class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
						{/if}
					</div>

					<!-- Expanded details -->
					{#if expandedId === entry.id}
						<div class="px-4 pb-4 pt-1 border-t border-border-subtle">
							{#if entry.details}
								<div class="mb-2">
									<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Details</p>
									<pre class="text-xs text-[var(--text-secondary)] bg-[var(--bg-1)] rounded-md px-3 py-2 font-mono whitespace-pre-wrap overflow-x-auto max-h-60">{entry.details}</pre>
								</div>
							{/if}
							{#if hasErrors}
								<div>
									<p class="text-[10px] text-[var(--color-error)] uppercase tracking-wider mb-1">Errors</p>
									{#each entry.errors as err}
										<p class="text-xs text-[var(--color-error)] bg-[var(--color-error)]/5 rounded-md px-3 py-1.5 mb-1 font-mono">{err}</p>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
