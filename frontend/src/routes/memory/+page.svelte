<script lang="ts">
	import { onMount } from 'svelte';
	import { getMemories, searchMemories, acceptMemory, rejectMemory } from '$lib/api/client';
	import { memoryStore } from '$lib/stores/memory.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { Search, Check, X, Brain } from '@lucide/svelte';

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

	const displayMemories = $derived(() => {
		const source = memoryStore.searchQuery.trim()
			? memoryStore.searchResults
			: memoryStore.memories;
		if (filter === 'all') return source;
		return source.filter((m) => m.status === filter);
	});

	const statusColor: Record<string, string> = {
		proposed: 'var(--color-warning)',
		accepted: 'var(--color-success)',
		rejected: 'var(--color-error)',
	};
</script>

<div class="mx-auto max-w-4xl p-6">
	<h1 class="mb-6 text-2xl font-semibold text-[var(--text-primary)]">Memory</h1>

	<!-- Search -->
	<div class="mb-4 flex gap-2">
		<div class="relative flex-1">
			<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-tertiary)]" />
			<input
				type="text"
				placeholder="Search memories..."
				bind:value={memoryStore.searchQuery}
				oninput={() => handleSearch()}
				class="w-full rounded-lg border border-border-subtle bg-[var(--bg-2)] pl-10 pr-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
			/>
		</div>
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
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
					<div class="mb-2 flex items-start justify-between">
						<div class="flex items-center gap-2">
							<span
								class="h-2 w-2 rounded-full"
								style="background: {statusColor[memory.status]}"
							></span>
							<span class="text-xs text-[var(--text-tertiary)]">{memory.category}</span>
							<span class="text-xs text-[var(--text-tertiary)]">&middot;</span>
							<span class="text-xs text-[var(--text-tertiary)]">{memory.status}</span>
						</div>
						<span class="text-xs text-[var(--text-tertiary)]">
							{relativeTime(memory.createdAt)}
						</span>
					</div>
					<p class="text-sm text-[var(--text-primary)]">{memory.content}</p>
					{#if memory.status === 'proposed'}
						<div class="mt-3 flex gap-2">
							<button
								class="flex items-center gap-1 rounded-md bg-[var(--color-success)]/10 px-3 py-1 text-xs font-medium text-[var(--color-success)] hover:bg-[var(--color-success)]/20 transition-colors"
								onclick={() => handleAccept(memory.id)}
							>
								<Check class="h-3 w-3" /> Accept
							</button>
							<button
								class="flex items-center gap-1 rounded-md bg-[var(--color-error)]/10 px-3 py-1 text-xs font-medium text-[var(--color-error)] hover:bg-[var(--color-error)]/20 transition-colors"
								onclick={() => handleReject(memory.id)}
							>
								<X class="h-3 w-3" /> Reject
							</button>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
