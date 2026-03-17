<script lang="ts">
	import { page } from '$app/state';
	import { appStore } from '$lib/stores/app.svelte';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import {
		LayoutDashboard,
		Inbox,
		MessageSquare,
		Brain,
		Bot,
		Sparkles,
		Heart,
		Settings,
	} from '@lucide/svelte';

	const navItems = [
		{ href: '/today', label: 'Today', icon: LayoutDashboard, key: '1' },
		{ href: '/ops', label: 'Ops', icon: Inbox, key: '2' },
		{ href: '/chat', label: 'Chat', icon: MessageSquare, key: '3' },
		{ href: '/memory', label: 'Memory', icon: Brain, key: '4' },
		{ href: '/agents', label: 'Agents', icon: Bot, key: '5' },
		{ href: '/skills', label: 'Skills', icon: Sparkles, key: '6' },
		{ href: '/soul', label: 'Soul', icon: Heart, key: '7' },
		{ href: '/settings', label: 'Settings', icon: Settings, key: '8' },
	];

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === '/today') return path === '/' || path === '/today';
		return path.startsWith(href);
	}

	function badge(href: string): number | null {
		if (href === '/ops') {
			const count = taskStore.pendingApprovals.length;
			return count > 0 ? count : null;
		}
		if (href === '/today') {
			const count = feedStore.unreadCount;
			return count > 0 ? count : null;
		}
		return null;
	}
</script>

<nav
	class="hidden md:flex w-[var(--sidebar-w)] flex-col border-r border-border-subtle bg-[var(--bg-1)] overflow-y-auto"
	class:!flex={!appStore.sidebarCollapsed}
>
	<div class="flex flex-col gap-1 p-3 pt-4">
		{#each navItems as item}
			{@const active = isActive(item.href)}
			{@const count = badge(item.href)}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors duration-[var(--dur-fast)]
					{active
					? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
					: 'text-[var(--text-secondary)] hover:bg-[var(--bg-3)] hover:text-[var(--text-primary)]'}"
			>
				<item.icon class="h-4 w-4 flex-shrink-0" />
				<span class="flex-1">{item.label}</span>
				{#if count}
					<span class="min-w-[20px] rounded-full bg-[var(--pub-accent)] px-1.5 py-0.5 text-center text-[10px] font-medium text-[var(--primary-foreground)]">
						{count > 99 ? '99+' : count}
					</span>
				{/if}
				<kbd class="hidden lg:inline text-[10px] text-[var(--text-tertiary)] opacity-50">{item.key}</kbd>
			</a>
		{/each}
	</div>
</nav>
