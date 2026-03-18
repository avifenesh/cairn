<script lang="ts">
	import { page } from '$app/state';
	import { appStore } from '$lib/stores/app.svelte';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import {
		LayoutDashboard,
		Inbox,
		MessageSquare,
		Brain,
		Bot,
		Sparkles,
		Heart,
		Settings,
		PanelLeftClose,
		PanelLeft,
	} from '@lucide/svelte';

	const navItems = [
		{ href: '/today', label: 'Today', icon: LayoutDashboard, key: '1' },
		{ href: '/ops', label: 'Ops', icon: Inbox, key: '2' },
		{ href: '/chat', label: 'Chat', icon: MessageSquare, key: '3' },
		{ href: '/memory', label: 'Memory', icon: Brain, key: '4' },
		{ href: '/agents', label: 'Agents', icon: Bot, key: '5' },
		{ href: '/skills', label: 'Skills', icon: Sparkles, key: '6' },
		{ href: '/soul', label: 'Soul', icon: Heart, key: '7' },
	];

	const bottomItems = [
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

	const collapsed = $derived(appStore.sidebarCollapsed);
</script>

<nav
	class="hidden md:flex flex-col border-r border-border-subtle bg-[var(--bg-1)] overflow-y-auto transition-[width] duration-[var(--dur-normal)] ease-[var(--ease-out)]"
	style="width: {collapsed ? 'var(--sidebar-collapsed-w)' : 'var(--sidebar-w)'}"
>
	<!-- Logo + collapse toggle -->
	<div class="flex items-center h-[var(--header-h)] px-3 border-b border-border-subtle">
		{#if !collapsed}
			<span class="text-sm font-semibold tracking-tight text-[var(--text-primary)] flex-1">Cairn</span>
		{/if}
		<button
			class="flex items-center justify-center h-8 w-8 rounded-md text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-2)] transition-colors"
			onclick={() => appStore.toggleSidebar()}
			aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
		>
			{#if collapsed}
				<PanelLeft class="h-4 w-4" />
			{:else}
				<PanelLeftClose class="h-4 w-4" />
			{/if}
		</button>
	</div>

	<!-- Main nav -->
	<div class="flex flex-col gap-0.5 p-2 flex-1">
		{#each navItems as item}
			{@const active = isActive(item.href)}
			{@const count = badge(item.href)}
			<Tooltip.Root>
				<Tooltip.Trigger>
					<a
						href={item.href}
						class="flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors duration-[var(--dur-fast)] relative
							{active
							? 'nav-active-bar bg-[var(--accent-dim)] text-[var(--cairn-accent)] font-medium'
							: 'text-[var(--text-secondary)] hover:bg-[var(--bg-2)] hover:text-[var(--text-primary)]'}"
					>
						<item.icon class="h-4 w-4 flex-shrink-0" />
						{#if !collapsed}
							<span class="flex-1 truncate">{item.label}</span>
							{#if count}
								<Badge variant="default" class="h-5 min-w-5 px-1.5 text-[10px]">
									{count > 99 ? '99+' : count}
								</Badge>
							{/if}
							<kbd class="hidden lg:inline text-[10px] text-[var(--text-tertiary)] opacity-40 font-mono">{item.key}</kbd>
						{:else if count}
							<span class="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-[var(--cairn-accent)]"></span>
						{/if}
					</a>
				</Tooltip.Trigger>
				{#if collapsed}
					<Tooltip.Content side="right">
						<p>{item.label}{count ? ` (${count})` : ''}</p>
					</Tooltip.Content>
				{/if}
			</Tooltip.Root>
		{/each}
	</div>

	<!-- Bottom section -->
	<div class="p-2">
		<Separator class="mb-2" />
		{#each bottomItems as item}
			{@const active = isActive(item.href)}
			<Tooltip.Root>
				<Tooltip.Trigger>
					<a
						href={item.href}
						class="flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors duration-[var(--dur-fast)]
							{active
							? 'nav-active-bar bg-[var(--accent-dim)] text-[var(--cairn-accent)]'
							: 'text-[var(--text-tertiary)] hover:bg-[var(--bg-2)] hover:text-[var(--text-secondary)]'}"
					>
						<item.icon class="h-4 w-4 flex-shrink-0" />
						{#if !collapsed}
							<span class="flex-1">{item.label}</span>
							<kbd class="hidden lg:inline text-[10px] text-[var(--text-tertiary)] opacity-40 font-mono">{item.key}</kbd>
						{/if}
					</a>
				</Tooltip.Trigger>
				{#if collapsed}
					<Tooltip.Content side="right">
						<p>{item.label}</p>
					</Tooltip.Content>
				{/if}
			</Tooltip.Root>
		{/each}
	</div>
</nav>
