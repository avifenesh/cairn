<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import type { ChatMode } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
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
			return JSON.parse(localStorage.getItem('cairn_custom_modes') || '[]');
		} catch {
			return [];
		}
	}

	function saveCustomModes() {
		try {
			localStorage.setItem('cairn_custom_modes', JSON.stringify(customModes));
		} catch {}
	}

	function selectBuiltin(mode: ChatMode) {
		chatStore.setMode(mode);
		activeCustomMode = null;
	}

	function selectCustom(mode: CustomMode) {
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

	export function getActivePromptInjection(): string | null {
		if (!activeCustomMode) return null;
		return customModes.find((m) => m.name === activeCustomMode)?.promptInjection ?? null;
	}
</script>

<div class="flex flex-wrap items-center gap-1">
	{#each builtinModes as m}
		<Button
			variant={chatStore.mode === m.value && !activeCustomMode ? 'secondary' : 'ghost'}
			size="sm"
			class="h-6 text-[11px] px-2
				{chatStore.mode === m.value && !activeCustomMode ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}"
			onclick={() => selectBuiltin(m.value)}
		>
			{m.label}
		</Button>
	{/each}
	{#each customModes as m}
		<Button
			variant={activeCustomMode === m.name ? 'secondary' : 'ghost'}
			size="sm"
			class="group h-6 text-[11px] px-2 gap-1
				{activeCustomMode === m.name ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}"
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
		</Button>
	{/each}
	<Button
		variant="ghost"
		size="sm"
		class="h-6 w-6 p-0 text-[var(--text-tertiary)]"
		onclick={() => (showAdd = !showAdd)}
	>
		<Plus class="h-3 w-3" />
	</Button>
</div>

{#if showAdd}
	<div class="mt-2 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
		<div class="flex flex-col gap-2">
			<Input
				bind:value={newName}
				placeholder="Mode name"
				class="h-7 text-xs"
			/>
			<Input
				bind:value={newDesc}
				placeholder="Description (optional)"
				class="h-7 text-xs"
			/>
			<textarea
				bind:value={newPrompt}
				placeholder="System prompt injection"
				rows="2"
				class="resize-none rounded-md border border-border-subtle bg-[var(--bg-0)] px-3 py-2 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none transition-colors"
			></textarea>
			<div class="flex gap-2">
				<Button size="sm" class="h-7 text-xs" onclick={addMode} disabled={!newName.trim()}>
					Add
				</Button>
				<Button variant="ghost" size="sm" class="h-7 text-xs" onclick={() => (showAdd = false)}>
					Cancel
				</Button>
			</div>
		</div>
	</div>
{/if}
