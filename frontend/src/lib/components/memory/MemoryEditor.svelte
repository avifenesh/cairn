<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Plus, X } from '@lucide/svelte';

	let { oncreate }: { oncreate: (content: string, category: string) => void } = $props();

	let open = $state(false);
	let content = $state('');
	let category = $state('fact');

	const categories = [
		{ value: 'fact', label: 'Fact' },
		{ value: 'preference', label: 'Preference' },
		{ value: 'hard_rule', label: 'Hard Rule' },
		{ value: 'decision', label: 'Decision' },
		{ value: 'writing_style', label: 'Writing Style' },
	];

	function handleCreate() {
		const text = content.trim();
		if (!text) return;
		oncreate(text, category);
		content = '';
		category = 'fact';
		open = false;
	}
</script>

{#if !open}
	<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={() => (open = true)}>
		<Plus class="h-3.5 w-3.5" /> New memory
	</Button>
{:else}
	<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<span class="text-sm font-medium text-[var(--text-primary)]">Create Memory</span>
			<Button variant="ghost" size="icon" class="h-7 w-7" onclick={() => (open = false)}>
				<X class="h-4 w-4 text-[var(--text-tertiary)]" />
			</Button>
		</div>
		<textarea
			bind:value={content}
			placeholder="What should I remember?"
			rows="3"
			class="mb-3 w-full resize-none rounded-lg border border-border-subtle bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none transition-colors"
		></textarea>
		<div class="flex items-center gap-2">
			<select
				bind:value={category}
				aria-label="Memory category"
				class="rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs text-[var(--text-secondary)] focus:border-[var(--cairn-accent)] focus:outline-none transition-colors"
			>
				{#each categories as cat}
					<option value={cat.value}>{cat.label}</option>
				{/each}
			</select>
			<Button size="sm" class="h-7 text-xs" onclick={handleCreate} disabled={!content.trim()}>
				Create
			</Button>
		</div>
	</div>
{/if}
