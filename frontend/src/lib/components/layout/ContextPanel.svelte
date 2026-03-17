<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { Wrench, Brain, X, PanelRightClose } from '@lucide/svelte';

	let { open = false, onclose }: { open: boolean; onclose: () => void } = $props();

	const activeStream = $derived(chatStore.activeStream);
	const lastMessage = $derived(chatStore.messages.at(-1));

	// Show tool calls and reasoning from active stream or last assistant message
	const toolCalls = $derived(
		activeStream?.toolCalls ?? (lastMessage?.role === 'assistant' ? lastMessage.toolCalls : []) ?? [],
	);
	const reasoning = $derived(
		activeStream?.reasoning ?? (lastMessage?.role === 'assistant' ? lastMessage.reasoning : []) ?? [],
	);
</script>

{#if open}
	<aside class="hidden md:flex w-72 flex-col border-l border-border-subtle bg-[var(--bg-1)] overflow-y-auto">
		<div class="flex items-center justify-between border-b border-border-subtle px-3 py-2">
			<span class="text-xs font-medium text-[var(--text-secondary)]">Context</span>
			<button
				class="rounded p-1 hover:bg-[var(--bg-3)] transition-colors duration-[var(--dur-fast)]"
				onclick={onclose}
				aria-label="Close context panel"
			>
				<PanelRightClose class="h-3.5 w-3.5 text-[var(--text-tertiary)]" />
			</button>
		</div>

		<div class="flex-1 p-3">
			{#if toolCalls.length > 0}
				<section class="mb-4">
					<h3 class="mb-2 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">
						<Wrench class="h-3 w-3" />
						Tool Calls ({toolCalls.length})
					</h3>
					<div class="flex flex-col gap-1.5">
						{#each toolCalls as tc, i (i)}
							<div class="rounded-md bg-[var(--bg-2)] px-2.5 py-1.5 text-xs">
								<span class="font-medium text-[var(--text-primary)]">{tc.toolName}</span>
								<span class="ml-1 text-[var(--text-tertiary)]">({tc.phase})</span>
								{#if tc.result}
									<p class="mt-1 truncate text-[var(--text-secondary)]">{tc.result.slice(0, 80)}</p>
								{/if}
							</div>
						{/each}
					</div>
				</section>
			{/if}

			{#if reasoning.length > 0}
				<section class="mb-4">
					<h3 class="mb-2 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">
						<Brain class="h-3 w-3" />
						Reasoning ({reasoning.length})
					</h3>
					<div class="flex flex-col gap-1.5">
						{#each reasoning as step (step.round)}
							<div class="rounded-md bg-[var(--bg-2)] px-2.5 py-1.5 text-xs">
								<span class="font-medium text-[var(--text-tertiary)]">Round {step.round}</span>
								<p class="mt-0.5 text-[var(--text-secondary)]">{step.thought}</p>
							</div>
						{/each}
					</div>
				</section>
			{/if}

			{#if toolCalls.length === 0 && reasoning.length === 0}
				<p class="py-8 text-center text-xs text-[var(--text-tertiary)]">
					Tool calls and reasoning will appear here during conversations.
				</p>
			{/if}
		</div>
	</aside>
{/if}
