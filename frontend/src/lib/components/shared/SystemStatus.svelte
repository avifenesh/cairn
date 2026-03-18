<script lang="ts">
	import { onMount } from 'svelte';
	import { getStatus, getCosts } from '$lib/api/client';
	import { appStore } from '$lib/stores/app.svelte';
	import { Activity, DollarSign, Clock, Wifi, WifiOff } from '@lucide/svelte';

	let status = $state<Record<string, unknown> | null>(null);
	let costs = $state<Record<string, number> | null>(null);
	let error = $state(false);

	onMount(async () => {
		try {
			const [s, c] = await Promise.all([getStatus(), getCosts()]);
			status = s;
			costs = c as unknown as Record<string, number>;
		} catch {
			error = true;
		}
	});

	const sseConnected = $derived(appStore.sseConnected);
</script>

<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
	<div class="flex items-center gap-2 mb-3">
		<Activity class="h-4 w-4 text-[var(--cairn-accent)]" />
		<h3 class="text-sm font-medium text-[var(--text-primary)]">System Status</h3>
		{#if sseConnected}
			<span class="ml-auto flex items-center gap-1 text-[10px] text-[var(--color-success)]">
				<Wifi class="h-3 w-3" /> Connected
			</span>
		{:else}
			<span class="ml-auto flex items-center gap-1 text-[10px] text-[var(--color-error)]">
				<WifiOff class="h-3 w-3" /> Disconnected
			</span>
		{/if}
	</div>

	{#if error}
		<p class="text-xs text-[var(--text-tertiary)]">Unable to load status</p>
	{:else}
		<div class="grid grid-cols-2 gap-3">
			<!-- Uptime -->
			<div class="flex items-start gap-2">
				<Clock class="h-3.5 w-3.5 mt-0.5 text-[var(--text-tertiary)]" />
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Uptime</p>
					<p class="text-xs text-[var(--text-primary)] font-mono">
						{status?.uptime ?? '...'}
					</p>
				</div>
			</div>

			<!-- Version -->
			<div class="flex items-start gap-2">
				<Activity class="h-3.5 w-3.5 mt-0.5 text-[var(--text-tertiary)]" />
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Version</p>
					<p class="text-xs text-[var(--text-primary)] font-mono">
						{status?.version ?? '...'}
					</p>
				</div>
			</div>

			<!-- Cost Today -->
			<div class="flex items-start gap-2">
				<DollarSign class="h-3.5 w-3.5 mt-0.5 text-[var(--text-tertiary)]" />
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Today</p>
					<p class="text-xs text-[var(--text-primary)] font-mono">
						${(costs?.todayUsd ?? costs?.today ?? 0).toFixed(4)}
					</p>
				</div>
			</div>

			<!-- Cost This Week -->
			<div class="flex items-start gap-2">
				<DollarSign class="h-3.5 w-3.5 mt-0.5 text-[var(--text-tertiary)]" />
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">This Week</p>
					<p class="text-xs text-[var(--text-primary)] font-mono">
						${(costs?.weekUsd ?? costs?.thisMonth ?? 0).toFixed(4)}
					</p>
				</div>
			</div>
		</div>
	{/if}
</div>
