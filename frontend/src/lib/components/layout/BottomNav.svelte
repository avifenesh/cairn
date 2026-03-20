<script lang="ts">
	import { page } from '$app/state';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import { Brain, Bot, Sparkles, Heart, Settings, LayoutDashboard, Inbox, MessageSquare, MoreHorizontal, Eye } from '@lucide/svelte';

	let moreOpen = $state(false);

	const mainItems = [
		{ href: '/today', label: 'Today', icon: LayoutDashboard },
		{ href: '/ops', label: 'Ops', icon: Inbox },
		{ href: '/chat', label: 'Chat', icon: MessageSquare },
		{ href: '/activity', label: 'Activity', icon: Eye },
	];

	const moreItems = [
		{ href: '/memory', label: 'Memory', icon: Brain },
		{ href: '/agents', label: 'Agents', icon: Bot },
		{ href: '/skills', label: 'Skills', icon: Sparkles },
		{ href: '/soul', label: 'Soul', icon: Heart },
		{ href: '/settings', label: 'Settings', icon: Settings },
	];

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === '/today') return path === '/' || path === '/today';
		return path.startsWith(href);
	}

	const isMoreActive = $derived(moreItems.some((i) => isActive(i.href)));
</script>

<nav class="md:hidden fixed bottom-0 left-0 right-0 z-50 border-t border-border-subtle bg-[var(--bg-1)]/95 backdrop-blur-sm safe-area-bottom">
	<div class="flex h-[var(--bottom-nav-h)] items-stretch">
		{#each mainItems as item}
			{@const active = isActive(item.href)}
			<a
				href={item.href}
				class="relative flex flex-1 flex-col items-center justify-center gap-0.5 text-[10px] transition-colors
					{active ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}"
			>
				{#if active}
					<span class="absolute top-0 left-1/2 -translate-x-1/2 h-0.5 w-6 rounded-full bg-[var(--cairn-accent)]"></span>
				{/if}
				<item.icon class="h-5 w-5" />
				<span class="font-medium">{item.label}</span>
				{#if item.href === '/ops' && taskStore.pendingApprovals.length > 0}
					<span class="absolute top-1 right-1/4 h-2 w-2 rounded-full bg-[var(--cairn-accent)] animate-pulse-dot"></span>
				{/if}
			</a>
		{/each}
		<button
			class="flex flex-1 flex-col items-center justify-center gap-0.5 text-[10px] transition-colors
				{isMoreActive || moreOpen ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}"
			onclick={() => (moreOpen = !moreOpen)}
			onkeydown={(e) => e.key === 'Escape' && moreOpen && (moreOpen = false)}
			aria-expanded={moreOpen}
			aria-haspopup="menu"
		>
			<MoreHorizontal class="h-5 w-5" />
			<span class="font-medium">More</span>
		</button>
	</div>

	{#if moreOpen}
		<button
			type="button"
			class="fixed inset-0 bottom-[var(--bottom-nav-h)] appearance-none bg-transparent border-none cursor-default"
			aria-label="Close menu"
			onclick={() => (moreOpen = false)}
		></button>
		<div
			class="absolute bottom-full left-0 right-0 border-t border-border-subtle bg-[var(--bg-1)] p-2 shadow-lg"
			role="menu"
			tabindex="-1"
			aria-label="More navigation"
			onkeydown={(e) => e.key === 'Escape' && (moreOpen = false)}
		>
			<div class="grid grid-cols-3 gap-1">
				{#each moreItems as item}
					<a
						href={item.href}
						role="menuitem"
						class="flex flex-col items-center gap-1.5 rounded-lg px-3 py-3 text-xs transition-colors
							{isActive(item.href)
							? 'bg-[var(--accent-dim)] text-[var(--cairn-accent)]'
							: 'text-[var(--text-secondary)] hover:bg-[var(--bg-2)]'}"
						onclick={() => (moreOpen = false)}
					>
						<item.icon class="h-5 w-5" />
						<span>{item.label}</span>
					</a>
				{/each}
			</div>
		</div>
	{/if}
</nav>

<style>
	.safe-area-bottom {
		padding-bottom: env(safe-area-inset-bottom, 0px);
	}
</style>
