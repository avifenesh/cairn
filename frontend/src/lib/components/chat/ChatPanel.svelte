<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { sendMessage, getSessions, getSessionMessages } from '$lib/api/client';
	import { chatStore } from '$lib/stores/chat.svelte';
	import MessageBubble from './MessageBubble.svelte';
	import StreamingText from './StreamingText.svelte';
	import ToolCallChip from './ToolCallChip.svelte';
	import ModeSelector from './ModeSelector.svelte';
	import SessionPicker from './SessionPicker.svelte';
	import VoiceButton from './VoiceButton.svelte';
	import ActiveSkillChip from './ActiveSkillChip.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Bot, Send, Loader2, Brain } from '@lucide/svelte';

	let inputText = $state('');
	let messagesEnd: HTMLDivElement;
	let textareaEl: HTMLTextAreaElement;
	let sending = $state(false);

	onMount(async () => {
		try {
			const res = await getSessions();
			chatStore.setSessions(res.items);
			// Auto-load last session if one was saved
			if (chatStore.currentSessionId && res.items.some((s) => s.id === chatStore.currentSessionId)) {
				const msgs = await getSessionMessages(chatStore.currentSessionId);
				chatStore.setMessages(msgs.items);
			}
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
			if (res.sessionId && !chatStore.currentSessionId) {
				chatStore.setCurrentSession(res.sessionId);
			}
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

	// Mode-specific accent colors and placeholders
	const modeColors: Record<string, string> = {
		talk: 'var(--cairn-accent)',
		work: 'var(--color-warning)',
		coding: 'var(--src-x)',
	};
	const modePlaceholders: Record<string, string> = {
		talk: 'Send a message...',
		work: 'What needs to get done?',
		coding: 'Describe a coding task...',
	};
	const modeColor = $derived(modeColors[chatStore.mode] ?? 'var(--cairn-accent)');
	const modePlaceholder = $derived(modePlaceholders[chatStore.mode] ?? 'Send a message...');

	const modeSuggestions: Record<string, string[]> = {
		talk: ['Summarize my unread feed', 'What do you remember about me?', 'Plan a weekend trip', 'Triage my notifications'],
		work: ['Show my pending tasks', 'Create a daily digest', 'Draft a status update', 'What needs attention?'],
		coding: ['Review the latest diff', 'Find TODOs in the codebase', 'Write tests for...', 'Explain this error...'],
	};
	const suggestions = $derived(modeSuggestions[chatStore.mode] ?? modeSuggestions.talk);

	// Auto-scroll when messages change (new message or session loaded)
	$effect(() => {
		// Track message count to trigger scroll
		const _ = chatStore.messages.length + streamingList.length;
		if (_ > 0) {
			tick().then(scrollToBottom);
		}
	});
</script>

<div class="flex h-full flex-col">
	<!-- Messages area -->
	<div class="flex-1 overflow-y-auto">
		<div class="mx-auto max-w-3xl flex flex-col gap-4 p-4">
			{#if !hasMessages}
				<div class="flex flex-col items-center justify-center py-16 text-center">
					<div class="flex h-12 w-12 items-center justify-center rounded-xl bg-[var(--accent-dim)] mb-4">
						<Bot class="h-6 w-6 text-[var(--cairn-accent)]" />
					</div>
					<h2 class="text-lg font-medium text-[var(--text-primary)] mb-1">What can I help with?</h2>
					<p class="text-sm text-[var(--text-tertiary)] max-w-sm">
						I can write code, manage tasks, search your memory, plan trips, triage emails, and more.
					</p>
					<div class="flex flex-wrap justify-center gap-2 max-w-md mt-6">
						{#each suggestions as suggestion}
							<button
								class="rounded-lg border border-border-subtle bg-[var(--bg-1)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-2)] hover:text-[var(--text-primary)] transition-colors"
								onclick={() => { inputText = suggestion; textareaEl?.focus(); }}
								type="button"
							>
								{suggestion}
							</button>
						{/each}
					</div>
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
									<ToolCallChip toolName={tc.toolName} phase={tc.phase} args={tc.args} result={tc.result} error={tc.error} durationMs={tc.durationMs} />
								{/each}
							</div>
						{/if}
						{#if sm.reasoning.length > 0}
							<details class="mb-2" open>
								<summary class="flex items-center gap-1.5 text-[10px] text-[var(--text-tertiary)] cursor-pointer hover:text-[var(--text-secondary)]">
									<Brain class="h-3 w-3" />
									<span>Thinking ({sm.reasoning.length} step{sm.reasoning.length !== 1 ? 's' : ''})</span>
								</summary>
								<div class="mt-1.5 border-l-2 border-[var(--cairn-accent)]/20 pl-3 text-xs text-[var(--text-secondary)] space-y-1">
									{#each sm.reasoning as step}
										<p><span class="font-mono text-[var(--cairn-accent)]">R{step.round}</span> {step.thought}</p>
									{/each}
								</div>
							</details>
						{/if}
						{#if sm.content}
							<StreamingText content={sm.content} isStreaming={sm.isStreaming} />
						{:else if sm.isStreaming}
							<!-- Thinking/waiting indicator -->
							<div class="flex items-center gap-2 text-sm text-[var(--text-tertiary)]">
								{#if sm.reasoning.length > 0 || sm.toolCalls.length > 0}
									<span>Thinking</span>
								{/if}
								<span class="thinking-dots flex gap-0.5">
									<span class="h-1.5 w-1.5 rounded-full bg-[var(--cairn-accent)]"></span>
									<span class="h-1.5 w-1.5 rounded-full bg-[var(--cairn-accent)]"></span>
									<span class="h-1.5 w-1.5 rounded-full bg-[var(--cairn-accent)]"></span>
								</span>
							</div>
						{/if}
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
				<ActiveSkillChip />
			</div>

			<div class="flex items-end gap-2">
				<div
					class="flex-1 rounded-lg border bg-[var(--bg-0)] transition-colors focus-within:ring-1"
					style="border-color: color-mix(in srgb, {modeColor} 25%, transparent); --tw-ring-color: color-mix(in srgb, {modeColor} 30%, transparent)"
				>
					<textarea
						bind:this={textareaEl}
						bind:value={inputText}
						onkeydown={handleKeydown}
						placeholder={modePlaceholder}
						rows="1"
						class="w-full resize-none bg-transparent px-3 py-2.5 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none"
					></textarea>
				</div>
				<VoiceButton />
				<Button
					size="icon"
					class="h-10 w-10 rounded-lg"
					style="background-color: {modeColor}"
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
