<script lang="ts">
	import type { SessionEvent, SessionStatus } from '$lib/types';
	import type { SessionStore } from '$lib/stores/session.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Send, Square, AlertTriangle, MessageSquare } from '@lucide/svelte';

	let { store, status }: {
		store: SessionStore;
		status: SessionStatus;
	} = $props();

	let input = $state('');
	let sending = $state(false);

	const steeringMessages = $derived(store.steeringMessages);
	const pendingApprovals = $derived(store.pendingApprovals);
	const isActive = $derived(status === 'running' || status === 'paused' || status === 'waiting_approval');

	async function sendSteer() {
		if (!input.trim() || sending) return;
		sending = true;
		try {
			await store.steer(input.trim());
			input = '';
		} catch (e) {
			console.error('Steering failed:', e);
		} finally {
			sending = false;
		}
	}

	async function stopSession() {
		try {
			await store.stop();
		} catch (e) {
			console.error('Stop failed:', e);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendSteer();
		}
	}
</script>

<div class="steering-sidebar">
	<!-- Pending approvals -->
	{#if pendingApprovals.length > 0}
		<div class="approvals-section">
			{#each pendingApprovals as approval}
				<div class="approval-card">
					<div class="approval-header">
						<AlertTriangle size={14} class="text-amber-500" />
						<Badge variant="outline" class="border-amber-500 text-amber-500 text-xs">Approval</Badge>
					</div>
					<p class="text-xs mt-1">{approval.payload.description ?? approval.payload.operation}</p>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Steering messages history -->
	<div class="messages-area">
		{#each steeringMessages as msg}
			<div class="steer-msg">
				<MessageSquare size={12} class="text-blue-400 shrink-0" />
				<span class="text-xs text-blue-400 italic">{msg.payload.content}</span>
			</div>
		{/each}
	</div>

	<!-- Controls -->
	<div class="controls">
		{#if isActive}
			<div class="input-row">
				<input
					type="text"
					bind:value={input}
					onkeydown={handleKeydown}
					placeholder="Steer the agent..."
					class="steer-input"
					disabled={!isActive || sending}
				/>
				<Button size="sm" variant="default" onclick={sendSteer} disabled={!input.trim() || sending}>
					<Send size={14} />
				</Button>
			</div>
			<div class="action-row">
				<Button size="sm" variant="destructive" onclick={stopSession}>
					<Square size={14} />
					<span class="ml-1">Stop</span>
				</Button>
			</div>
		{:else}
			<div class="session-ended">
				<Badge variant={status === 'completed' ? 'default' : 'destructive'}>
					Session {status}
				</Badge>
			</div>
		{/if}
	</div>
</div>

<style>
	.steering-sidebar {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}
	.approvals-section {
		padding: 0.5rem;
		border-bottom: 1px solid hsl(var(--border));
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.approval-card {
		border: 1px solid hsl(var(--border));
		border-left: 3px solid #f59e0b;
		border-radius: 0.375rem;
		padding: 0.5rem;
	}
	.approval-header {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}
	.messages-area {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.steer-msg {
		display: flex;
		align-items: flex-start;
		gap: 0.375rem;
		padding: 0.25rem;
	}
	.controls {
		padding: 0.5rem;
		border-top: 1px solid hsl(var(--border));
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.input-row {
		display: flex;
		gap: 0.375rem;
	}
	.steer-input {
		flex: 1;
		padding: 0.375rem 0.5rem;
		border-radius: 0.375rem;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--background));
		color: inherit;
		font-size: 0.8125rem;
	}
	.steer-input:focus {
		outline: none;
		border-color: var(--cairn-accent, #60a5fa);
	}
	.action-row {
		display: flex;
		gap: 0.375rem;
	}
	.session-ended {
		display: flex;
		justify-content: center;
		padding: 0.5rem;
	}
</style>
