<script lang="ts">
	import { goto } from '$app/navigation';
	import { appStore } from '$lib/stores/app.svelte';
	import { chatStore } from '$lib/stores/chat.svelte';
	import { memoryStore } from '$lib/stores/memory.svelte';
	import { Search } from '@lucide/svelte';
	import {
		LayoutDashboard,
		Inbox,
		MessageSquare,
		Brain,
		Bot,
		Sparkles,
		Heart,
		Settings,
		Sun,
		Moon,
		RefreshCw,
	} from '@lucide/svelte';

	let query = $state('');
	let selectedIndex = $state(0);
	let inputEl = $state<HTMLInputElement | null>(null);

	interface CommandItem {
		id: string;
		label: string;
		category: string;
		icon: typeof Search;
		action: () => void;
	}

	const staticCommands: CommandItem[] = [
		{ id: 'nav-today', label: 'Go to Today', category: 'Navigation', icon: LayoutDashboard, action: () => nav('/today') },
		{ id: 'nav-ops', label: 'Go to Ops', category: 'Navigation', icon: Inbox, action: () => nav('/ops') },
		{ id: 'nav-chat', label: 'Go to Chat', category: 'Navigation', icon: MessageSquare, action: () => nav('/chat') },
		{ id: 'nav-memory', label: 'Go to Memory', category: 'Navigation', icon: Brain, action: () => nav('/memory') },
		{ id: 'nav-agents', label: 'Go to Agents', category: 'Navigation', icon: Bot, action: () => nav('/agents') },
		{ id: 'nav-skills', label: 'Go to Skills', category: 'Navigation', icon: Sparkles, action: () => nav('/skills') },
		{ id: 'nav-soul', label: 'Go to Soul', category: 'Navigation', icon: Heart, action: () => nav('/soul') },
		{ id: 'nav-settings', label: 'Go to Settings', category: 'Navigation', icon: Settings, action: () => nav('/settings') },
		{ id: 'act-theme', label: 'Toggle theme', category: 'Actions', icon: Sun, action: () => { appStore.toggleTheme(); close(); } },
		{ id: 'act-sync', label: 'Manual sync', category: 'Actions', icon: RefreshCw, action: () => { close(); } },
	];

	// Session commands built from chat sessions
	const sessionCommands = $derived<CommandItem[]>(
		chatStore.sessions.map((s) => ({
			id: `session-${s.id}`,
			label: s.title ?? `Session ${s.id.slice(0, 8)}`,
			category: 'Sessions',
			icon: MessageSquare,
			action: () => {
				chatStore.setCurrentSession(s.id);
				nav('/chat');
			},
		})),
	);

	const allCommands = $derived([...staticCommands, ...sessionCommands]);

	const filtered = $derived(() => {
		if (!query.trim()) return allCommands;
		const q = query.toLowerCase();
		return allCommands.filter(
			(c) => c.label.toLowerCase().includes(q) || c.category.toLowerCase().includes(q),
		);
	});

	$effect(() => {
		if (appStore.commandPaletteOpen) {
			selectedIndex = 0;
			query = '';
			// Focus input after render
			setTimeout(() => inputEl?.focus(), 10);
		}
	});

	function nav(path: string) {
		close();
		goto(path);
	}

	function close() {
		appStore.closeCommandPalette();
	}

	function handleKeydown(e: KeyboardEvent) {
		const items = filtered();
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				selectedIndex = (selectedIndex + 1) % items.length;
				break;
			case 'ArrowUp':
				e.preventDefault();
				selectedIndex = (selectedIndex - 1 + items.length) % items.length;
				break;
			case 'Enter':
				e.preventDefault();
				if (items[selectedIndex]) {
					items[selectedIndex].action();
				}
				break;
			case 'Escape':
				e.preventDefault();
				close();
				break;
		}
	}
</script>

{#if appStore.commandPaletteOpen}
	<div class="fixed inset-0 z-50 bg-black/50" role="presentation">
		<button
			type="button"
			class="absolute inset-0 w-full h-full appearance-none bg-transparent border-none cursor-default"
			aria-label="Close command palette"
			onclick={close}
		></button>
		<div
			class="relative mx-auto mt-[20vh] w-full max-w-lg rounded-xl border border-border-subtle bg-[var(--bg-1)] shadow-lg backdrop-blur-sm"
			role="dialog"
			aria-modal="true"
			aria-label="Command palette"
			tabindex="-1"
			onclick={(e) => e.stopPropagation()}
			onkeydown={handleKeydown}
		>
			<!-- Search input -->
			<div class="flex items-center gap-3 border-b border-border-subtle px-4 py-3">
				<Search class="h-4 w-4 flex-shrink-0 text-[var(--text-tertiary)]" />
				<input
					bind:this={inputEl}
					bind:value={query}
					type="text"
					placeholder="Search commands, views, sessions..."
					class="flex-1 bg-transparent text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none"
				/>
				<kbd class="rounded border border-border-subtle bg-[var(--bg-3)] px-1.5 py-0.5 text-[10px] text-[var(--text-tertiary)]">
					esc
				</kbd>
			</div>

			<!-- Results -->
			<div class="max-h-72 overflow-y-auto p-2" role="menu" aria-label="Commands">
				{#each filtered() as item, i (item.id)}
					<button
						class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors duration-[var(--dur-fast)]
							{i === selectedIndex
							? 'bg-[var(--accent-dim)] text-[var(--cairn-accent)]'
							: 'text-[var(--text-secondary)] hover:bg-[var(--bg-2)]'}"
						role="menuitem"
						onclick={() => item.action()}
						onmouseenter={() => (selectedIndex = i)}
					>
						<item.icon class="h-4 w-4 flex-shrink-0" />
						<span class="flex-1 text-left">{item.label}</span>
						<span class="text-[10px] text-[var(--text-tertiary)]">{item.category}</span>
					</button>
				{/each}
				{#if filtered().length === 0}
					<p class="px-3 py-4 text-center text-xs text-[var(--text-tertiary)]">No results</p>
				{/if}
			</div>
		</div>
	</div>
{/if}
