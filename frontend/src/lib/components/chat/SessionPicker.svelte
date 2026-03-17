<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import { getSessionMessages } from '$lib/api/client';
	import { relativeTime } from '$lib/utils/time';
	import { MessageSquare, Plus } from '@lucide/svelte';

	let open = $state(false);

	async function selectSession(id: string) {
		chatStore.setCurrentSession(id);
		open = false;
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
		open = false;
	}
</script>

<div class="relative">
	<button
		class="flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-2.5 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors duration-[var(--dur-fast)]"
		onclick={() => (open = !open)}
	>
		<MessageSquare class="h-3 w-3" />
		{chatStore.currentSessionId ? 'Session' : 'New chat'}
	</button>

	{#if open}
		<div class="absolute left-0 top-full z-10 mt-1 w-64 rounded-lg border border-border-subtle bg-[var(--bg-2)] p-1 shadow-lg">
			<button
				class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)]"
				onclick={newSession}
			>
				<Plus class="h-3 w-3" /> New chat
			</button>
			{#each chatStore.sessions as session (session.id)}
				<button
					class="flex w-full items-center justify-between rounded-md px-3 py-2 text-xs hover:bg-[var(--bg-3)]
						{chatStore.currentSessionId === session.id
						? 'text-[var(--pub-accent)]'
						: 'text-[var(--text-secondary)]'}"
					onclick={() => selectSession(session.id)}
				>
					<span class="truncate">{session.title ?? `Session ${session.id.slice(0, 6)}`}</span>
					<span class="ml-2 flex-shrink-0 text-[10px] text-[var(--text-tertiary)]">
						{relativeTime(session.lastMessageAt)}
					</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
