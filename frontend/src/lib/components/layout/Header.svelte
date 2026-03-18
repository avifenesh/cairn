<script lang="ts">
	import { page } from '$app/state';
	import { appStore } from '$lib/stores/app.svelte';
	import { sseStore } from '$lib/stores/sse.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { Circle, Search, Sun, Moon, DollarSign, HelpCircle, PanelRight } from '@lucide/svelte';

	const isChat = $derived(page.url.pathname === '/chat');

	function handleKeyboardShortcut() {
		appStore.openCommandPalette();
	}

	const budgetPct = $derived(
		appStore.budgetTodayUsd != null && appStore.budgetDailyLimitUsd
			? Math.min(100, Math.round((appStore.budgetTodayUsd / appStore.budgetDailyLimitUsd) * 100))
			: null,
	);

	const healthColor = $derived(
		appStore.sseConnected ? 'text-[var(--color-success)]'
		: sseStore.reconnecting ? 'text-[var(--color-warning)]'
		: 'text-[var(--color-error)]',
	);
	const healthLabel = $derived(
		appStore.sseConnected ? 'Live'
		: sseStore.reconnecting ? 'Reconnecting'
		: 'Offline',
	);

	const currentView = $derived(() => {
		const path = page.url.pathname;
		if (path === '/' || path === '/today') return 'Today';
		return path.slice(1).charAt(0).toUpperCase() + path.slice(2);
	});
</script>

<header
	class="flex h-[var(--header-h)] items-center border-b border-border-subtle bg-[var(--bg-1)] px-3 gap-3"
>
	<!-- Breadcrumb -->
	<div class="flex items-center gap-2 flex-1 min-w-0">
		<span class="text-sm font-medium text-[var(--text-primary)] truncate">{currentView()}</span>
		<Separator orientation="vertical" class="h-4" />
		<span class="flex items-center gap-1.5 text-xs">
			<Circle class="h-1.5 w-1.5 fill-current {healthColor} {appStore.sseConnected ? 'animate-pulse-dot' : ''}" />
			<span class="text-[var(--text-tertiary)] hidden sm:inline">{healthLabel}</span>
		</span>
	</div>

	<!-- Actions -->
	<div class="flex items-center gap-1.5">
		<!-- Search / command palette -->
		<button
			class="flex items-center gap-2 rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1 text-xs text-[var(--text-tertiary)] hover:border-[var(--border-default)] hover:text-[var(--text-secondary)] transition-colors"
			onclick={handleKeyboardShortcut}
		>
			<Search class="h-3 w-3" />
			<span class="hidden sm:inline">Search</span>
			<kbd class="hidden sm:inline ml-1 rounded border border-border-subtle bg-[var(--bg-2)] px-1 py-0.5 text-[10px] font-mono">⌘K</kbd>
		</button>

		{#if budgetPct != null}
			<span class="hidden sm:flex items-center gap-1 rounded-md border border-border-subtle bg-[var(--bg-0)] px-2 py-1 text-[10px] font-mono text-[var(--text-tertiary)]
				{budgetPct > 95 ? 'border-[var(--color-error)]/30 text-[var(--color-error)]' : budgetPct > 80 ? 'border-[var(--color-warning)]/30 text-[var(--color-warning)]' : ''}">
				<DollarSign class="h-3 w-3" />
				{budgetPct}%
			</span>
		{/if}

		{#if isChat}
			<Button
				variant="ghost"
				size="icon"
				class="h-8 w-8"
				onclick={() => appStore.toggleContextPanel()}
				aria-label="Toggle context panel"
			>
				<PanelRight class="h-4 w-4 text-[var(--text-tertiary)] {appStore.contextPanelOpen ? 'text-[var(--cairn-accent)]' : ''}" />
			</Button>
		{/if}

		<Button
			variant="ghost"
			size="icon"
			class="h-8 w-8"
			onclick={() => appStore.toggleHelpModal()}
		>
			<HelpCircle class="h-4 w-4 text-[var(--text-tertiary)]" />
		</Button>

		<Button
			variant="ghost"
			size="icon"
			class="h-8 w-8"
			onclick={() => appStore.toggleTheme()}
		>
			{#if appStore.theme === 'dark'}
				<Sun class="h-4 w-4 text-[var(--text-tertiary)]" />
			{:else}
				<Moon class="h-4 w-4 text-[var(--text-tertiary)]" />
			{/if}
		</Button>
	</div>
</header>
