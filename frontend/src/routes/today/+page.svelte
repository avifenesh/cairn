<script lang="ts">
	import { onMount } from 'svelte';
	import { getDashboard, getFeed, triggerPoll, markAllRead, deleteFeedItem } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import FeedItemComponent from '$lib/components/feed/FeedItem.svelte';
	import type { DashboardResponse } from '$lib/types';
	import { Activity, Eye, Zap, TrendingUp, RefreshCw, CheckCheck, Loader2, Filter } from '@lucide/svelte';
	import { createPullToRefresh } from '$lib/utils/touch.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Badge } from '$lib/components/ui/badge';

	let dashboard = $state<DashboardResponse | null>(null);
	let error = $state<string | null>(null);
	let activeSource = $state<string | null>(null);

	const SOURCES = ['github', 'gmail', 'calendar', 'hn', 'reddit', 'npm', 'crates', 'webhook'];

	async function loadDashboard() {
		dashboard = await getDashboard({ limit: 20 });
		if (dashboard) {
			feedStore.setItems(dashboard.feed, dashboard.feed.length >= 20);
		}
	}

	onMount(async () => {
		try {
			await loadDashboard();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load dashboard';
		}
	});

	const ptr = createPullToRefresh(async () => {
		await loadDashboard().catch(() => {});
	});

	const greeting = $derived(() => {
		const hour = new Date().getHours();
		if (hour < 12) return 'Good morning';
		if (hour < 18) return 'Good afternoon';
		return 'Good evening';
	});

	async function handleSync() {
		await triggerPoll().catch(() => {});
		await loadDashboard().catch(() => {});
	}

	async function handleMarkAllRead() {
		feedStore.markAllItemsRead();
		await markAllRead().catch(() => {});
	}

	function toggleSource(source: string) {
		activeSource = activeSource === source ? null : source;
	}

	const filteredItems = $derived(
		activeSource
			? feedStore.items.filter((i) => i.source === activeSource)
			: feedStore.items
	);

	async function handleDelete(id: string) {
		feedStore.removeItem(id);
		await deleteFeedItem(id).catch(() => {});
	}

	const MAX_FEED_ITEMS = 200;
	let loadingMore = $state(false);
	let loadMoreError = $state<string | null>(null);
	async function loadMore() {
		if (loadingMore || !feedStore.hasMore || feedStore.items.length >= MAX_FEED_ITEMS) return;
		const lastItem = feedStore.items.at(-1);
		if (!lastItem) return;
		loadingMore = true;
		loadMoreError = null;
		try {
			const remaining = MAX_FEED_ITEMS - feedStore.items.length;
			const res = await getFeed({
				limit: Math.min(20, remaining),
				before: lastItem.id,
				source: activeSource ?? undefined,
			});
			feedStore.appendItems(res.items, res.hasMore);
		} catch (e) {
			loadMoreError = e instanceof Error ? e.message : 'Failed to load more';
		} finally {
			loadingMore = false;
		}
	}

	// Compute active sources from dashboard stats
	const activeSources = $derived(
		dashboard?.stats?.bySource
			? Object.keys(dashboard.stats.bySource).sort()
			: []
	);
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="mx-auto max-w-5xl p-6 overflow-y-auto h-full"
	ontouchstart={ptr.handleTouchStart}
	ontouchmove={ptr.handleTouchMove}
	ontouchend={ptr.handleTouchEnd}
	ontouchcancel={ptr.handleTouchCancel}
