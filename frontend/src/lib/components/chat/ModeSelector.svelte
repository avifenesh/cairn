<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import type { ChatMode } from '$lib/types';
	import { Plus, X } from '@lucide/svelte';

	interface CustomMode {
		name: string;
		description: string;
		promptInjection: string;
	}

	const builtinModes: { value: ChatMode; label: string }[] = [
		{ value: 'talk', label: 'Talk' },
		{ value: 'work', label: 'Work' },
		{ value: 'coding', label: 'Coding' },
	];

	let customModes = $state<CustomMode[]>(loadCustomModes());
	let activeCustomMode = $state<string | null>(null);
	let showAdd = $state(false);
	let newName = $state('');
	let newDesc = $state('');
	let newPrompt = $state('');

	function loadCustomModes(): CustomMode[] {
		try {
			return JSON.parse(localStorage.getItem('pub_custom_modes') || '[]');
		} catch {
			return [];
		}
	}

	function saveCustomModes() {
		try {
			localStorage.setItem('pub_custom_modes', JSON.stringify(customModes));
		} catch {
			// storage full or unavailable
		}
	}

	function selectBuiltin(mode: ChatMode) {
		chatStore.setMode(mode);
		activeCustomMode = null;
	}

	function selectCustom(mode: CustomMode) {
		// Custom modes use 'talk' as the backend mode, prompt injection is client-side
		chatStore.setMode('talk');
		activeCustomMode = mode.name;
	}

	function addMode() {
		const name = newName.trim();
		if (!name) return;
		customModes = [...customModes, { name, description: newDesc, promptInjection: newPrompt }];
		saveCustomModes();
		newName = '';
		newDesc = '';
		newPrompt = '';
		showAdd = false;
	}

	function removeMode(name: string) {
		customModes = customModes.filter((m) => m.name !== name);
		saveCustomModes();
		if (activeCustomMode === name) {
			activeCustomMode = null;
			chatStore.setMode('talk');
		}
	}

	// Expose active custom mode's prompt injection for ChatPanel to use
	export function getActivePromptInjection(): string | null {
		if (!activeCustomMode) return null;
		return customModes.find((m) => m.name === activeCustomMode)?.promptInjection ?? null;
	}
</script>

<div class="flex flex-wrap items-center gap-1">
	{#each builtinModes as m}
		<button
			class="rounded-md px-2.5 py-1 text-xs transition-colors duration-[var(--dur-fast)]
				{chatStore.mode === m.value && !activeCustomMode
				? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
				: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
			onclick={() => selectBuiltin(m.value)}
		>
			{m.label}
		</button>
	{/each}
	{#each customModes as m}
		<button
			class="group flex items-center gap-1 rounded-md px-2.5 py-1 text-xs transition-colors duration-[var(--dur-fast)]
				{activeCustomMode === m.name
				? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
				: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
			onclick={() => selectCustom(m)}
		>
			{m.name}
			<span
				role="button"
				tabindex="-1"
				class="hidden group-hover:inline text-[var(--text-tertiary)] hover:text-[var(--color-error)]"
				onclick={(e: MouseEvent) => { e.stopPropagation(); removeMode(m.name); }}
				onkeydown={(e) => e.key === 'Enter' && removeMode(m.name)}
			>
				<X class="h-2.5 w-2.5" />
			</span>
		</button>
	{/each}
	<button
		class="rounded-md px-1.5 py-1 text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
		onclick={() => (showAdd = !showAdd)}
	>
		<Plus class="h-3 w-3" />
	</button>
</div>

{#if showAdd}
	<div class="mt-2 rounded-lg border border-border-subtle bg-[var(--bg-2)] p-3">
		<div class="flex flex-col gap-2">
			<input
				bind:value={newName}
				placeholder="Mode name"
				class="rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 py-1 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
			/>
			<input
				bind:value={newDesc}
				placeholder="Description (optional)"
				class="rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 py-1 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none"
			/>
			<textarea
				bind:value={newPrompt}
				placeholder="System prompt injection"
				rows="2"
				class="resize-none rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 py-1 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none"
			></textarea>
			<div class="flex gap-2">
				<button
					class="rounded-md bg-[var(--pub-accent)] px-2.5 py-1 text-xs text-[var(--primary-foreground)] disabled:opacity-50"
					onclick={addMode}
					disabled={!newName.trim()}
				>
					Add
				</button>
				<button
					class="text-xs text-[var(--text-tertiary)]"
					onclick={() => (showAdd = false)}
				>
					Cancel
				</button>
			</div>
		</div>
	</div>
{/if}
