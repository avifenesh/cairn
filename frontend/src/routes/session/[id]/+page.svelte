<script lang="ts">
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import { SessionStore } from '$lib/stores/session.svelte';
	import SessionHeader from '$lib/components/session/SessionHeader.svelte';
	import ActivityStream from '$lib/components/session/ActivityStream.svelte';
	import DiffViewer from '$lib/components/session/DiffViewer.svelte';
	import SteeringSidebar from '$lib/components/session/SteeringSidebar.svelte';
	import { Button } from '$lib/components/ui/button';
	import { List, FileText, MessageSquare } from '@lucide/svelte';

	const sessionId = $page.params.id ?? '';
	const store = new SessionStore(sessionId);

	// Mobile tab state.
	let activeTab = $state<'activity' | 'diffs' | 'steer'>('activity');
	let selectedFile = $state<string | null>(null);

	onMount(() => {
		store.connect();
	});

	onDestroy(() => {
		store.disconnect();
	});
</script>

<svelte:head>
	<title>Session {sessionId.slice(0, 8)} - Cairn</title>
</svelte:head>

<div class="session-page">
	<SessionHeader
		{sessionId}
		status={store.status}
		currentRound={store.currentRound}
		totalToolCalls={store.totalToolCalls}
		totalTokensIn={store.totalTokensIn}
		totalTokensOut={store.totalTokensOut}
	/>

	<!-- Mobile tab bar -->
	<div class="tab-bar md:hidden">
		<Button
			variant={activeTab === 'activity' ? 'default' : 'ghost'}
			size="sm"
			onclick={() => (activeTab = 'activity')}
		>
			<List size={14} />
			<span class="ml-1">Activity</span>
		</Button>
		<Button
			variant={activeTab === 'diffs' ? 'default' : 'ghost'}
			size="sm"
			onclick={() => (activeTab = 'diffs')}
		>
			<FileText size={14} />
			<span class="ml-1">Diffs</span>
			{#if store.fileChanges.length > 0}
				<span class="tab-badge">{store.fileChanges.length}</span>
			{/if}
		</Button>
		<Button
			variant={activeTab === 'steer' ? 'default' : 'ghost'}
			size="sm"
			onclick={() => (activeTab = 'steer')}
		>
			<MessageSquare size={14} />
			<span class="ml-1">Steer</span>
			{#if store.pendingApprovals.length > 0}
				<span class="tab-badge tab-badge--warn">{store.pendingApprovals.length}</span>
			{/if}
		</Button>
	</div>

	<!-- Panels -->
	<div class="panels">
		<!-- Activity Stream (always visible on desktop, tab on mobile) -->
		<div class="panel panel-activity" class:hidden-mobile={activeTab !== 'activity'}>
			<ActivityStream
				events={store.events}
				streamingText={store.streamingText}
				thinkingText={store.thinkingText}
			/>
		</div>

		<!-- Diff Viewer (always visible on desktop, tab on mobile) -->
		<div class="panel panel-diff" class:hidden-mobile={activeTab !== 'diffs'}>
			<DiffViewer files={store.fileChanges} bind:selectedFile />
		</div>

		<!-- Steering Sidebar (always visible on desktop, tab on mobile) -->
		<div class="panel panel-steer" class:hidden-mobile={activeTab !== 'steer'}>
			<SteeringSidebar {store} status={store.status} />
		</div>
	</div>

	{#if store.error}
		<div class="error-banner">
			{store.error}
		</div>
	{/if}
</div>

<style>
	.session-page {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}

	.tab-bar {
		display: flex;
		gap: 0.25rem;
		padding: 0.375rem 0.5rem;
		border-bottom: 1px solid hsl(var(--border));
		overflow-x: auto;
	}

	.tab-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.125rem;
		height: 1.125rem;
		border-radius: 9999px;
		background: hsl(var(--muted));
		font-size: 0.625rem;
		font-weight: 600;
		margin-left: 0.25rem;
	}
	.tab-badge--warn {
		background: #f59e0b;
		color: black;
	}

	.panels {
		flex: 1;
		display: flex;
		overflow: hidden;
	}

	.panel {
		display: flex;
		flex-direction: column;
		overflow: hidden;
		position: relative;
	}

	.panel-activity {
		flex: 2;
		border-right: 1px solid hsl(var(--border));
	}
	.panel-diff {
		flex: 2;
		border-right: 1px solid hsl(var(--border));
	}
	.panel-steer {
		flex: 1;
		min-width: 240px;
	}

	.error-banner {
		padding: 0.5rem 0.75rem;
		background: hsl(var(--destructive) / 0.1);
		color: hsl(var(--destructive));
		font-size: 0.8125rem;
		text-align: center;
	}

	/* Mobile: hide non-active panels */
	@media (max-width: 768px) {
		.hidden-mobile {
			display: none !important;
		}
		.panel {
			flex: 1;
		}
		.panel-activity,
		.panel-diff,
		.panel-steer {
			border-right: none;
		}
	}

	/* Desktop: always show all panels, hide tab bar */
	@media (min-width: 769px) {
		.tab-bar {
			display: none;
		}
	}
</style>
