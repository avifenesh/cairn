<script lang="ts">
	import { Wrench } from '@lucide/svelte';

	let { toolName, phase }: { toolName: string; phase: string } = $props();
</script>

<span class="inline-flex items-center gap-1 rounded-full bg-[var(--bg-3)] px-2 py-0.5 text-[10px] text-[var(--text-tertiary)]">
	<Wrench class="h-2.5 w-2.5" />
	{toolName}
	{#if phase === 'start'}
		<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--pub-accent)]"></span>
	{/if}
</span>
