<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import { getSessionMessages } from '$lib/api/client';
	import { relativeTime } from '$lib/utils/time';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MessageSquare, Plus } from '@lucide/svelte';

	async function selectSession(id: string) {
		chatStore.setCurrentSession(id);
		try {
			const res = await getSessionMessages(id);
			chatStore.setMessages(res.items);
		} catch {
			// handled
		}
	}

	function newSession() {
		chatStore.setCurrentSession(null);
		chatStore.setMessages([]);
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		<Button variant="outline" size="sm" class="h-6 text-[11px] gap-1.5 px-2">
			<MessageSquare class="h-3 w-3" />
			{chatStore.currentSessionId ? 'Session' : 'New chat'}
		</Button>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content class="w-64" align="start">
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
					<span class="truncate">{session.title ?? `Session ${session.id.slice(0, 6)}`}</span>
					<span class="ml-2 flex-shrink-0 text-[10px] text-[var(--text-tertiary)] tabular-nums font-mono">
						{relativeTime(session.lastMessageAt)}
					</span>
				</DropdownMenu.Item>
			{/each}
		{/if}
	</DropdownMenu.Content>
</DropdownMenu.Root>
