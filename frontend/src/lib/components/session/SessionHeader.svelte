<script lang="ts">
	import type { SessionStatus } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Activity, Cpu, Hash, AlertCircle } from '@lucide/svelte';

	let { sessionId, title = '', status, currentRound, totalToolCalls, totalErrors, totalTokensIn, totalTokensOut }: {
		sessionId: string;
		title?: string;
		status: SessionStatus;
		currentRound: number;
		totalToolCalls: number;
		totalErrors: number;
		totalTokensIn: number;
		totalTokensOut: number;
	} = $props();

	const statusColor: Record<SessionStatus, string> = {
		running: 'bg-green-500',
		paused: 'bg-amber-500',
		waiting_approval: 'bg-amber-500',
		completed: 'bg-blue-500',
		failed: 'bg-red-500',
		stopped: 'bg-gray-500',
	};

	function formatTokens(n: number): string {
		if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
		return String(n);
	}
</script>

<header class="session-header">
	<div class="header-left">
		<div class="status-dot {statusColor[status] ?? 'bg-gray-500'}"></div>
		<span class="session-title">{title || 'Session'}</span>
		<Badge variant="outline" class="text-xs font-mono">{sessionId.slice(0, 8)}</Badge>
		<Badge variant={status === 'running' ? 'default' : status === 'failed' ? 'destructive' : 'secondary'} class="text-xs">
			{status}
		</Badge>
	</div>
	<div class="header-right">
		<div class="stat" title="Round">
			<Hash size={12} />
			<span>{currentRound + 1}</span>
		</div>
		<div class="stat" title="Tool calls">
			<Activity size={12} />
			<span>{totalToolCalls}</span>
		</div>
		{#if totalErrors > 0}
			<div class="stat stat--error" title="Errors">
				<AlertCircle size={12} />
				<span>{totalErrors}</span>
			</div>
		{/if}
		<div class="stat" title="Tokens (in/out)">
			<Cpu size={12} />
			<span>{formatTokens(totalTokensIn)}/{formatTokens(totalTokensOut)}</span>
		</div>
	</div>
</header>

<style>
	.session-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0.75rem;
		border-bottom: 1px solid hsl(var(--border));
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.header-left {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.header-right {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.status-dot {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 9999px;
		flex-shrink: 0;
	}
	.session-title {
		font-weight: 600;
		font-size: 0.875rem;
	}
	.stat {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--text-tertiary, hsl(var(--muted-foreground)));
	}
	.stat--error {
		color: hsl(var(--destructive));
	}

	@media (max-width: 640px) {
		.session-header {
			padding: 0.375rem 0.5rem;
		}
		.header-right {
			gap: 0.5rem;
			font-size: 0.6875rem;
		}
	}
</style>
