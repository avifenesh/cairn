<script lang="ts">
	import { page } from '$app/state';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import { LayoutDashboard, Inbox, MessageSquare, MoreHorizontal } from '@lucide/svelte';

	let moreOpen = $state(false);

	const mainItems = [
		{ href: '/today', label: 'Today', icon: LayoutDashboard },
		{ href: '/ops', label: 'Ops', icon: Inbox },
		{ href: '/chat', label: 'Chat', icon: MessageSquare },
	];

	const moreItems = [
		{ href: '/memory', label: 'Memory' },
		{ href: '/agents', label: 'Agents' },
		{ href: '/skills', label: 'Skills' },
		{ href: '/soul', label: 'Soul' },
		{ href: '/settings', label: 'Settings' },
	];

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === '/today') return path === '/' || path === '/today';
		return path.startsWith(href);
	}

	const isMoreActive = $derived(moreItems.some((i) => isActive(i.href)));
</script>

<nav class="md:hidden fixed bottom-0 left-0 right-0 z-50 border-t border-border-subtle bg-[var(--bg-1)] safe-area-bottom">
	<div class="flex h-[var(--bottom-nav-h)] items-stretch">
		{#each mainItems as item}
			{@const active = isActive(item.href)}
			<a
				href={item.href}
				class="relative flex flex-1 flex-col items-center justify-center gap-0.5 text-[10px]
					{active ? 'text-[var(--pub-accent)]' : 'text-[var(--text-tertiary)]'}"
			>
				<item.icon class="h-5 w-5" />
				<span>{item.label}</span>
				{#if item.href === '/ops' && taskStore.pendingApprovals.length > 0}
					<span class="absolute top-1.5 right-1/4 h-2 w-2 rounded-full bg-[var(--pub-accent)]"></span>
				{/if}
			</a>
		{/each}
		<button
			class="flex flex-1 flex-col items-center justify-center gap-0.5 text-[10px]
				{isMoreActive || moreOpen ? 'text-[var(--pub-accent)]' : 'text-[var(--text-tertiary)]'}"
			onclick={() => (moreOpen = !moreOpen)}
			onkeydown={(e) => e.key === 'Escape' && moreOpen && (moreOpen = false)}
			aria-expanded={moreOpen}
			aria-haspopup="menu"
		>
			<MoreHorizontal class="h-5 w-5" />
			<span>More</span>
		</button>
	</div>

	{#if moreOpen}
		<div
			class="absolute bottom-full left-0 right-0 border-t border-border-subtle bg-[var(--bg-1)] p-2 shadow-lg"
			role="menu"
			aria-label="More navigation"
		>
			{#each moreItems as item}
				<a
					href={item.href}
					role="menuitem"
					class="block rounded-md px-4 py-2.5 text-sm
						{isActive(item.href)
						? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
						: 'text-[var(--text-secondary)] hover:bg-[var(--bg-3)]'}"
					onclick={() => (moreOpen = false)}
				>
					{item.label}
				</a>
			{/each}
		</div>
	{/if}
</nav>

<style>
	.safe-area-bottom {
		padding-bottom: env(safe-area-inset-bottom, 0px);
	}
</style>
