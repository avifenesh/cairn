<script lang="ts">
	import type { ReasoningStep } from '$lib/types';
	import { Brain, ChevronRight } from '@lucide/svelte';

	let {
		steps,
		isStreaming = false,
	}: {
		steps: ReasoningStep[];
		isStreaming?: boolean;
	} = $props();

	let expanded = $state(false);
	let wasStreaming = $state(false);

	// Auto-expand during streaming, auto-collapse when done
	$effect(() => {
		if (isStreaming && steps.length > 0 && !wasStreaming) {
			expanded = true;
			wasStreaming = true;
		} else if (!isStreaming && wasStreaming) {
			expanded = false;
			wasStreaming = false;
		}
	});

	const totalText = $derived(steps.map((s) => s.thought).join(' '));
	const wordCount = $derived(totalText.split(/\s+/).filter(Boolean).length);
</script>

<div class="mb-2 rounded-lg border border-border-subtle bg-[var(--bg-0)]/50 overflow-hidden">
	<button
		class="flex w-full items-center gap-2 px-3 py-2 text-left text-[11px] transition-colors hover:bg-[var(--bg-2)]/50"
		onclick={() => { expanded = !expanded; }}
		aria-expanded={expanded}
		aria-controls="reasoning-content"
		type="button"
	>
		<Brain class="h-3.5 w-3.5 text-[var(--cairn-accent)] flex-shrink-0" />
		{#if isStreaming}
			<span class="text-[var(--text-secondary)]">
				Thinking
				<span class="thinking-dots inline-flex gap-0.5 ml-0.5">
					<span class="inline-block h-1 w-1 rounded-full bg-[var(--cairn-accent)]"></span>
					<span class="inline-block h-1 w-1 rounded-full bg-[var(--cairn-accent)]"></span>
					<span class="inline-block h-1 w-1 rounded-full bg-[var(--cairn-accent)]"></span>
				</span>
			</span>
		{:else}
			<span class="text-[var(--text-tertiary)]">
				Thought ({steps.length} step{steps.length !== 1 ? 's' : ''}, ~{wordCount} words)
			</span>
		{/if}
		<ChevronRight class="h-3 w-3 ml-auto flex-shrink-0 text-[var(--text-tertiary)] transition-transform {expanded ? 'rotate-90' : ''}" />
	</button>

	{#if expanded}
		<div
			id="reasoning-content"
			class="border-t border-border-subtle px-3 py-2 text-xs text-[var(--text-secondary)] leading-relaxed max-h-64 overflow-y-auto"
			aria-hidden={!expanded}
		>
			{#each steps as step}
				<p class="mb-1.5 last:mb-0">
					{#if steps.length > 1}
						<span class="font-mono text-[var(--cairn-accent)] text-[10px] mr-1">R{step.round}</span>
					{/if}
					{step.thought}
				</p>
			{/each}
			{#if isStreaming}
				<span class="inline-block h-3 w-0.5 animate-pulse bg-[var(--cairn-accent)] ml-0.5 rounded-full"></span>
			{/if}
		</div>
	{/if}
</div>
