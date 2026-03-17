<script lang="ts">
	import { Search } from '@lucide/svelte';
	import { onDestroy } from 'svelte';

	let { value = $bindable(''), onsearch }: { value: string; onsearch: () => void } = $props();
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	function handleInput() {
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(onsearch, 300);
	}

	onDestroy(() => {
		if (debounceTimer) clearTimeout(debounceTimer);
	});
</script>

<div class="relative flex-1">
	<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-tertiary)]" />
	<input
		type="search"
		placeholder="Search memories..."
		bind:value
		oninput={handleInput}
		class="w-full rounded-lg border border-border-subtle bg-[var(--bg-2)] pl-10 pr-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
	/>
</div>
