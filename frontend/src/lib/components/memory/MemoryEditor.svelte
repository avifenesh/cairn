<script lang="ts">
	import { Plus, X } from '@lucide/svelte';

	let { oncreate }: { oncreate: (content: string, category: string) => void } = $props();

	let open = $state(false);
	let content = $state('');
	let category = $state('general');

	function handleCreate() {
		const text = content.trim();
		if (!text) return;
		oncreate(text, category);
		content = '';
		category = 'general';
		open = false;
	}
</script>

{#if !open}
	<button
		class="flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-2 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors"
		onclick={() => (open = true)}
	>
		<Plus class="h-3.5 w-3.5" /> New memory
	</button>
{:else}
	<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<span class="text-sm font-medium text-[var(--text-primary)]">Create Memory</span>
			<button
				class="rounded p-1 hover:bg-[var(--bg-3)]"
				onclick={() => (open = false)}
			>
				<X class="h-4 w-4 text-[var(--text-tertiary)]" />
			</button>
		</div>
		<textarea
			bind:value={content}
			placeholder="What should I remember?"
			rows="3"
			class="mb-2 w-full resize-none rounded-lg border border-border-subtle bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
		></textarea>
		<div class="flex items-center gap-2">
			<select
				bind:value={category}
				class="rounded-md border border-border-subtle bg-[var(--bg-2)] px-2 py-1 text-xs text-[var(--text-secondary)]"
			>
				<option value="general">General</option>
				<option value="preference">Preference</option>
				<option value="project">Project</option>
				<option value="person">Person</option>
				<option value="process">Process</option>
			</select>
			<button
				class="rounded-md bg-[var(--pub-accent)] px-3 py-1 text-xs font-medium text-[var(--primary-foreground)] hover:opacity-90 transition-opacity disabled:opacity-50"
				onclick={handleCreate}
				disabled={!content.trim()}
			>
				Create
			</button>
		</div>
	</div>
{/if}
