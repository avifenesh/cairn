<script lang="ts">
	import { Copy, Check } from '@lucide/svelte';

	let { code, lang = '' }: { code: string; lang?: string } = $props();
	let copied = $state(false);

	async function handleCopy() {
		await navigator.clipboard.writeText(code);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}
</script>

<div class="group relative">
	<pre class="overflow-x-auto rounded-md bg-[var(--bg-3)] p-3 text-sm"><code class="font-mono text-[var(--text-primary)]">{code}</code></pre>
	<button
		class="absolute right-2 top-2 rounded-md bg-[var(--bg-4)] p-1.5 text-[var(--text-tertiary)] opacity-0 transition-opacity group-hover:opacity-100 hover:text-[var(--text-primary)]"
		onclick={handleCopy}
		aria-label="Copy code"
	>
		{#if copied}
			<Check class="h-3.5 w-3.5 text-[var(--color-success)]" />
		{:else}
			<Copy class="h-3.5 w-3.5" />
		{/if}
	</button>
	{#if lang}
		<span class="absolute right-2 bottom-2 text-[10px] text-[var(--text-tertiary)] opacity-0 group-hover:opacity-100 transition-opacity">
			{lang}
		</span>
	{/if}
</div>
