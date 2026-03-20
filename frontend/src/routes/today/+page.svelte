<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getDashboard, getFeed, getApprovals, getAgentActivity, getCrons, getCosts, triggerPoll, markAllRead, markRead, deleteFeedItem, archiveFeedItem, approve, deny } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import { activityStore } from '$lib/stores/activity.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { createPullToRefresh } from '$lib/utils/touch.svelte';
	import FeedItemComponent from '$lib/components/feed/FeedItem.svelte';
	import ApprovalCard from '$lib/components/tasks/ApprovalCard.svelte';
	import type { DashboardResponse, CronJob, CostData, ActivityEntry } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Activity, Eye, Search, Loader2, Filter, Archive, Trash2, RefreshCw, CheckCheck, ChevronDown, ChevronUp, Brain, Zap, Clock, DollarSign, Radio, Sparkles, ArrowRight, MessageSquare, ShieldCheck } from '@lucide/svelte';

	// --- State ---
	let dashboard = $state<DashboardResponse | null>(null);
	let error = $state<string | null>(null);
	let cronJobs = $state<CronJob[]>([]);
	let costs = $state<CostData | null>(null);
	let chatInput = $state('');
	let showFullFeed = $state(false);
	let activeSource = $state<string | null>(null);
	let loadingMore = $state(false);

	// --- Data Loading ---
	async function loadAll() {
		const results = await Promise.allSettled([
			getDashboard({ limit: 20 }),
			getApprovals({ status: 'pending' }),
			getAgentActivity({ limit: 10 }),
			getCrons(),
			getCosts(),
		]);

		if (results[0].status === 'fulfilled') {
			dashboard = results[0].value;
			if (dashboard) feedStore.setItems(dashboard.feed, dashboard.feed.length >= 20);
		} else {
			error = 'Failed to load dashboard';
		}

		if (results[1].status === 'fulfilled') {
			taskStore.setApprovals(results[1].value.items);
		}

		if (results[2].status === 'fulfilled') {
			const actRes = results[2].value;
			activityStore.setEntries(actRes.items ?? []);
			if (actRes.stats) activityStore.setToolStats(actRes.stats);
		}

		if (results[3].status === 'fulfilled') {
			cronJobs = results[3].value.items ?? [];
		}

		if (results[4].status === 'fulfilled') {
			costs = results[4].value;
		}
	}

	onMount(() => { loadAll(); });

	const ptr = createPullToRefresh(async () => { await loadAll(); });

	// --- Derived State ---
	const greeting = $derived(() => {
		const hour = new Date().getHours();
		if (hour < 12) return 'Good morning';
		if (hour < 18) return 'Good afternoon';
		return 'Good evening';
	});

	const agentStatus = $derived(() => {
		const running = taskStore.activeTasks;
		if (running.length > 0) {
			const title = running[0].title ?? running[0].description ?? 'task';
			return { state: 'working' as const, label: title.length > 30 ? title.slice(0, 30) + '...' : title };
		}
		const progress = Object.values(appStore.agentProgresses);
		if (progress.length > 0) return { state: 'thinking' as const, label: progress[0] };
		return { state: 'idle' as const, label: 'Idle' };
	});

	const pendingApprovals = $derived(taskStore.pendingApprovals);
	const unreadHighlights = $derived(feedStore.items.filter(i => !i.isRead).slice(0, 5));
	const recentActivity = $derived(activityStore.entries.slice(0, 3));
	const pulseActivity = $derived(activityStore.entries.slice(0, 5));
	const hasActions = $derived(pendingApprovals.length > 0 || recentActivity.length > 0 || unreadHighlights.length > 0);

	const nextCron = $derived(() => {
		const now = new Date().toISOString();
		return cronJobs
			.filter(j => j.enabled && j.nextRunAt && j.nextRunAt > now)
			.sort((a, b) => (a.nextRunAt ?? '').localeCompare(b.nextRunAt ?? ''))[0] ?? null;
	});

	const heartbeatAgo = $derived(() => {
		const hb = appStore.lastHeartbeat;
		if (!hb) return null;
		const secs = Math.floor((Date.now() - hb.at) / 1000);
		if (secs < 60) return `${secs}s ago`;
		return `${Math.floor(secs / 60)}m ago`;
	});

	const activeSources = $derived(
		dashboard?.stats?.bySource ? Object.keys(dashboard.stats.bySource).sort() : []
	);

	const filteredItems = $derived(
		activeSource ? feedStore.items.filter(i => i.source === activeSource) : feedStore.items
	);

	const TYPE_COLORS: Record<string, string> = {
		task: 'var(--cairn-accent)',
		idle: 'var(--text-tertiary)',
		reflection: '#818CF8',
		cron: '#F59E0B',
		error: 'var(--color-error)',
	};

	// --- Handlers ---
	function handleChatSubmit() {
		if (!chatInput.trim()) return;
		goto(`/chat?msg=${encodeURIComponent(chatInput.trim())}`);
		chatInput = '';
	}

	async function handleApprove(id: string) {
		taskStore.resolveApproval(id, 'approved');
		await approve(id).catch(e => console.error('Approve failed:', e));
	}

	async function handleDeny(id: string) {
		taskStore.resolveApproval(id, 'denied');
		await deny(id).catch(e => console.error('Deny failed:', e));
	}

	async function handleSync() {
		await triggerPoll().catch(() => {});
		await loadAll();
	}

	async function handleMarkAllRead() {
		feedStore.markAllItemsRead();
		await markAllRead().catch(() => {});
	}

	async function handleDelete(id: string) {
		feedStore.removeItem(id);
		await deleteFeedItem(id).catch(() => {});
	}

	async function handleArchiveAll() {
		const ids = filteredItems.map(i => i.id);
		ids.forEach(id => feedStore.archiveItem(id));
		const results = await Promise.allSettled(ids.map(id => archiveFeedItem(id)));
		const failed = results.filter(r => r.status === 'rejected').length;
		if (failed > 0) console.error(`Failed to archive ${failed} items`);
	}

	async function handleDeleteAll() {
		const ids = filteredItems.map(i => i.id);
		ids.forEach(id => feedStore.removeItem(id));
		const results = await Promise.allSettled(ids.map(id => deleteFeedItem(id)));
		const failed = results.filter(r => r.status === 'rejected').length;
		if (failed > 0) console.error(`Failed to delete ${failed} items`);
	}

	function toggleSource(source: string) {
		activeSource = activeSource === source ? null : source;
	}

	const MAX_FEED_ITEMS = 200;
	async function loadMore() {
		if (loadingMore || !feedStore.hasMore || feedStore.items.length >= MAX_FEED_ITEMS) return;
		const lastItem = feedStore.items.at(-1);
		if (!lastItem) return;
		loadingMore = true;
		try {
			const remaining = MAX_FEED_ITEMS - feedStore.items.length;
			const res = await getFeed({ limit: Math.min(20, remaining), before: lastItem.id, source: activeSource ?? undefined });
			feedStore.appendItems(res.items, res.hasMore);
		} catch (e) {
			console.error('Failed to load more:', e);
		} finally {
			loadingMore = false;
		}
	}
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
		<div class="flex items-center justify-center transition-all duration-[var(--dur-fast)]" style="height: {ptr.state.refreshing ? 40 : ptr.state.distance}px">
			<Loader2 class="h-5 w-5 text-[var(--cairn-accent)] {ptr.state.triggered || ptr.state.refreshing ? 'animate-spin' : ''}" style="opacity: {ptr.state.refreshing ? 1 : Math.min(1, ptr.state.distance / 60)}" />
		</div>
	{/if}

	<!-- ZONE 1: BRIEFING STRIP -->
	<div class="mb-6 animate-in">
		<div class="flex flex-wrap items-center gap-3 mb-3">
			<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)] flex-1 min-w-0">{greeting()}</h1>

			<!-- Agent status pill -->
			<div class="flex items-center gap-1.5 rounded-full border border-border-subtle bg-[var(--bg-1)] px-2.5 py-1 text-xs transition-all duration-150">
				{#if agentStatus().state === 'idle'}
					<span class="h-2 w-2 rounded-full bg-[var(--color-success)] animate-pulse-dot"></span>
					<span class="text-[var(--text-secondary)]">Idle</span>
				{:else if agentStatus().state === 'thinking'}
					<span class="h-2 w-2 rounded-full bg-[var(--cairn-accent)] animate-pulse"></span>
					<span class="text-[var(--cairn-accent)]">Thinking</span>
				{:else}
					<Loader2 class="h-3 w-3 text-[var(--cairn-accent)] animate-spin" />
					<span class="text-[var(--cairn-accent)] truncate max-w-32">{agentStatus().label}</span>
				{/if}
			</div>

			<!-- Count badges -->
			<div class="flex items-center gap-2">
				{#if feedStore.unreadCount > 0}
					<a href="/today" class="flex items-center gap-1 rounded-full bg-[var(--cairn-accent)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--cairn-accent)]" onclick={(e) => { e.preventDefault(); showFullFeed = true; }}>
						<Eye class="h-3 w-3" /> {feedStore.unreadCount}
					</a>
				{/if}
				{#if pendingApprovals.length > 0}
					<span class="flex items-center gap-1 rounded-full bg-[var(--color-warning)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-warning)]">
						<ShieldCheck class="h-3 w-3" /> {pendingApprovals.length}
					</span>
				{/if}
				{#if heartbeatAgo()}
					<span class="text-[10px] text-[var(--text-tertiary)] tabular-nums">{heartbeatAgo()}</span>
				{/if}
			</div>
		</div>

		<!-- Quick chat input -->
		<div class="relative">
			<MessageSquare class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-[var(--text-tertiary)]" />
			<input
				type="text"
				bind:value={chatInput}
				placeholder="Ask cairn..."
				class="w-full rounded-lg border border-border-subtle bg-[var(--bg-1)] pl-9 pr-4 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none focus:ring-1 focus:ring-[var(--cairn-accent)]/30"
				onkeydown={(e) => { if (e.key === 'Enter') handleChatSubmit(); }}
			/>
		</div>
	</div>

	{#if error}
		<div class="rounded-lg border border-[var(--color-error)]/20 bg-[var(--color-error)]/5 p-4 text-sm text-[var(--color-error)] mb-6">{error}</div>
	{:else if !dashboard}
		<!-- Loading skeletons -->
		<div class="space-y-4 mb-8">
			<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				{#each Array(3) as _, i}
					<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 animate-in" style="animation-delay: {i * 50}ms">
						<Skeleton class="h-3 w-20 mb-3" />
						<Skeleton class="h-4 w-full mb-2" />
						<Skeleton class="h-3 w-3/4" />
					</div>
				{/each}
			</div>
		</div>
	{:else}

		<!-- ZONE 2: ACTION CARDS -->
		{#if hasActions}
			<div class="mb-8 grid grid-cols-1 lg:grid-cols-3 gap-4">
				<!-- 2a: Pending Approvals -->
				{#if pendingApprovals.length > 0}
					<div class="animate-in" style="animation-delay: 100ms">
						<div class="flex items-center gap-2 mb-2">
							<ShieldCheck class="h-3.5 w-3.5 text-[var(--color-warning)]" />
							<span class="text-[11px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">Approvals</span>
							<Badge variant="outline" class="h-4 px-1 text-[10px]">{pendingApprovals.length}</Badge>
						</div>
						<div class="space-y-2">
							{#each pendingApprovals.slice(0, 3) as approval (approval.id)}
								<ApprovalCard
									{approval}
									onapprove={handleApprove}
									ondeny={handleDeny}
								/>
							{/each}
							{#if pendingApprovals.length > 3}
								<a href="/ops" class="text-xs text-[var(--cairn-accent)] hover:underline flex items-center gap-1">
									+{pendingApprovals.length - 3} more in Ops <ArrowRight class="h-3 w-3" />
								</a>
							{/if}
						</div>
					</div>
				{/if}

				<!-- 2b: Agent Activity -->
				{#if recentActivity.length > 0}
					<div class="animate-in" style="animation-delay: 150ms">
						<div class="flex items-center gap-2 mb-2">
							<Brain class="h-3.5 w-3.5 text-[var(--text-tertiary)]" />
							<span class="text-[11px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">Agent Activity</span>
						</div>
						<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 space-y-2">
							{#each recentActivity as entry (entry.id)}
								<div class="flex items-start gap-2">
									<span class="mt-1 h-2 w-2 rounded-full flex-shrink-0" style="background: {TYPE_COLORS[entry.type] ?? 'var(--text-tertiary)'}"></span>
									<div class="min-w-0 flex-1">
										<p class="text-xs text-[var(--text-primary)] line-clamp-1">{entry.summary}</p>
										<p class="text-[10px] text-[var(--text-tertiary)]">{relativeTime(entry.createdAt)}</p>
									</div>
								</div>
							{/each}
							<a href="/activity" class="text-xs text-[var(--cairn-accent)] hover:underline flex items-center gap-1 pt-1">
								View all <ArrowRight class="h-3 w-3" />
							</a>
						</div>
					</div>
				{/if}

				<!-- 2c: Unread Highlights -->
				{#if unreadHighlights.length > 0}
					<div class="animate-in" style="animation-delay: 200ms">
						<div class="flex items-center gap-2 mb-2">
							<Eye class="h-3.5 w-3.5 text-[var(--cairn-accent)]" />
							<span class="text-[11px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">Unread</span>
							<Badge variant="outline" class="h-4 px-1 text-[10px]">{feedStore.unreadCount}</Badge>
						</div>
						<div class="space-y-1.5">
							{#each unreadHighlights as item (item.id)}
								<FeedItemComponent {item} ondelete={handleDelete} />
							{/each}
							{#if feedStore.unreadCount > 5}
								<button class="text-xs text-[var(--cairn-accent)] hover:underline flex items-center gap-1" onclick={() => showFullFeed = true} type="button">
									View all {feedStore.unreadCount} unread <ArrowRight class="h-3 w-3" />
								</button>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/if}

		<!-- ZONE 3: SYSTEM PULSE -->
		<div class="mb-6 animate-in" style="animation-delay: 250ms">
			<div class="flex flex-wrap items-center gap-3 md:gap-4 rounded-lg border border-border-subtle bg-[var(--bg-1)] px-4 py-2.5">
				{#if dashboard.poller}
					<div class="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)]">
						<Radio class="h-3 w-3 {dashboard.poller.running ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
						<span>{Object.keys(dashboard.poller.sources ?? {}).length} pollers</span>
					</div>
				{/if}
				{#if dashboard.stats}
					<div class="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)]">
						<Sparkles class="h-3 w-3" />
						<span>{dashboard.stats.total} events</span>
					</div>
				{/if}
				{#if costs}
					<div class="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)]">
						<DollarSign class="h-3 w-3" />
						<span>${costs.todayUsd.toFixed(2)} today</span>
					</div>
				{/if}
				{#if nextCron()}
					<div class="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)]">
						<Clock class="h-3 w-3" />
						<span>Next: {nextCron()?.name ?? 'cron'} {nextCron()?.nextRunAt ? relativeTime(nextCron()!.nextRunAt!) : ''}</span>
					</div>
				{/if}
				{#if !appStore.sseConnected}
					<span class="text-[10px] text-[var(--color-warning)]">SSE disconnected</span>
				{/if}
			</div>

			<!-- Mini activity stream -->
			{#if pulseActivity.length > 0}
				<div class="mt-3 space-y-1">
					{#each pulseActivity as entry (entry.id)}
						<div class="flex items-center gap-2 text-[11px]">
							<span class="h-1.5 w-1.5 rounded-full flex-shrink-0" style="background: {TYPE_COLORS[entry.type] ?? 'var(--text-tertiary)'}"></span>
							<span class="text-[var(--text-secondary)] truncate flex-1">{entry.summary}</span>
							<span class="text-[var(--text-tertiary)] tabular-nums flex-shrink-0">{relativeTime(entry.createdAt)}</span>
						</div>
					{/each}
					<a href="/activity" class="text-[11px] text-[var(--cairn-accent)] hover:underline flex items-center gap-1">
						Full activity <ArrowRight class="h-3 w-3" />
					</a>
				</div>
			{/if}
		</div>

		<!-- FULL FEED (collapsible) -->
		<div class="animate-in" style="animation-delay: 300ms">
			<button
				class="flex items-center gap-2 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors mb-3"
				onclick={() => showFullFeed = !showFullFeed}
				type="button"
			>
				{#if showFullFeed}
					<ChevronUp class="h-4 w-4" />
				{:else}
					<ChevronDown class="h-4 w-4" />
				{/if}
				{showFullFeed ? 'Hide' : 'Show'} full feed ({feedStore.items.length} items)
			</button>

			{#if showFullFeed}
				<!-- Source filter chips -->
				{#if activeSources.length > 0}
					<div class="mb-3 flex flex-wrap gap-1.5">
						{#each activeSources as source}
							{@const count = dashboard?.stats?.bySource?.[source] ?? 0}
							<button
								class="rounded-full border px-2.5 py-0.5 text-[10px] font-medium transition-colors
									{activeSource === source
									? 'bg-[var(--cairn-accent)]/10 text-[var(--cairn-accent)] border-[var(--cairn-accent)]/30'
									: 'border-border-subtle text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'}"
								onclick={() => toggleSource(source)}
								type="button"
							>
								{source} {#if count}<span class="opacity-60">({count})</span>{/if}
							</button>
						{/each}
					</div>
				{/if}

				<!-- Quick actions -->
				<div class="mb-3 flex items-center gap-2">
					<Button variant="outline" size="sm" class="h-6 text-[10px] gap-1 px-2" onclick={handleSync}>
						<RefreshCw class="h-3 w-3" /> Sync
					</Button>
					<Button variant="outline" size="sm" class="h-6 text-[10px] gap-1 px-2" onclick={handleMarkAllRead}>
						<CheckCheck class="h-3 w-3" /> Read all
					</Button>
					<Button variant="outline" size="sm" class="h-6 text-[10px] gap-1 px-2" onclick={handleArchiveAll}>
						<Archive class="h-3 w-3" /> Archive all
					</Button>
					<Button variant="outline" size="sm" class="h-6 text-[10px] gap-1 px-2 text-[var(--color-error)]" onclick={handleDeleteAll}>
						<Trash2 class="h-3 w-3" /> Delete all
					</Button>
				</div>

				<!-- Feed list -->
				{#if filteredItems.length === 0}
					<p class="py-8 text-center text-sm text-[var(--text-tertiary)]">No events</p>
				{:else}
					<div class="flex flex-col gap-1.5">
						{#each filteredItems as item, i (item.id)}
							<div class="animate-in" style="animation-delay: {Math.min(i, 10) * 20}ms">
								<FeedItemComponent {item} ondelete={handleDelete} />
							</div>
						{/each}
					</div>

					{#if feedStore.hasMore && feedStore.items.length < MAX_FEED_ITEMS}
						<div class="mt-4 flex justify-center">
							<Button variant="outline" size="sm" class="text-xs" onclick={loadMore} disabled={loadingMore}>
								{#if loadingMore}<Loader2 class="h-3 w-3 animate-spin mr-1" />{/if}
								Load more
							</Button>
						</div>
					{/if}
				{/if}
			{/if}
		</div>
	{/if}
</div>
