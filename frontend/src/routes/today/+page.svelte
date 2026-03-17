<script lang="ts">
	import { onMount } from 'svelte';
	import { getDashboard } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { relativeTime } from '$lib/utils/time';
	import type { DashboardResponse } from '$lib/types';
	import { Activity, Eye, Zap, TrendingUp } from '@lucide/svelte';

	let dashboard = $state<DashboardResponse | null>(null);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			dashboard = await getDashboard({ limit: 20 });
			if (dashboard) {
				feedStore.setItems(dashboard.feed, dashboard.feed.length >= 20);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load dashboard';
		}
	});

	const greeting = $derived(() => {
		const hour = new Date().getHours();
		if (hour < 12) return 'Good morning';
		if (hour < 18) return 'Good afternoon';
		return 'Good evening';
	});
</script>

<div class="mx-auto max-w-4xl p-6">
	<h1 class="mb-6 text-2xl font-semibold text-[var(--text-primary)]">
		{greeting()}, Avi
	</h1>

	{#if error}
		<div class="rounded-lg border border-[var(--color-error)]/20 bg-[var(--color-error)]/5 p-4 text-sm text-[var(--color-error)]">
			{error}
		</div>
	{:else if !dashboard}
		<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
			{#each Array(4) as _}
				<div class="h-24 animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
			{/each}
		</div>
	{:else}
		<!-- Stats cards -->
		<div class="mb-8 grid grid-cols-2 gap-4 md:grid-cols-4">
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)]">
					<Activity class="h-4 w-4" />
					<span class="text-xs">Total Events</span>
				</div>
				<p class="mt-2 text-2xl font-semibold text-[var(--text-primary)]">{dashboard.stats.total}</p>
			</div>
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)]">
					<Eye class="h-4 w-4" />
					<span class="text-xs">Unread</span>
				</div>
				<p class="mt-2 text-2xl font-semibold text-[var(--pub-accent)]">{dashboard.stats.unread}</p>
			</div>
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)]">
					<Zap class="h-4 w-4" />
					<span class="text-xs">Sources</span>
				</div>
				<p class="mt-2 text-2xl font-semibold text-[var(--text-primary)]">
					{Object.keys(dashboard.stats.bySource).length}
				</p>
			</div>
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)]">
					<TrendingUp class="h-4 w-4" />
					<span class="text-xs">Poller</span>
				</div>
				<p class="mt-2 text-sm font-medium text-[var(--text-primary)]">
					{dashboard.poller.running ? 'Active' : 'Stopped'}
				</p>
			</div>
		</div>

		<!-- Recent activity -->
		<h2 class="mb-4 text-lg font-medium text-[var(--text-primary)]">Recent Activity</h2>
		<div class="flex flex-col gap-2">
			{#each feedStore.items.slice(0, 20) as item (item.id)}
				<a
					href={item.url ?? '#'}
					target={item.url ? '_blank' : undefined}
					rel="noopener"
					class="flex items-start gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 transition-colors duration-[var(--dur-fast)] hover:bg-[var(--bg-2)]"
					class:opacity-60={item.isRead}
				>
					<span
						class="mt-0.5 h-2 w-2 flex-shrink-0 rounded-full"
						style="background: var(--src-{item.source}, var(--text-tertiary))"
					></span>
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm text-[var(--text-primary)]">{item.title}</p>
						<p class="mt-0.5 text-xs text-[var(--text-tertiary)]">
							{item.source} &middot; {relativeTime(item.createdAt)}
						</p>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>
