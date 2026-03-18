<script lang="ts">
	import { Search } from '@lucide/svelte';
	import { Input } from '$lib/components/ui/input';
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
	<Search class="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--text-tertiary)]" />
	<Input
		type="search"
		placeholder="Search memories..."
		bind:value
		oninput={handleInput}
		class="pl-9 h-9 bg-[var(--bg-0)]"
	/>
</div>
