<script lang="ts">
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import { SessionStore } from '$lib/stores/session.svelte';
	import SessionHeader from '$lib/components/session/SessionHeader.svelte';
	import ActivityStream from '$lib/components/session/ActivityStream.svelte';
	import DiffViewer from '$lib/components/session/DiffViewer.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Send, Square, PanelRightClose, PanelRight, X } from '@lucide/svelte';

	const sessionId = $page.params.id ?? '';
	const store = new SessionStore(sessionId);

	let steerInput = $state('');
	let sending = $state(false);
	let selectedFile = $state<string | null>(null);
	let showDiffs = $state(false);

	const isActive = $derived(
		store.status === 'running' || store.status === 'paused' || store.status === 'waiting_approval'
	);
	const hasFiles = $derived(store.fileChanges.length > 0);

	onMount(() => {
		store.connect();
	});

	onDestroy(() => {
		store.disconnect();
	});

	async function sendSteer() {
		if (!steerInput.trim() || sending) return;
		sending = true;
		try {
			await store.steer(steerInput.trim());
			steerInput = '';
		} catch (e) {
			console.error('Steering failed:', e);
		} finally {
			sending = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendSteer();
		}
	}
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

	<div class="main-area">
		<!-- Activity stream with inline steering -->
		<div class="stream-panel">
			<ActivityStream
				events={store.events}
				streamingText={store.streamingText}
				thinkingText={store.thinkingText}
			/>

			<!-- Inline steering input at bottom of stream -->
			<div class="steer-bar">
				{#if isActive}
					<Button size="sm" variant="destructive" aria-label="Stop session" onclick={() => store.stop().catch(() => {})} class="shrink-0">
						<Square size={14} />
					</Button>
					<input
						type="text"
						bind:value={steerInput}
						onkeydown={handleKeydown}
						placeholder="Steer the agent..."
						class="steer-input"
						disabled={sending}
					/>
					<Button size="sm" variant="default" aria-label="Send steering message" onclick={sendSteer} disabled={!steerInput.trim() || sending} class="shrink-0">
						<Send size={14} />
					</Button>
				{:else}
					<span class="session-ended-label">Session {store.status}</span>
				{/if}
				{#if hasFiles}
					<Button
						size="sm"
						variant={showDiffs ? 'secondary' : 'ghost'}
						onclick={() => (showDiffs = !showDiffs)}
						class="shrink-0 ml-1"
						aria-label={showDiffs ? 'Hide diffs' : 'Show diffs'}
						aria-pressed={showDiffs}
					>
						{#if showDiffs}<PanelRightClose size={14} />{:else}<PanelRight size={14} />{/if}
						<span class="ml-1 text-xs">{store.fileChanges.length}</span>
					</Button>
				{/if}
			</div>
		</div>

		<!-- Diff panel (toggleable, only shown when there are file changes) -->
		{#if showDiffs && hasFiles}
			<div class="diff-panel">
				<div class="diff-panel-header md:hidden">
					<span class="text-xs font-medium">Changed Files</span>
					<Button size="sm" variant="ghost" aria-label="Close diff panel" onclick={() => (showDiffs = false)}>
						<X size={14} />
					</Button>
				</div>
				<DiffViewer files={store.fileChanges} bind:selectedFile />
			</div>
		{/if}
	</div>

	{#if store.error}
		<div class="error-banner">{store.error}</div>
	{/if}
</div>

<style>
	.session-page {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}

	.main-area {
		flex: 1;
		display: flex;
		overflow: hidden;
		position: relative;
	}

	.stream-panel {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		min-width: 0;
	}

	.diff-panel {
		width: 40%;
		min-width: 300px;
		max-width: 600px;
		border-left: 1px solid hsl(var(--border));
		overflow: hidden;
		display: flex;
		flex-direction: column;
	}

	.diff-panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.375rem 0.5rem;
		border-bottom: 1px solid hsl(var(--border));
	}

	.steer-bar {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.5rem 0.75rem;
		border-top: 1px solid hsl(var(--border));
		background: hsl(var(--background));
	}

	.steer-input {
		flex: 1;
		padding: 0.375rem 0.625rem;
		border-radius: 0.375rem;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--background));
		color: inherit;
		font-size: 0.8125rem;
		min-width: 0;
	}
	.steer-input:focus {
		outline: none;
		border-color: var(--cairn-accent, #60a5fa);
	}

	.session-ended-label {
		flex: 1;
		font-size: 0.8125rem;
		color: var(--text-tertiary, hsl(var(--muted-foreground)));
		text-align: center;
	}

	.error-banner {
		padding: 0.5rem 0.75rem;
		background: hsl(var(--destructive) / 0.1);
		color: hsl(var(--destructive));
		font-size: 0.8125rem;
		text-align: center;
	}

	@media (max-width: 768px) {
		.diff-panel {
			position: absolute;
			right: 0;
			top: 0;
			bottom: 0;
			width: 85%;
			max-width: none;
			z-index: 20;
			background: hsl(var(--background));
			box-shadow: -4px 0 16px rgba(0, 0, 0, 0.2);
		}
	}
</style>
