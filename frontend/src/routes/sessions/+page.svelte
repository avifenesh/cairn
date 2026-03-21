<script lang="ts">
	import { onMount } from 'svelte';
	import { getSessions } from '$lib/api/client';
	import type { ChatSession } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { relativeTime } from '$lib/utils/time';
	import { MonitorPlay, MessageSquare, Clock, ChevronRight, Loader2 } from '@lucide/svelte';

	let sessions = $state<ChatSession[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await getSessions();
			// Backend already returns sessions sorted by updated_at DESC.
			sessions = res.items ?? [];
		} catch (e) {
			console.error('Failed to load sessions:', e);
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Sessions - Cairn</title>
</svelte:head>

<div class="page-container">
	<header class="page-header">
		<div class="flex items-center gap-2">
			<MonitorPlay size={20} class="text-[var(--cairn-accent)]" />
			<h1 class="text-lg font-semibold">Coding Sessions</h1>
		</div>
		<p class="text-sm text-[var(--text-tertiary)] mt-1">
			Watch agent sessions in real-time, review past work, and steer active sessions.
		</p>
	</header>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 size={24} class="animate-spin text-[var(--text-tertiary)]" />
		</div>
	{:else if sessions.length === 0}
		<div class="empty-state">
			<MonitorPlay size={32} class="text-[var(--text-tertiary)]" />
			<p class="text-sm text-[var(--text-tertiary)] mt-2">No sessions yet</p>
			<p class="text-xs text-[var(--text-tertiary)]">Sessions appear when the agent processes tasks via chat or idle mode.</p>
		</div>
	{:else}
		<div class="session-list">
			{#each sessions as session (session.id)}
				<a href="/session/{session.id}" class="session-card">
					<div class="session-card-header">
						<span class="session-title">{session.title || 'Untitled session'}</span>
						<ChevronRight size={14} class="text-[var(--text-tertiary)] shrink-0" />
					</div>
					<div class="session-card-meta">
						<span class="meta-item">
							<MessageSquare size={12} />
							{session.messageCount} messages
						</span>
						<span class="meta-item">
							<Clock size={12} />
							{relativeTime(session.updatedAt ?? session.createdAt)}
						</span>
					</div>
					<div class="session-card-footer">
						<Badge variant="outline" class="text-xs font-mono">{session.id.slice(0, 12)}</Badge>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page-container {
		padding: 1.5rem;
		max-width: 800px;
		margin: 0 auto;
	}
	.page-header {
		margin-bottom: 1.5rem;
	}
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 3rem;
		text-align: center;
	}
	.session-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.session-card {
		display: block;
		padding: 0.75rem 1rem;
		border: 1px solid hsl(var(--border));
		border-radius: 0.5rem;
		transition: background 0.15s, border-color 0.15s;
		text-decoration: none;
		color: inherit;
	}
	.session-card:hover {
		background: hsl(var(--muted) / 0.3);
		border-color: var(--cairn-accent, #60a5fa);
	}
	.session-card-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.session-title {
		font-size: 0.875rem;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.session-card-meta {
		display: flex;
		gap: 0.75rem;
		margin-top: 0.375rem;
	}
	.meta-item {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--text-tertiary, hsl(var(--muted-foreground)));
	}
	.session-card-footer {
		margin-top: 0.375rem;
	}
</style>
