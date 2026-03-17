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
	import { Bot } from '@lucide/svelte';
	import { Send } from '@lucide/svelte';

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
</script>

<div class="flex h-full flex-col">
	<!-- Messages area -->
	<div class="flex-1 overflow-y-auto p-4">
		<div class="mx-auto max-w-3xl flex flex-col gap-4">
			{#each chatStore.messages as msg (msg.id)}
				<MessageBubble message={msg} />
			{/each}

			<!-- Streaming messages -->
			{#each streamingList as sm (sm.taskId)}
				<div class="flex gap-3">
					<div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-[var(--accent-dim)]">
						<Bot class="h-3.5 w-3.5 text-[var(--pub-accent)]" />
					</div>
					<div class="max-w-[80%] rounded-lg bg-[var(--bg-2)] px-3 py-2">
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
				<textarea
					bind:value={inputText}
					onkeydown={handleKeydown}
					placeholder="Send a message..."
					rows="1"
					class="flex-1 resize-none rounded-lg border border-border-subtle bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
				></textarea>
				<VoiceButton />
				<button
					class="rounded-lg bg-[var(--pub-accent)] p-2 text-[var(--primary-foreground)] hover:opacity-90 transition-opacity disabled:opacity-50"
					onclick={handleSend}
					disabled={!inputText.trim() || sending}
					aria-label="Send"
				>
					<Send class="h-5 w-5" />
				</button>
			</div>
		</div>
	</div>
</div>
