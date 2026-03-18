<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Copy, Check } from '@lucide/svelte';

	let { code, lang = '' }: { code: string; lang?: string } = $props();
	let copied = $state(false);

	async function handleCopy() {
		await navigator.clipboard.writeText(code);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}
</script>

<div class="group relative rounded-lg border border-border-subtle overflow-hidden">
	{#if lang}
		<div class="flex items-center justify-between border-b border-border-subtle bg-[var(--bg-2)] px-3 py-1.5">
			<Badge variant="secondary" class="h-4 px-1.5 text-[10px] font-mono">{lang}</Badge>
			<Button
				variant="ghost"
				size="icon"
				class="h-6 w-6"
				onclick={handleCopy}
				aria-label="Copy code"
			>
				{#if copied}
					<Check class="h-3 w-3 text-[var(--color-success)]" />
				{:else}
					<Copy class="h-3 w-3 text-[var(--text-tertiary)]" />
				{/if}
			</Button>
		</div>
	{/if}
	<pre class="overflow-x-auto bg-[var(--bg-1)] p-3 text-sm"><code class="font-mono text-[var(--text-primary)]">{code}</code></pre>
	{#if !lang}
		<Button
			variant="ghost"
			size="icon"
			class="absolute right-2 top-2 h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity"
			onclick={handleCopy}
			aria-label="Copy code"
		>
			{#if copied}
				<Check class="h-3 w-3 text-[var(--color-success)]" />
			{:else}
				<Copy class="h-3 w-3 text-[var(--text-tertiary)]" />
			{/if}
		</Button>
	{/if}
</div>
