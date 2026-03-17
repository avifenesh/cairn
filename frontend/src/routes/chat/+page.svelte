<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { sendMessage, getSessions } from '$lib/api/client';
	import { chatStore } from '$lib/stores/chat.svelte';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { relativeTime } from '$lib/utils/time';
	import { Send, Mic, Bot, User, Wrench, Brain } from '@lucide/svelte';
	import type { ChatMode } from '$lib/types';

	let inputText = $state('');
	let messagesEnd: HTMLDivElement;
	let sending = $state(false);

	const modes: { value: ChatMode; label: string }[] = [
		{ value: 'talk', label: 'Talk' },
		{ value: 'work', label: 'Work' },
		{ value: 'coding', label: 'Coding' },
	];

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
			chatStore.addUserMessage('[Error sending message]');
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

	// All display messages: committed + streaming
	const allMessages = $derived(() => {
		const committed = chatStore.messages;
		const streaming = [...chatStore.streamingMessages.values()];
		return { committed, streaming };
	});
</script>

<div class="flex h-full flex-col">
	<!-- Messages area -->
	<div class="flex-1 overflow-y-auto p-4">
		<div class="mx-auto max-w-3xl flex flex-col gap-4">
			{#each allMessages().committed as msg (msg.id)}
				<div class="flex gap-3 {msg.role === 'user' ? 'flex-row-reverse' : ''}">
					<div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full {msg.role === 'user' ? 'bg-[var(--bg-3)]' : 'bg-[var(--accent-dim)]'}">
						{#if msg.role === 'user'}
							<User class="h-3.5 w-3.5 text-[var(--text-secondary)]" />
						{:else}
							<Bot class="h-3.5 w-3.5 text-[var(--pub-accent)]" />
						{/if}
					</div>
					<div class="max-w-[80%] rounded-lg px-3 py-2 {msg.role === 'user' ? 'bg-[var(--bg-3)]' : 'bg-[var(--bg-2)]'}">
						{#if msg.toolCalls && msg.toolCalls.length > 0}
							<div class="mb-2 flex flex-wrap gap-1">
								{#each msg.toolCalls as tc}
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--bg-3)] px-2 py-0.5 text-[10px] text-[var(--text-tertiary)]">
										<Wrench class="h-2.5 w-2.5" />
										{tc.toolName}
									</span>
								{/each}
							</div>
						{/if}
						{#if msg.reasoning && msg.reasoning.length > 0}
							<details class="mb-2">
								<summary class="flex cursor-pointer items-center gap-1 text-[10px] text-[var(--text-tertiary)]">
									<Brain class="h-2.5 w-2.5" />
									{msg.reasoning.length} reasoning step{msg.reasoning.length !== 1 ? 's' : ''}
								</summary>
								<div class="mt-1 border-l-2 border-border-subtle pl-2 text-xs text-[var(--text-secondary)]">
									{#each msg.reasoning as step}
										<p class="mb-1"><strong>Round {step.round}:</strong> {step.thought}</p>
									{/each}
								</div>
							</details>
						{/if}
						<div class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)]">
							{@html renderMarkdown(msg.content)}
						</div>
						<time class="mt-1 block text-[10px] text-[var(--text-tertiary)]">
							{relativeTime(msg.createdAt)}
						</time>
					</div>
				</div>
			{/each}

			<!-- Streaming messages -->
			{#each allMessages().streaming as sm (sm.taskId)}
				<div class="flex gap-3">
					<div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-[var(--accent-dim)]">
						<Bot class="h-3.5 w-3.5 text-[var(--pub-accent)]" />
					</div>
					<div class="max-w-[80%] rounded-lg bg-[var(--bg-2)] px-3 py-2">
						{#if sm.toolCalls.length > 0}
							<div class="mb-2 flex flex-wrap gap-1">
								{#each sm.toolCalls as tc}
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--bg-3)] px-2 py-0.5 text-[10px] text-[var(--text-tertiary)]">
										<Wrench class="h-2.5 w-2.5" />
										{tc.toolName}
									</span>
								{/each}
							</div>
						{/if}
						<div class="prose prose-sm prose-invert max-w-none text-sm text-[var(--text-primary)]">
							{@html renderMarkdown(sm.content || '...')}
							{#if sm.isStreaming}
								<span class="inline-block h-4 w-0.5 animate-pulse bg-[var(--pub-accent)]"></span>
							{/if}
						</div>
					</div>
				</div>
			{/each}

			<div bind:this={messagesEnd}></div>
		</div>
	</div>

	<!-- Input area -->
	<div class="border-t border-border-subtle bg-[var(--bg-1)] p-4">
		<div class="mx-auto max-w-3xl">
			<!-- Mode selector -->
			<div class="mb-2 flex gap-1">
				{#each modes as m}
					<button
						class="rounded-md px-2.5 py-1 text-xs transition-colors duration-[var(--dur-fast)]
							{chatStore.mode === m.value
							? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
							: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
						onclick={() => chatStore.setMode(m.value)}
					>
						{m.label}
					</button>
				{/each}
			</div>

			<div class="flex items-end gap-2">
				<textarea
					bind:value={inputText}
					onkeydown={handleKeydown}
					placeholder="Send a message..."
					rows="1"
					class="flex-1 resize-none rounded-lg border border-border-subtle bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--pub-accent)] focus:outline-none"
				></textarea>
				<button
					class="rounded-lg p-2 text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] transition-colors"
					aria-label="Voice input"
				>
					<Mic class="h-5 w-5" />
				</button>
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
