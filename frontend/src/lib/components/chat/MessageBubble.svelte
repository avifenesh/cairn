<script lang="ts">
	import type { ChatMessage } from '$lib/types';
	import StreamingText from './StreamingText.svelte';
	import ToolCallChip from './ToolCallChip.svelte';
	import ReasoningBlock from './ReasoningBlock.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { Bot, User } from '@lucide/svelte';

	let { message }: { message: ChatMessage } = $props();
</script>

<div class="flex gap-3 {message.role === 'user' ? 'flex-row-reverse' : ''}">
	<div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg {message.role === 'user' ? 'bg-[var(--bg-2)]' : 'bg-[var(--accent-dim)]'}">
		{#if message.role === 'user'}
			<User class="h-3.5 w-3.5 text-[var(--text-secondary)]" />
		{:else}
			<Bot class="h-3.5 w-3.5 text-[var(--cairn-accent)]" />
		{/if}
	</div>
	<div class="max-w-[80%] rounded-lg px-4 py-3 {message.role === 'user' ? 'bg-[var(--bg-2)]' : 'bg-[var(--bg-1)] border border-border-subtle'}">
		{#if message.toolCalls && message.toolCalls.length > 0}
			<div class="mb-2 flex flex-wrap gap-1">
				{#each message.toolCalls as tc}
					<ToolCallChip toolName={tc.toolName} phase={tc.phase} />
				{/each}
			</div>
		{/if}
		{#if message.reasoning && message.reasoning.length > 0}
			<ReasoningBlock steps={message.reasoning} />
		{/if}
		<StreamingText content={message.content} isStreaming={false} />
		<time class="mt-1.5 block text-[10px] text-[var(--text-tertiary)] tabular-nums font-mono" datetime={message.createdAt}>
			{relativeTime(message.createdAt)}
		</time>
	</div>
</div>
