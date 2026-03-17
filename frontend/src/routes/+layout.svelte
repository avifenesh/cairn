<script lang="ts">
	import '../app.css';
	import Header from '$lib/components/layout/Header.svelte';
	import Sidebar from '$lib/components/layout/Sidebar.svelte';
	import BottomNav from '$lib/components/layout/BottomNav.svelte';
	import StatusBar from '$lib/components/layout/StatusBar.svelte';
	import ContextPanel from '$lib/components/layout/ContextPanel.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { sseStore } from '$lib/stores/sse.svelte';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(() => {
		appStore.initTheme();
		sseStore.connect();

		function handleKeydown(e: KeyboardEvent) {
			if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
				e.preventDefault();
				appStore.toggleCommandPalette();
				return;
			}

			const tag = (e.target as HTMLElement)?.tagName;
			if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
			if ((e.target as HTMLElement)?.isContentEditable) return;

			switch (e.key) {
				case 't':
					appStore.toggleTheme();
					break;
				case 'Escape':
					appStore.closeCommandPalette();
					break;
			}
		}

		document.addEventListener('keydown', handleKeydown);
		return () => {
			sseStore.disconnect();
			document.removeEventListener('keydown', handleKeydown);
		};
	});
</script>

<div class="flex h-dvh flex-col overflow-hidden bg-background text-foreground">
	<Header />

	<div class="flex flex-1 overflow-hidden">
		<Sidebar />
		<main class="flex-1 overflow-y-auto pb-[var(--bottom-nav-h)] md:pb-0">
			{@render children()}
		</main>
		<ContextPanel
			open={appStore.contextPanelOpen}
			onclose={() => appStore.closeContextPanel()}
		/>
	</div>

	<StatusBar />
	<BottomNav />
</div>

{#if appStore.notifications.length > 0}
	<div class="fixed right-4 top-[calc(var(--header-h)+8px)] z-50 flex flex-col gap-2">
		{#each appStore.notifications as notification (notification.id)}
			<div
				class="rounded-lg border border-border-subtle bg-[var(--bg-2)] px-4 py-3 text-sm text-[var(--text-primary)] shadow-md"
			>
				<button
					class="float-right ml-2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
					onclick={() => appStore.dismissNotification(notification.id)}
				>
					&times;
				</button>
				{notification.message}
			</div>
		{/each}
	</div>
{/if}