>
	{#if ptr.state.distance > 0 || ptr.state.refreshing}
		<div
			class="flex items-center justify-center transition-all duration-[var(--dur-fast)]"
			style="height: {ptr.state.refreshing ? 40 : ptr.state.distance}px"
		>
			<Loader2
				class="h-5 w-5 text-[var(--cairn-accent)] {ptr.state.triggered || ptr.state.refreshing ? 'animate-spin' : ''}"
				style="opacity: {ptr.state.refreshing ? 1 : Math.min(1, ptr.state.distance / 60)}"
			/>
		</div>
	{/if}

	<h1 class="mb-6 text-2xl font-semibold tracking-tight text-[var(--text-primary)]">
		{greeting()}
	</h1>

	{#if error}
		<div class="rounded-lg border border-[var(--color-error)]/20 bg-[var(--color-error)]/5 p-4 text-sm text-[var(--color-error)]">
			{error}
		</div>
	{:else if !dashboard}
		<div class="grid grid-cols-2 gap-3 md:grid-cols-4 mb-8">
			{#each Array(4) as _, i}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 animate-in" style="animation-delay: {i * 50}ms">
					<Skeleton class="h-3 w-16 mb-3" />
					<Skeleton class="h-7 w-12" />
				</div>
			{/each}
		</div>
	{:else}
		<!-- Stats cards -->
		<div class="mb-8 grid grid-cols-2 gap-3 md:grid-cols-4">
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover animate-in" style="animation-delay: 0ms">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)] mb-2">
					<Activity class="h-3.5 w-3.5" />
					<span class="text-[11px] font-medium uppercase tracking-wider">Events</span>
				</div>
				<p class="text-2xl font-semibold tabular-nums text-[var(--text-primary)]">{dashboard.stats.total ?? 0}</p>
			</div>
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover animate-in" style="animation-delay: 50ms">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)] mb-2">
					<Eye class="h-3.5 w-3.5" />
					<span class="text-[11px] font-medium uppercase tracking-wider">Unread</span>
				</div>
				<p class="text-2xl font-semibold tabular-nums text-[var(--cairn-accent)]">{dashboard.stats.unread ?? 0}</p>
			</div>
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover animate-in" style="animation-delay: 100ms">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)] mb-2">
					<Zap class="h-3.5 w-3.5" />
					<span class="text-[11px] font-medium uppercase tracking-wider">Sources</span>
				</div>
				<p class="text-2xl font-semibold tabular-nums text-[var(--text-primary)]">
					{Object.keys(dashboard.stats.bySource ?? {}).length}
				</p>
			</div>
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 card-hover animate-in" style="animation-delay: 150ms">
				<div class="flex items-center gap-2 text-[var(--text-tertiary)] mb-2">
					<TrendingUp class="h-3.5 w-3.5" />
					<span class="text-[11px] font-medium uppercase tracking-wider">Status</span>
				</div>
				<div class="flex items-center gap-2">
					<span class="h-2 w-2 rounded-full {dashboard.poller?.running ? 'bg-[var(--color-success)] animate-pulse-dot' : 'bg-[var(--text-tertiary)]'}"></span>
					<p class="text-sm font-medium text-[var(--text-primary)]">
						{dashboard.poller?.running ? 'Active' : 'Stopped'}
					</p>
				</div>
			</div>
		</div>

		<!-- Source filter chips -->
		{#if activeSources.length > 0}
			<div class="mb-4 flex items-center gap-2 flex-wrap">
				<Filter class="h-3.5 w-3.5 text-[var(--text-tertiary)]" />
				{#each activeSources as source}
					<button
						class="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-medium transition-colors
							{activeSource === source
								? 'bg-[var(--cairn-accent)] text-white'
								: 'bg-[var(--bg-2)] text-[var(--text-secondary)] hover:bg-[var(--bg-3)]'}"
						onclick={() => toggleSource(source)}
					>
						{source}
						{#if dashboard.stats.bySource?.[source]}
							<span class="tabular-nums opacity-70">{dashboard.stats.bySource[source]}</span>
						{/if}
					</button>
				{/each}
				{#if activeSource}
					<button
						class="text-[11px] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] underline"
						onclick={() => activeSource = null}
					>
						Clear
					</button>
				{/if}
			</div>
		{/if}

		<!-- Quick actions -->
		<div class="mb-4 flex items-center gap-2">
			<Button variant="outline" size="sm" onclick={handleSync} class="h-7 text-xs gap-1.5">
				<RefreshCw class="h-3 w-3" /> Sync
			</Button>
			{#if feedStore.unreadCount > 0}
				<Button variant="outline" size="sm" onclick={handleMarkAllRead} class="h-7 text-xs gap-1.5">
					<CheckCheck class="h-3 w-3" /> Mark all read
				</Button>
			{/if}
			<span class="flex-1"></span>
			<span class="text-[11px] text-[var(--text-tertiary)] tabular-nums font-mono">
				{filteredItems.length} items{activeSource ? ` (${activeSource})` : ''}
			</span>
		</div>

		<!-- Feed -->
		{#if filteredItems.length === 0}
			<div class="flex flex-col items-center justify-center py-16 text-[var(--text-tertiary)]">
				<Activity class="h-8 w-8 mb-3 opacity-40" />
				<p class="text-sm">
					{activeSource ? `No ${activeSource} events` : 'No events yet'}
				</p>
				<p class="text-xs mt-1 opacity-60">Events will appear as sources are polled</p>
			</div>
		{:else}
			<div class="flex flex-col gap-1" role="feed" aria-label="Recent activity">
				{#each filteredItems as item, i (item.id)}
					<div class="animate-in" style="animation-delay: {Math.min(i * 30, 300)}ms">
						<FeedItemComponent {item} ondelete={handleDelete} />
					</div>
				{/each}
			</div>
		{/if}

		{#if loadMoreError}
			<p class="mt-3 text-center text-xs text-[var(--color-error)]">{loadMoreError}</p>
		{/if}
		{#if feedStore.hasMore && feedStore.items.length < MAX_FEED_ITEMS && !activeSource}
			<Button
				variant="ghost"
				class="mt-4 w-full text-xs text-[var(--text-tertiary)]"
				onclick={loadMore}
				disabled={loadingMore}
			>
				{loadingMore ? 'Loading...' : 'Load more'}
			</Button>
		{/if}
	{/if}
</div>
