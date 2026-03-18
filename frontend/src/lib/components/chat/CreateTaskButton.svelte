<script lang="ts">
	import { createTask } from '$lib/api/client';
	import { TASK_PRIORITIES } from '$lib/constants';
	import { Button } from '$lib/components/ui/button';
	import { ListPlus, Check, AlertCircle } from '@lucide/svelte';

	let { content }: { content: string } = $props();

	let phase = $state<'idle' | 'picking' | 'saving' | 'done' | 'error'>('idle');
	let priority = $state(2);

	async function save() {
		phase = 'saving';
		try {
			await createTask(content.slice(0, 500), 'general', priority);
			phase = 'done';
			setTimeout(() => { phase = 'idle'; }, 2000);
		} catch (err) {
			console.error('[task] Failed to create:', err);
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
		aria-label="Create task"
	>
		<ListPlus class="h-3 w-3 text-[var(--text-tertiary)]" />
	</Button>
{:else if phase === 'picking'}
	<div class="flex items-center gap-1">
		<select
			bind:value={priority}
			aria-label="Task priority"
			class="h-6 rounded border border-border-subtle bg-[var(--bg-0)] px-1.5 text-[10px] text-[var(--text-secondary)] focus:outline-none"
		>
			{#each TASK_PRIORITIES as p}
				<option value={p.value}>{p.label}</option>
			{/each}
		</select>
		<Button variant="ghost" size="icon" class="h-6 w-6" onclick={save} aria-label="Save task">
			<Check class="h-3 w-3 text-[var(--cairn-accent)]" />
		</Button>
	</div>
{:else if phase === 'saving'}
	<span class="text-[10px] text-[var(--text-tertiary)]">Creating...</span>
{:else if phase === 'done'}
	<span class="flex items-center gap-1 text-[10px] text-[var(--color-success)]">
		<Check class="h-3 w-3" /> Created
	</span>
{:else if phase === 'error'}
	<span class="flex items-center gap-1 text-[10px] text-[var(--color-error)]">
		<AlertCircle class="h-3 w-3" /> Failed
	</span>
{/if}
