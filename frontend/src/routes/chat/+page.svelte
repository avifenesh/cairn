<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { chatStore } from '$lib/stores/chat.svelte';
	import { getSessionMessages } from '$lib/api/client';
	import ChatPanel from '$lib/components/chat/ChatPanel.svelte';

	// Load session from ?session= query param (e.g. linked from ops tasks)
	onMount(() => {
		const sessionId = $page.url.searchParams.get('session');
		if (sessionId && sessionId !== chatStore.currentSessionId) {
			chatStore.setCurrentSession(sessionId);
			chatStore.clearStreaming();
			getSessionMessages(sessionId)
				.then((res) => chatStore.setMessages(res.items))
				.catch(() => chatStore.setMessages([]));
		}
	});
</script>

<ChatPanel />
