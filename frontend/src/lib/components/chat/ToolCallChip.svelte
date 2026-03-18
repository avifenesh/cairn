<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Wrench, Check } from '@lucide/svelte';

	let { toolName, phase }: { toolName: string; phase: string } = $props();
</script>

<Badge variant="outline" class="h-5 gap-1 px-1.5 text-[10px] font-mono {phase === 'start' ? 'border-[var(--cairn-accent)]/30 text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}">
	{#if phase === 'start'}
		<Wrench class="h-2.5 w-2.5 animate-spin" />
	{:else}
		<Check class="h-2.5 w-2.5" />
	{/if}
	{toolName}
</Badge>
