<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Alert from '$lib/components/ui/alert';
	import { AlertTriangle, RefreshCw } from '@lucide/svelte';

	let { error = null, onretry }: { error: string | null; onretry?: () => void } = $props();
</script>

{#if error}
	<div class="mx-auto max-w-md py-16 text-center" role="alert">
		<div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-[var(--color-error)]/10">
			<AlertTriangle class="h-6 w-6 text-[var(--color-error)]" />
		</div>
		<p class="mb-1 text-sm font-medium text-[var(--text-primary)]">Something went wrong</p>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">{error}</p>
		{#if onretry}
			<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={onretry}>
				<RefreshCw class="h-3.5 w-3.5" /> Retry
			</Button>
		{/if}
	</div>
{/if}
