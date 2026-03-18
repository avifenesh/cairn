<script lang="ts">
	import { chatStore } from '$lib/stores/chat.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import { Wrench, Brain, PanelRightClose } from '@lucide/svelte';

	let { open = false, onclose }: { open: boolean; onclose: () => void } = $props();

	const activeStream = $derived(chatStore.activeStream);
	const lastMessage = $derived(chatStore.messages.at(-1));

	const toolCalls = $derived(
		activeStream?.toolCalls ?? (lastMessage?.role === 'assistant' ? lastMessage.toolCalls : []) ?? [],
	);
	const reasoning = $derived(
		activeStream?.reasoning ?? (lastMessage?.role === 'assistant' ? lastMessage.reasoning : []) ?? [],
	);
</script>

{#if open}
	<aside class="hidden md:flex w-72 flex-col border-l border-border-subtle bg-[var(--bg-1)] overflow-y-auto">
		<div class="flex items-center justify-between border-b border-border-subtle px-3 h-[var(--header-h)]">
			<span class="text-xs font-medium text-[var(--text-secondary)]">Context</span>
			<Button variant="ghost" size="icon" class="h-7 w-7" onclick={onclose} aria-label="Close context panel">
				<PanelRightClose class="h-3.5 w-3.5 text-[var(--text-tertiary)]" />
			</Button>
		</div>

		<div class="flex-1 p-3">
			{#if toolCalls.length > 0}
				<section class="mb-4">
					<h3 class="mb-2 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">
						<Wrench class="h-3 w-3" />
						Tools
						<Badge variant="secondary" class="h-3.5 px-1 text-[9px]">{toolCalls.length}</Badge>
					</h3>
					<div class="flex flex-col gap-1.5">
						{#each toolCalls as tc, i (i)}
							<div class="rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs">
								<div class="flex items-center gap-1.5">
									<span class="font-mono font-medium text-[var(--text-primary)]">{tc.toolName}</span>
									<Badge variant={tc.phase === 'start' ? 'default' : 'outline'} class="h-3.5 px-1 text-[9px]">{tc.phase}</Badge>
								</div>
								{#if tc.result}
									<p class="mt-1 truncate text-[var(--text-tertiary)]">{tc.result.slice(0, 80)}</p>
								{/if}
							</div>
						{/each}
					</div>
				</section>
			{/if}

			{#if toolCalls.length > 0 && reasoning.length > 0}
				<Separator class="mb-4" />
			{/if}

			{#if reasoning.length > 0}
				<section class="mb-4">
					<h3 class="mb-2 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">
						<Brain class="h-3 w-3" />
						Reasoning
						<Badge variant="secondary" class="h-3.5 px-1 text-[9px]">{reasoning.length}</Badge>
					</h3>
					<div class="flex flex-col gap-1.5">
						{#each reasoning as step (step.round)}
							<div class="rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs">
								<span class="font-mono text-[var(--cairn-accent)]">R{step.round}</span>
								<p class="mt-0.5 text-[var(--text-secondary)]">{step.thought}</p>
							</div>
						{/each}
					</div>
				</section>
			{/if}

			{#if toolCalls.length === 0 && reasoning.length === 0}
				<div class="py-12 text-center">
					<Wrench class="mx-auto mb-2 h-6 w-6 text-[var(--text-tertiary)] opacity-30" />
					<p class="text-xs text-[var(--text-tertiary)]">
						Tool calls and reasoning appear here during conversations
					</p>
				</div>
			{/if}
		</div>
	</aside>
{/if}
