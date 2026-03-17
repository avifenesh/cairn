<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import type { ChatMode } from '$lib/types';

	const modes: { value: ChatMode; label: string }[] = [
		{ value: 'talk', label: 'Talk' },
		{ value: 'work', label: 'Work' },
		{ value: 'coding', label: 'Coding' },
	];
</script>

<div class="mb-2 flex gap-1">
	{#each modes as m}
		<button
			class="rounded-md px-2.5 py-1 text-xs transition-colors duration-[var(--dur-fast)]
				{chatStore.mode === m.value
				? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
				: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
			onclick={() => chatStore.setMode(m.value)}
		>
			{m.label}
		</button>
	{/each}
</div>
