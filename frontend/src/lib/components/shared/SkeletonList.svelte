<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';

	let { count = 3, height = 'h-20' }: { count?: number; height?: string } = $props();
</script>

<div class="flex flex-col gap-3">
	{#each Array(count) as _, i}
		<Skeleton class="{height} w-full rounded-lg animate-in" style="animation-delay: {i * 50}ms" />
	{/each}
</div>
