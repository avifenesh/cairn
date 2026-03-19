<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import type { ChatMode } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Plus, X, ChevronDown } from '@lucide/svelte';

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
		customModes = [...customModes, { name, description: '', promptInjection: newPrompt }];
		saveCustomModes();
		newName = '';
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

	const currentLabel = $derived(
		activeCustomMode ?? builtinModes.find(m => m.value === chatStore.mode)?.label ?? 'Talk'
	);
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		<Button
			variant="ghost"
			size="sm"
			class="h-6 text-[11px] px-2 gap-1 text-[var(--cairn-accent)]"
		>
			{currentLabel}
			<ChevronDown class="h-2.5 w-2.5 opacity-50" />
		</Button>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content class="w-56" align="start">
		{#each builtinModes as m}
			<DropdownMenu.Item
				class="text-xs gap-2 {chatStore.mode === m.value && !activeCustomMode ? 'text-[var(--cairn-accent)]' : ''}"
				onclick={() => selectBuiltin(m.value)}
			>
				{m.label}
			</DropdownMenu.Item>
		{/each}

		{#if customModes.length > 0}
			<DropdownMenu.Separator />
			{#each customModes as m}
				<DropdownMenu.Item
					class="text-xs gap-2 justify-between {activeCustomMode === m.name ? 'text-[var(--cairn-accent)]' : ''}"
					onclick={() => selectCustom(m)}
				>
					{m.name}
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<span
						class="text-[var(--text-tertiary)] hover:text-[var(--color-error)] p-0.5 rounded"
						onclick={(e) => { e.stopPropagation(); removeMode(m.name); }}
					>
						<X class="h-3 w-3" />
					</span>
				</DropdownMenu.Item>
			{/each}
		{/if}

		<DropdownMenu.Separator />
		{#if showAdd}
			<div class="p-2 space-y-2" onclick={(e) => e.stopPropagation()}>
				<Input
					bind:value={newName}
					placeholder="Mode name"
					class="h-7 text-xs"
				/>
				<textarea
					bind:value={newPrompt}
					placeholder="System prompt (optional)"
					rows="2"
					class="w-full resize-none rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none transition-colors"
				></textarea>
				<div class="flex gap-1.5">
					<Button size="sm" class="h-6 text-[10px] px-2" onclick={addMode} disabled={!newName.trim()}>
						Add
					</Button>
					<Button variant="ghost" size="sm" class="h-6 text-[10px] px-2" onclick={() => (showAdd = false)}>
						Cancel
					</Button>
				</div>
			</div>
		{:else}
			<DropdownMenu.Item class="text-xs gap-2" onclick={() => (showAdd = true)}>
				<Plus class="h-3 w-3" />
				New mode
			</DropdownMenu.Item>
		{/if}
	</DropdownMenu.Content>
</DropdownMenu.Root>
