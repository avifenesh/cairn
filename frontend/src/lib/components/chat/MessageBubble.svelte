<script lang="ts">
	import type { ChatMessage } from '$lib/types';
	import StreamingText from './StreamingText.svelte';
	import ToolCallChip from './ToolCallChip.svelte';
	import ReasoningBlock from './ReasoningBlock.svelte';
	import QuickMemoryButton from './QuickMemoryButton.svelte';
	import CreateTaskButton from './CreateTaskButton.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { Button } from '$lib/components/ui/button';
	import { Bot, User, Copy, Check, Volume2, Loader2, VolumeOff } from '@lucide/svelte';
	import { playTTS, stopTTS } from '$lib/utils/tts';

	let { message }: { message: ChatMessage } = $props();

	let copied = $state(false);
	let speaking = $state(false);

	async function copyContent() {
		await navigator.clipboard.writeText(message.content);
		copied = true;
		setTimeout(() => { copied = false; }, 2000);
	}

	async function toggleSpeak() {
		if (speaking) {
			stopTTS();
			speaking = false;
			return;
		}
		speaking = true;
		try {
			await playTTS(message.content);
		} catch {
			// TTS failed silently
		} finally {
			speaking = false;
		}
	}
</script>

<div class="group flex gap-3 {message.role === 'user' ? 'flex-row-reverse' : ''}">
	<div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg {message.role === 'user' ? 'bg-[var(--bg-2)]' : 'bg-[var(--accent-dim)]'}">
		{#if message.role === 'user'}
			<User class="h-3.5 w-3.5 text-[var(--text-secondary)]" />
		{:else}
			<Bot class="h-3.5 w-3.5 text-[var(--cairn-accent)]" />
		{/if}
	</div>
	<div class="relative max-w-[80%] rounded-lg px-4 py-3 {message.role === 'user' ? 'bg-[var(--bg-2)]' : 'bg-[var(--bg-1)] border border-border-subtle'}">
		{#if message.toolCalls && message.toolCalls.length > 0}
			<div class="mb-2 flex flex-wrap gap-1">
				{#each message.toolCalls as tc}
					<ToolCallChip toolName={tc.toolName} phase={tc.phase} args={tc.args} result={tc.result} error={tc.error} durationMs={tc.durationMs} isExternal={tc.isExternal} />
				{/each}
			</div>
		{/if}
		{#if message.reasoning && message.reasoning.length > 0}
			<ReasoningBlock steps={message.reasoning} />
		{/if}
		<StreamingText content={message.content} isStreaming={false} />
		<div class="mt-1.5 flex items-center justify-between">
			<time class="text-[10px] text-[var(--text-tertiary)] tabular-nums font-mono" datetime={message.createdAt}>
				{relativeTime(message.createdAt)}
			</time>
			<!-- Action bar — visible on hover -->
			<div class="flex items-center gap-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto transition-opacity">
				<Button
					variant="ghost"
					size="icon"
					class="h-6 w-6"
					onclick={copyContent}
					aria-label="Copy message"
					title={copied ? 'Copied!' : 'Copy'}
				>
					{#if copied}
						<Check class="h-3 w-3 text-[var(--color-success)]" />
					{:else}
						<Copy class="h-3 w-3 text-[var(--text-tertiary)]" />
					{/if}
				</Button>
				{#if message.role === 'assistant'}
					<Button
						variant="ghost"
						size="icon"
						class="h-6 w-6"
						onclick={toggleSpeak}
						aria-label={speaking ? 'Stop speaking' : 'Read aloud'}
						title={speaking ? 'Stop' : 'Read aloud'}
					>
						{#if speaking}
							<VolumeOff class="h-3 w-3 text-[var(--cairn-accent)]" />
						{:else}
							<Volume2 class="h-3 w-3 text-[var(--text-tertiary)]" />
						{/if}
					</Button>
					<span title="Remember this">
						<QuickMemoryButton content={message.content} />
					</span>
					<span title="Create task">
						<CreateTaskButton content={message.content} />
					</span>
				{/if}
			</div>
		</div>
	</div>
</div>
