<script lang="ts">
	import { Search } from '@lucide/svelte';

	let { value = $bindable(''), onsearch }: { value: string; onsearch: () => void } = $props();
</script>

<div class="relative flex-1">
	<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-tertiary)]" />
	<input
		type="text"
		placeholder="Search memories..."
		bind:value
		oninput={onsearch}
		class="w-full rounded-lg border border-border-subtle bg-[var(--bg-2)] pl-10 pr-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
	/>
</div>
