<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { TASK_TYPES, TASK_PRIORITIES } from '$lib/constants';
	import { Plus, X } from '@lucide/svelte';

	let { oncreate }: { oncreate: (description: string, type: string, priority: number) => void } = $props();

	let open = $state(false);
	let description = $state('');
	let type = $state('general');
	let priority = $state(2);

	function handleCreate() {
		const text = description.trim();
		if (!text) return;
		oncreate(text, type, priority);
		description = '';
		type = 'general';
		priority = 2;
		open = false;
	}
</script>

{#if !open}
	<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={() => (open = true)}>
		<Plus class="h-3.5 w-3.5" /> New task
	</Button>
{:else}
	<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 mb-4">
		<div class="mb-3 flex items-center justify-between">
			<span class="text-sm font-medium text-[var(--text-primary)]">Create Task</span>
			<Button variant="ghost" size="icon" class="h-7 w-7" onclick={() => (open = false)}>
				<X class="h-4 w-4 text-[var(--text-tertiary)]" />
			</Button>
		</div>
		<textarea
			bind:value={description}
			placeholder="What needs to be done?"
			rows="2"
			class="mb-3 w-full resize-none rounded-lg border border-border-subtle bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none transition-colors"
		></textarea>
		<div class="flex items-center gap-2">
			<select
				bind:value={type}
				aria-label="Task type"
				class="rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs text-[var(--text-secondary)] focus:border-[var(--cairn-accent)] focus:outline-none transition-colors"
			>
				{#each TASK_TYPES as t}
					<option value={t.value}>{t.label}</option>
				{/each}
			</select>
			<select
				bind:value={priority}
				aria-label="Task priority"
				class="rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs text-[var(--text-secondary)] focus:border-[var(--cairn-accent)] focus:outline-none transition-colors"
			>
				{#each TASK_PRIORITIES as p}
					<option value={p.value}>{p.label}</option>
				{/each}
			</select>
			<Button size="sm" class="h-7 text-xs" onclick={handleCreate} disabled={!description.trim()}>
				Create
			</Button>
		</div>
	</div>
{/if}
