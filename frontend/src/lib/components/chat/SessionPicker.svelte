<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import { getSessionMessages } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MessageSquare, Plus, Loader2 } from '@lucide/svelte';

	let loading = $state(false);

	async function selectSession(id: string) {
		if (loading) return;
		loading = true;
		chatStore.setCurrentSession(id);
		chatStore.clearStreaming();
		try {
			const res = await getSessionMessages(id);
			chatStore.setMessages(res.items);
		} catch {
			chatStore.setMessages([]);
		} finally {
			loading = false;
		}
	}

	function newSession() {
		chatStore.setCurrentSession(null);
		chatStore.setMessages([]);
		chatStore.clearStreaming();
	}

	const sessionLabel = $derived(() => {
		if (loading) return 'Loading...';
		if (!chatStore.currentSessionId) return 'New chat';
		const s = chatStore.sessions.find((s) => s.id === chatStore.currentSessionId);
		return s?.title ?? `Session ${chatStore.currentSessionId.slice(0, 6)}`;
	});
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		<Button variant="outline" size="sm" class="h-6 text-[11px] gap-1.5 px-2 max-w-40">
			{#if loading}
				<Loader2 class="h-3 w-3 animate-spin" />
			{:else}
				<MessageSquare class="h-3 w-3" />
			{/if}
			<span class="truncate">{sessionLabel()}</span>
		</Button>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content class="w-72" align="start">
		<DropdownMenu.Item class="gap-2 text-xs" onclick={newSession}>
			<Plus class="h-3 w-3" /> New chat
		</DropdownMenu.Item>
		{#if chatStore.sessions.length > 0}
			<DropdownMenu.Separator />
			{#each chatStore.sessions as session (session.id)}
				<DropdownMenu.Item
					class="justify-between text-xs {chatStore.currentSessionId === session.id ? 'text-[var(--cairn-accent)]' : ''}"
					onclick={() => selectSession(session.id)}
				>
					<span class="truncate flex-1">{session.title ?? `Session ${session.id.slice(0, 6)}`}</span>
					<span class="ml-2 flex-shrink-0 text-[10px] text-[var(--text-tertiary)] tabular-nums font-mono">
						{session.messageCount ?? 0} msgs
					</span>
				</DropdownMenu.Item>
			{/each}
		{/if}
	</DropdownMenu.Content>
</DropdownMenu.Root>
