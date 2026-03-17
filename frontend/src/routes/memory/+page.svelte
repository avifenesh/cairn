<script lang="ts">
	import { onMount } from 'svelte';
	import { getMemories, searchMemories, acceptMemory, rejectMemory, createMemory } from '$lib/api/client';
	import { memoryStore } from '$lib/stores/memory.svelte';
	import MemoryCard from '$lib/components/memory/MemoryCard.svelte';
	import MemorySearch from '$lib/components/memory/MemorySearch.svelte';
	import MemoryEditor from '$lib/components/memory/MemoryEditor.svelte';
	import { Brain } from '@lucide/svelte';

	let filter = $state<'all' | 'proposed' | 'accepted'>('all');

	onMount(async () => {
		memoryStore.setLoading(true);
		try {
			const res = await getMemories();
			memoryStore.setMemories(res.items);
		} catch {
			// handled
		} finally {
			memoryStore.setLoading(false);
		}
	});

	async function handleSearch() {
		if (!memoryStore.searchQuery.trim()) {
			memoryStore.setSearchResults([]);
			return;
		}
		memoryStore.setLoading(true);
		try {
			const res = await searchMemories(memoryStore.searchQuery);
			memoryStore.setSearchResults(res.items);
		} catch {
			// handled
		} finally {
			memoryStore.setLoading(false);
		}
	}

	async function handleAccept(id: string) {
		memoryStore.resolveMemory(id, 'accepted');
		await acceptMemory(id);
	}

	async function handleReject(id: string) {
		memoryStore.resolveMemory(id, 'rejected');
		await rejectMemory(id);
	}

	async function handleCreate(content: string, category: string) {
		try {
			const memory = await createMemory(content, category);
			memoryStore.setMemories([memory, ...memoryStore.memories]);
		} catch {
			// handled
		}
	}

	const displayMemories = $derived(() => {
		const source = memoryStore.searchQuery.trim()
			? memoryStore.searchResults
			: memoryStore.memories;
		if (filter === 'all') return source;
		return source.filter((m) => m.status === filter);
	});
</script>

<div class="mx-auto max-w-4xl p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold text-[var(--text-primary)]">Memory</h1>
		<MemoryEditor oncreate={handleCreate} />
	</div>

	<!-- Search -->
	<div class="mb-4 flex gap-2">
		<MemorySearch bind:value={memoryStore.searchQuery} onsearch={handleSearch} />
	</div>

	<!-- Filter tabs -->
	<div class="mb-4 flex gap-1">
		{#each ['all', 'proposed', 'accepted'] as f}
			<button
				class="rounded-md px-3 py-1.5 text-xs transition-colors duration-[var(--dur-fast)]
					{filter === f
					? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
					: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
				onclick={() => (filter = f as typeof filter)}
			>
				{f.charAt(0).toUpperCase() + f.slice(1)}
				{#if f === 'proposed' && memoryStore.proposedCount > 0}
					<span class="ml-1 text-[10px]">({memoryStore.proposedCount})</span>
				{/if}
			</button>
		{/each}
	</div>

	{#if memoryStore.loading}
		<div class="flex flex-col gap-3">
			{#each Array(5) as _}
				<div class="h-20 animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
			{/each}
		</div>
	{:else if displayMemories().length === 0}
		<div class="flex flex-col items-center justify-center py-16 text-[var(--text-tertiary)]">
			<Brain class="mb-3 h-10 w-10 opacity-40" />
			<p class="text-sm">No memories found</p>
		</div>
	{:else}
		<div class="flex flex-col gap-3">
			{#each displayMemories() as memory (memory.id)}
				<MemoryCard {memory} onaccept={handleAccept} onreject={handleReject} />
			{/each}
		</div>
	{/if}
</div>
