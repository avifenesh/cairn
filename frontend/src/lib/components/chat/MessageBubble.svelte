<script lang="ts">
	import type { ChatMessage } from '$lib/types';
	import StreamingText from './StreamingText.svelte';
	import ToolCallChip from './ToolCallChip.svelte';
	import ReasoningBlock from './ReasoningBlock.svelte';
	import QuickMemoryButton from './QuickMemoryButton.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { Button } from '$lib/components/ui/button';
	import { Bot, User, Copy, Check } from '@lucide/svelte';

	let { message }: { message: ChatMessage } = $props();

	let copied = $state(false);

	async function copyContent() {
		await navigator.clipboard.writeText(message.content);
		copied = true;
		setTimeout(() => { copied = false; }, 2000);
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
					<ToolCallChip toolName={tc.toolName} phase={tc.phase} args={tc.args} result={tc.result} error={tc.error} durationMs={tc.durationMs} />
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
			<div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
				<Button
					variant="ghost"
					size="icon"
					class="h-6 w-6"
					onclick={copyContent}
					aria-label="Copy message"
				>
					{#if copied}
						<Check class="h-3 w-3 text-[var(--color-success)]" />
					{:else}
						<Copy class="h-3 w-3 text-[var(--text-tertiary)]" />
					{/if}
				</Button>
				{#if message.role === 'assistant'}
					<QuickMemoryButton content={message.content} />
				{/if}
			</div>
		</div>
	</div>
</div>
