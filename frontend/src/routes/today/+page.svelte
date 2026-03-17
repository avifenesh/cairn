<script lang="ts">
	import { onMount } from 'svelte';
	import { getDashboard, getFeed, triggerPoll, markAllRead } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import FeedItemComponent from '$lib/components/feed/FeedItem.svelte';
	import type { DashboardResponse } from '$lib/types';
	import { Activity, Eye, Zap, TrendingUp, RefreshCw, CheckCheck, Loader2 } from '@lucide/svelte';
	import { createPullToRefresh } from '$lib/utils/touch.svelte';

	let dashboard = $state<DashboardResponse | null>(null);
	let error = $state<string | null>(null);

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
	}

	async function handleMarkAllRead() {
		feedStore.markAllItemsRead();
		await markAllRead().catch(() => {});
	}

	let loadingMore = $state(false);
	async function loadMore() {
		if (loadingMore || !feedStore.hasMore) return;
		const lastItem = feedStore.items.at(-1);
		if (!lastItem) return;
		loadingMore = true;
		try {
			const res = await getFeed({ limit: 20, before: lastItem.createdAt });
			feedStore.appendItems(res.items, res.hasMore);
		} catch {
			// handled
		} finally {
			loadingMore = false;
		}
	}
</script>

<div
	class="mx-auto max-w-4xl p-6 overflow-y-auto h-full"
	ontouchstart={ptr.handleTouchStart}
	ontouchmove={ptr.handleTouchMove}
	ontouchend={ptr.handleTouchEnd}
	ontouchcancel={ptr.handleTouchCancel}
>
	<!-- Pull-to-refresh indicator -->
	{#if ptr.state.distance > 0 || ptr.state.refreshing}
		<div
			class="flex items-center justify-center transition-all duration-[var(--dur-fast)]"
			style="height: {ptr.state.refreshing ? 40 : ptr.state.distance}px"
		>
			<Loader2
				class="h-5 w-5 text-[var(--pub-accent)] {ptr.state.triggered || ptr.state.refreshing ? 'animate-spin' : ''}"
				style="opacity: {ptr.state.refreshing ? 1 : Math.min(1, ptr.state.distance / 60)}"
			/>
		</div>
	{/if}

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

		<!-- Quick actions -->
		<div class="mb-6 flex gap-2">
			<button
				class="flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors"
				onclick={handleSync}
			>
				<RefreshCw class="h-3.5 w-3.5" /> Sync now
			</button>
			{#if feedStore.unreadCount > 0}
				<button
					class="flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors"
					onclick={handleMarkAllRead}
				>
					<CheckCheck class="h-3.5 w-3.5" /> Mark all read
				</button>
			{/if}
		</div>

		<!-- Recent activity -->
		<h2 class="mb-4 text-lg font-medium text-[var(--text-primary)]">Recent Activity</h2>
		<div class="flex flex-col gap-2">
			{#each feedStore.items as item (item.id)}
				<FeedItemComponent {item} />
			{/each}
		</div>
		{#if feedStore.hasMore}
			<button
				class="mt-4 w-full rounded-lg border border-border-subtle bg-[var(--bg-2)] py-2 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors disabled:opacity-50"
				onclick={loadMore}
				disabled={loadingMore}
			>
				{loadingMore ? 'Loading...' : 'Load more'}
			</button>
		{/if}
	{/if}
</div>
