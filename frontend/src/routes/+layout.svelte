<script lang="ts">
	import '../app.css';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import Header from '$lib/components/layout/Header.svelte';
	import Sidebar from '$lib/components/layout/Sidebar.svelte';
	import BottomNav from '$lib/components/layout/BottomNav.svelte';
	import StatusBar from '$lib/components/layout/StatusBar.svelte';
	import ContextPanel from '$lib/components/layout/ContextPanel.svelte';
	import CommandPalette from '$lib/components/layout/CommandPalette.svelte';
	import HelpModal from '$lib/components/layout/HelpModal.svelte';
	import TokenGate from '$lib/components/layout/TokenGate.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { sseStore } from '$lib/stores/sse.svelte';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import { keyboardNav } from '$lib/stores/keyboard-nav.svelte';
	import { markRead, triggerPoll, approve, deny } from '$lib/api/client';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	let { children } = $props();

	// Token gate: check once on mount, reload on auth
	let isAuthed = $state(false);
	try {
		isAuthed = !!localStorage.getItem('cairn_api_token');
	} catch {}

	// Derive item count from active view so j/k/o/r/a/d shortcuts work
	$effect(() => {
		const path = page.url.pathname;
		if (path === '/' || path === '/today') {
			// On home page, keyboard targets approvals first, then feed
			const approvalCount = taskStore.pendingApprovals.length;
			keyboardNav.setItemCount(approvalCount > 0 ? approvalCount : feedStore.items.length);
		} else if (path === '/ops') {
			keyboardNav.setItemCount(taskStore.pendingApprovals.length);
		} else {
			keyboardNav.setItemCount(0);
		}
	});

	// Auto-mood: reactive to Settings toggle, hourly check
	let moodInterval: ReturnType<typeof setInterval> | undefined;
	$effect(() => {
		if (moodInterval) clearInterval(moodInterval);
		if (appStore.autoMoodEnabled) {
			appStore.applyAutoMood();
			moodInterval = setInterval(() => appStore.applyAutoMood(), 60 * 60 * 1000);
		}
		return () => { if (moodInterval) clearInterval(moodInterval); };
	});

	onMount(() => {
		appStore.initTheme();
		if (isAuthed) sseStore.connect();

		function handleKeydown(e: KeyboardEvent) {
			if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
				e.preventDefault();
				appStore.toggleCommandPalette();
				return;
			}

			const tag = (e.target as HTMLElement)?.tagName;
			if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
			if ((e.target as HTMLElement)?.isContentEditable) return;

			const viewKeys: Record<string, string> = {
				'1': '/today', '2': '/ops', '3': '/chat', '4': '/memory',
				'5': '/agents', '6': '/skills', '7': '/soul', '8': '/settings',
			};

			if (viewKeys[e.key]) {
				goto(viewKeys[e.key]);
				return;
			}

			const path = page.url.pathname;

			switch (e.key) {
				case 'j':
					keyboardNav.moveDown();
					break;
				case 'k':
					keyboardNav.moveUp();
					break;
				case 'o': {
					// Open focused item URL
					if (path === '/today' || path === '/') {
						const item = feedStore.items[keyboardNav.focusedIndex];
						if (item?.url) window.open(item.url, '_blank');
					}
					break;
				}
				case 'r': {
					// Mark focused feed item read
					if (path === '/today' || path === '/') {
						const item = feedStore.items[keyboardNav.focusedIndex];
						if (item && !item.isRead) {
							feedStore.markItemRead(item.id);
							markRead(item.id).catch(() => {});
						}
					}
					break;
				}
				case 'x':
					keyboardNav.toggleSelection();
					break;
				case 's':
					triggerPoll().catch(() => {});
					break;
				case 'a': {
					// Approve focused approval (ops or today)
					if (path === '/ops' || path === '/today' || path === '/') {
						const appr = taskStore.pendingApprovals[keyboardNav.focusedIndex];
						if (appr) {
							taskStore.resolveApproval(appr.id, 'approved');
							approve(appr.id).catch(() => {});
						}
					}
					break;
				}
				case 'd': {
					// Deny focused approval (ops or today)
					if (path === '/ops' || path === '/today' || path === '/') {
						const appr = taskStore.pendingApprovals[keyboardNav.focusedIndex];
						if (appr) {
							taskStore.resolveApproval(appr.id, 'denied');
							deny(appr.id).catch(() => {});
						}
					}
					break;
				}
				case 't':
					appStore.toggleTheme();
					break;
				case '?':
					appStore.toggleHelpModal();
					break;
				case 'Escape':
					appStore.closeCommandPalette();
					appStore.closeHelpModal();
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

{#if !isAuthed}
	<TokenGate />
{:else}
<Tooltip.Provider>
<div class="flex h-dvh flex-col overflow-hidden bg-background text-foreground">
	<Header />

	<div class="flex flex-1 overflow-hidden">
		<Sidebar />
		<main class="flex-1 overflow-y-auto pb-[var(--bottom-nav-h)] md:pb-0">
			{@render children()}
		</main>
		{#if page.url.pathname === '/chat'}
			<ContextPanel
				open={appStore.contextPanelOpen}
				onclose={() => appStore.closeContextPanel()}
				onopen={() => appStore.openContextPanel()}
			/>
		{/if}
	</div>

	<StatusBar />
	<BottomNav />
</div>
</Tooltip.Provider>

<CommandPalette />
<HelpModal open={appStore.helpModalOpen} onclose={() => appStore.closeHelpModal()} />

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
{/if}
