<script lang="ts">
	import { AlertTriangle, RefreshCw } from '@lucide/svelte';

	let { error = null, onretry }: { error: string | null; onretry?: () => void } = $props();
</script>

{#if error}
	<div class="mx-auto max-w-md py-16 text-center" role="alert">
		<AlertTriangle class="mx-auto mb-4 h-10 w-10 text-[var(--color-error)] opacity-60" />
		<p class="mb-2 text-sm text-[var(--text-primary)]">Something went wrong</p>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">{error}</p>
		{#if onretry}
			<button
				class="inline-flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors"
				onclick={onretry}
			>
				<RefreshCw class="h-3.5 w-3.5" /> Retry
			</button>
		{/if}
	</div>
{/if}
