<script lang="ts">
	import type { ReasoningStep } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Brain } from '@lucide/svelte';

	let { steps }: { steps: ReasoningStep[] } = $props();
</script>

<details class="mb-2 group">
	<summary class="flex cursor-pointer items-center gap-1.5 text-[10px] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] transition-colors">
		<Brain class="h-3 w-3" />
		<span>{steps.length} reasoning step{steps.length !== 1 ? 's' : ''}</span>
		<Badge variant="outline" class="h-3.5 px-1 text-[9px]">expand</Badge>
	</summary>
	<div class="mt-2 border-l-2 border-[var(--cairn-accent)]/20 pl-3 text-xs text-[var(--text-secondary)] space-y-1.5">
		{#each steps as step}
			<p><span class="font-mono text-[var(--cairn-accent)]">R{step.round}</span> {step.thought}</p>
		{/each}
	</div>
</details>
