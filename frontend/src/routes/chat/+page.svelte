<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { chatStore } from '$lib/stores/chat.svelte';
	import { getSessionMessages, sendMessage } from '$lib/api/client';
	import ChatPanel from '$lib/components/chat/ChatPanel.svelte';

	// Load session from ?session= query param (e.g. linked from ops tasks)
	// Pre-fill message from ?msg= query param (e.g. from home page quick chat)
	onMount(() => {
		const sessionId = $page.url.searchParams.get('session');
		if (sessionId && sessionId !== chatStore.currentSessionId) {
			chatStore.setCurrentSession(sessionId);
			chatStore.clearStreaming();
			getSessionMessages(sessionId)
				.then((res) => chatStore.setMessages(res.items))
				.catch(() => chatStore.setMessages([]));
		}

		const msg = $page.url.searchParams.get('msg');
		if (msg) {
			// Clear the query param to avoid re-sending on navigation
			history.replaceState({}, '', '/chat');
			// Pre-fill the chat input via store
			chatStore.setPendingMessage(msg);
		}
	});
</script>

<ChatPanel />
