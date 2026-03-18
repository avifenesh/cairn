<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { sendMessage, getSessions } from '$lib/api/client';
	import { chatStore } from '$lib/stores/chat.svelte';
	import MessageBubble from './MessageBubble.svelte';
	import StreamingText from './StreamingText.svelte';
	import ToolCallChip from './ToolCallChip.svelte';
	import ModeSelector from './ModeSelector.svelte';
	import SessionPicker from './SessionPicker.svelte';
	import VoiceButton from './VoiceButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Bot, Send, Loader2 } from '@lucide/svelte';

	let inputText = $state('');
	let messagesEnd: HTMLDivElement;
	let sending = $state(false);

	onMount(async () => {
		try {
			const res = await getSessions();
			chatStore.setSessions(res.items);
		} catch {
			// ignore
		}
	});

	async function handleSend() {
		const text = inputText.trim();
		if (!text || sending) return;

		inputText = '';
		sending = true;
		chatStore.addUserMessage(text);
		await tick();
		scrollToBottom();

		try {
			const res = await sendMessage(text, chatStore.mode, chatStore.currentSessionId ?? undefined);
			chatStore.startStreaming(res.taskId);
		} catch {
			// error handled via notification
		} finally {
			sending = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSend();
		}
	}

	function scrollToBottom() {
		messagesEnd?.scrollIntoView({ behavior: 'smooth' });
	}

	const streamingList = $derived([...chatStore.streamingMessages.values()]);
	const hasMessages = $derived(chatStore.messages.length > 0 || streamingList.length > 0);
</script>

<div class="flex h-full flex-col">
	<!-- Messages area -->
	<div class="flex-1 overflow-y-auto">
		<div class="mx-auto max-w-3xl flex flex-col gap-4 p-4">
			{#if !hasMessages}
				<div class="flex flex-col items-center justify-center py-24 text-center">
					<div class="flex h-12 w-12 items-center justify-center rounded-xl bg-[var(--accent-dim)] mb-4">
						<Bot class="h-6 w-6 text-[var(--cairn-accent)]" />
					</div>
					<h2 class="text-lg font-medium text-[var(--text-primary)] mb-1">What can I help with?</h2>
					<p class="text-sm text-[var(--text-tertiary)] max-w-sm">
						I can write code, manage tasks, search your memory, plan trips, triage emails, and more.
					</p>
				</div>
			{/if}

			{#each chatStore.messages as msg, i (msg.id)}
				<div class="animate-in" style="animation-delay: {Math.min(i * 20, 200)}ms">
					<MessageBubble message={msg} />
				</div>
			{/each}

			<!-- Streaming messages -->
			{#each streamingList as sm (sm.taskId)}
				<div class="flex gap-3 animate-in">
					<div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg bg-[var(--accent-dim)]">
						<Bot class="h-3.5 w-3.5 text-[var(--cairn-accent)]" />
					</div>
					<div class="max-w-[80%] rounded-lg bg-[var(--bg-1)] border border-border-subtle px-4 py-3">
						{#if sm.toolCalls.length > 0}
							<div class="mb-2 flex flex-wrap gap-1">
								{#each sm.toolCalls as tc}
									<ToolCallChip toolName={tc.toolName} phase={tc.phase} />
								{/each}
							</div>
						{/if}
						<StreamingText content={sm.content || '...'} isStreaming={sm.isStreaming} />
					</div>
				</div>
			{/each}

			<div bind:this={messagesEnd}></div>
		</div>
	</div>

	<!-- Input area -->
	<div class="border-t border-border-subtle bg-[var(--bg-1)] p-4">
		<div class="mx-auto max-w-3xl">
			<div class="mb-2 flex items-center gap-2">
				<ModeSelector />
				<SessionPicker />
			</div>

			<div class="flex items-end gap-2">
				<div class="flex-1 rounded-lg border border-border-subtle bg-[var(--bg-0)] focus-within:border-[var(--cairn-accent)] focus-within:ring-1 focus-within:ring-[var(--cairn-accent)]/30 transition-colors">
					<textarea
						bind:value={inputText}
						onkeydown={handleKeydown}
						placeholder="Send a message..."
						rows="1"
						class="w-full resize-none bg-transparent px-3 py-2.5 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none"
					></textarea>
				</div>
				<VoiceButton />
				<Button
					size="icon"
					class="h-10 w-10 rounded-lg"
					onclick={handleSend}
					disabled={!inputText.trim() || sending}
					aria-label="Send"
				>
					{#if sending}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<Send class="h-4 w-4" />
					{/if}
				</Button>
			</div>
		</div>
	</div>
</div>
