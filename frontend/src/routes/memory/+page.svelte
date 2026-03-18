<script lang="ts">
	import { onMount } from 'svelte';
	import { getMemories, searchMemories, acceptMemory, rejectMemory, createMemory } from '$lib/api/client';
	import { memoryStore } from '$lib/stores/memory.svelte';
	import MemoryCard from '$lib/components/memory/MemoryCard.svelte';
	import MemorySearch from '$lib/components/memory/MemorySearch.svelte';
	import MemoryEditor from '$lib/components/memory/MemoryEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Brain, Check, X, Square, CheckSquare } from '@lucide/svelte';

	let filter = $state<'all' | 'proposed' | 'accepted'>('all');
	let selectedIds = $state<Set<string>>(new Set());

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
		selectedIds.delete(id);
		selectedIds = new Set(selectedIds);
		await acceptMemory(id);
	}

	async function handleReject(id: string) {
		memoryStore.resolveMemory(id, 'rejected');
		selectedIds.delete(id);
		selectedIds = new Set(selectedIds);
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

	function toggleSelect(id: string) {
		if (selectedIds.has(id)) {
			selectedIds.delete(id);
		} else {
			selectedIds.add(id);
		}
		selectedIds = new Set(selectedIds);
	}

	async function bulkAccept() {
		const ids = [...selectedIds];
		ids.forEach((id) => memoryStore.resolveMemory(id, 'accepted'));
		selectedIds = new Set();
		await Promise.all(ids.map((id) => acceptMemory(id)));
	}

	async function bulkReject() {
		const ids = [...selectedIds];
		ids.forEach((id) => memoryStore.resolveMemory(id, 'rejected'));
		selectedIds = new Set();
		await Promise.all(ids.map((id) => rejectMemory(id)));
	}

	const displayMemories = $derived(() => {
		const source = memoryStore.searchQuery.trim()
			? memoryStore.searchResults
			: memoryStore.memories;
		if (filter === 'all') return source;
		return source.filter((m) => m.status === filter);
	});

	const proposedInView = $derived(
		displayMemories().filter((m) => m.status === 'proposed'),
	);

	const filters: Array<{ key: typeof filter; label: string }> = [
		{ key: 'all', label: 'All' },
		{ key: 'proposed', label: 'Proposed' },
		{ key: 'accepted', label: 'Accepted' },
	];
</script>

<div class="mx-auto max-w-5xl p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Memory</h1>
		<MemoryEditor oncreate={handleCreate} />
	</div>

	<div class="mb-4 flex gap-2">
		<MemorySearch bind:value={memoryStore.searchQuery} onsearch={handleSearch} />
	</div>

	<div class="mb-4 flex items-center gap-1">
		{#each filters as f}
			<Button
				variant={filter === f.key ? 'secondary' : 'ghost'}
				size="sm"
				class="h-7 text-xs gap-1.5
					{filter === f.key ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}"
				onclick={() => (filter = f.key)}
			>
				{f.label}
				{#if f.key === 'proposed' && memoryStore.proposedCount > 0}
					<Badge variant="default" class="h-4 min-w-4 px-1 text-[10px]">{memoryStore.proposedCount}</Badge>
				{/if}
			</Button>
		{/each}
	</div>

	<!-- Batch actions bar -->
	{#if selectedIds.size > 0}
		<div class="mb-4 flex items-center gap-2 rounded-lg border border-border-subtle bg-[var(--bg-1)] px-4 py-2">
			<span class="text-xs text-[var(--text-secondary)] font-medium">{selectedIds.size} selected</span>
			<Button
				variant="outline"
				size="sm"
				class="h-7 text-xs gap-1 border-[var(--color-success)]/30 text-[var(--color-success)] hover:bg-[var(--color-success)]/10"
				onclick={bulkAccept}
			>
				<Check class="h-3 w-3" /> Accept all
			</Button>
			<Button
				variant="outline"
				size="sm"
				class="h-7 text-xs gap-1 border-[var(--color-error)]/30 text-[var(--color-error)] hover:bg-[var(--color-error)]/10"
				onclick={bulkReject}
			>
				<X class="h-3 w-3" /> Reject all
			</Button>
			<Button
				variant="ghost"
				size="sm"
				class="h-7 text-xs ml-auto"
				onclick={() => (selectedIds = new Set())}
			>
				Clear
			</Button>
		</div>
	{/if}

	{#if memoryStore.loading}
		<div class="flex flex-col gap-3">
			{#each Array(5) as _, i}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 animate-in" style="animation-delay: {i * 40}ms">
					<Skeleton class="h-4 w-64 mb-2" />
					<Skeleton class="h-3 w-32" />
				</div>
			{/each}
		</div>
	{:else if displayMemories().length === 0}
		<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
			<Brain class="mb-3 h-10 w-10 opacity-30" />
			<p class="text-sm">No memories found</p>
			<p class="mt-1 text-xs opacity-60">Memories appear as the agent learns from your interactions</p>
		</div>
	{:else}
		<div class="flex flex-col gap-3">
			{#each displayMemories() as memory, i (memory.id)}
				<div class="flex gap-2 animate-in" style="animation-delay: {Math.min(i * 30, 300)}ms">
					{#if memory.status === 'proposed'}
						<button
							class="mt-4 flex-shrink-0 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors"
							onclick={() => toggleSelect(memory.id)}
							type="button"
							aria-label={selectedIds.has(memory.id) ? 'Deselect memory' : 'Select memory'}
						>
							{#if selectedIds.has(memory.id)}
								<CheckSquare class="h-4 w-4 text-[var(--cairn-accent)]" />
							{:else}
								<Square class="h-4 w-4" />
							{/if}
						</button>
					{/if}
					<div class="flex-1">
						<MemoryCard {memory} onaccept={handleAccept} onreject={handleReject} />
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
