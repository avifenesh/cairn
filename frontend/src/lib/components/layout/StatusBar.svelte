<script lang="ts">
	import { appStore } from '$lib/stores/app.svelte';
	import { Separator } from '$lib/components/ui/separator';
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

<footer class="hidden md:flex h-6 items-center justify-between border-t border-border-subtle bg-[var(--bg-1)] px-4 text-[10px] font-mono text-[var(--text-tertiary)]">
	<div class="flex items-center gap-3">
		<span class="flex items-center gap-1.5">
			<Circle
				class="h-1.5 w-1.5 fill-current {appStore.sseConnected
					? 'text-[var(--color-success)] animate-pulse-dot'
					: 'text-[var(--color-error)]'}"
			/>
			{appStore.sseConnected ? 'connected' : 'disconnected'}
		</span>
		{#if agentCount > 0}
			<Separator orientation="vertical" class="h-3" />
			<span>{agentCount} agent{agentCount !== 1 ? 's' : ''}</span>
		{/if}
	</div>
	<div class="flex items-center gap-3">
		{#if lastPoll()}
			<span>polled {lastPoll()}</span>
		{/if}
		<span class="text-[var(--text-tertiary)]/50">cairn</span>
	</div>
</footer>
