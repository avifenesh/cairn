<script lang="ts">
	import type { ReasoningStep } from '$lib/types';
	import { Brain } from '@lucide/svelte';

	let { steps }: { steps: ReasoningStep[] } = $props();
</script>

<details class="mb-2">
	<summary class="flex cursor-pointer items-center gap-1 text-[10px] text-[var(--text-tertiary)]">
		<Brain class="h-2.5 w-2.5" />
		{steps.length} reasoning step{steps.length !== 1 ? 's' : ''}
	</summary>
	<div class="mt-1 border-l-2 border-border-subtle pl-2 text-xs text-[var(--text-secondary)]">
		{#each steps as step}
			<p class="mb-1"><strong>Round {step.round}:</strong> {step.thought}</p>
		{/each}
	</div>
</details>
