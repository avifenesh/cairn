<script lang="ts">
	let { count = 3, height = 'h-20' }: { count?: number; height?: string } = $props();
</script>

<div class="flex flex-col gap-3">
	{#each Array(count) as _}
		<div class="{height} animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
	{/each}
</div>
