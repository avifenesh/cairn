<script lang="ts">
	import { appStore } from '$lib/stores/app.svelte';
	import { Circle } from '@lucide/svelte';

	const lastPoll = $derived(() => {
		const entries = Object.values(appStore.pollStatuses);
		if (entries.length === 0) return null;
		const latest = entries.reduce((a, b) => (a.at > b.at ? a : b));
		const secs = Math.floor((Date.now() - latest.at) / 1000);
		if (secs < 60) return `${secs}s ago`;
		return `${Math.floor(secs / 60)}m ago`;
	});

	const agentCount = $derived(Object.keys(appStore.agentProgresses).length);
</script>

<footer class="hidden md:flex h-6 items-center justify-between border-t border-border-subtle bg-[var(--bg-1)] px-4 text-[11px] text-[var(--text-tertiary)]">
	<div class="flex items-center gap-3">
		<span class="flex items-center gap-1">
			<Circle
				class="h-1.5 w-1.5 fill-current {appStore.sseConnected
					? 'text-[var(--color-success)]'
					: 'text-[var(--color-error)]'}"
			/>
			SSE {appStore.sseConnected ? 'connected' : 'disconnected'}
		</span>
		{#if agentCount > 0}
			<span>{agentCount} agent{agentCount !== 1 ? 's' : ''}</span>
		{/if}
	</div>
	<div>
		{#if lastPoll()}
			<span>Polled {lastPoll()}</span>
		{/if}
	</div>
</footer>
