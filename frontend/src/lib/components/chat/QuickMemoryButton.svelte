<script lang="ts">
	import { createMemory } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { BookmarkPlus, Check, AlertCircle } from '@lucide/svelte';

	let { content }: { content: string } = $props();

	let phase = $state<'idle' | 'picking' | 'saving' | 'done' | 'error'>('idle');
	let category = $state('fact');

	const categories = [
		{ value: 'fact', label: 'Fact' },
		{ value: 'preference', label: 'Preference' },
		{ value: 'hard_rule', label: 'Hard Rule' },
		{ value: 'decision', label: 'Decision' },
	];

	async function save() {
		phase = 'saving';
		try {
			await createMemory(content, category);
			phase = 'done';
			setTimeout(() => { phase = 'idle'; }, 2000);
		} catch {
			phase = 'error';
			setTimeout(() => { phase = 'idle'; }, 3000);
		}
	}
</script>

{#if phase === 'idle'}
	<Button
		variant="ghost"
		size="icon"
		class="h-6 w-6"
		onclick={() => { phase = 'picking'; }}
		aria-label="Remember this"
	>
		<BookmarkPlus class="h-3 w-3 text-[var(--text-tertiary)]" />
	</Button>
{:else if phase === 'picking'}
	<div class="flex items-center gap-1">
		<select
			bind:value={category}
			aria-label="Memory category"
			class="h-6 rounded border border-border-subtle bg-[var(--bg-0)] px-1.5 text-[10px] text-[var(--text-secondary)] focus:outline-none"
		>
			{#each categories as cat}
				<option value={cat.value}>{cat.label}</option>
			{/each}
		</select>
		<Button variant="ghost" size="icon" class="h-6 w-6" onclick={save} aria-label="Save memory">
			<Check class="h-3 w-3 text-[var(--cairn-accent)]" />
		</Button>
	</div>
{:else if phase === 'saving'}
	<span class="text-[10px] text-[var(--text-tertiary)]">Saving...</span>
{:else if phase === 'done'}
	<span class="flex items-center gap-1 text-[10px] text-[var(--color-success)]">
		<Check class="h-3 w-3" /> Saved
	</span>
{:else if phase === 'error'}
	<span class="flex items-center gap-1 text-[10px] text-[var(--color-error)]">
		<AlertCircle class="h-3 w-3" /> Failed
	</span>
{/if}
