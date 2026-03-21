<script lang="ts">
	import type { SessionEvent } from '$lib/types';
	import EventCard from './EventCard.svelte';
	import { tick } from 'svelte';

	let { events, streamingText = '', thinkingText = '' }: {
		events: SessionEvent[];
		streamingText?: string;
		thinkingText?: string;
	} = $props();

	let scrollEl: HTMLElement | undefined = $state();
	let autoScroll = $state(true);
	let userScrolled = $state(false);

	// Filter out text_delta events (they're shown as aggregated streaming text).
	const displayEvents = $derived(
		events.filter((e) => e.eventType !== 'text_delta' && e.eventType !== 'thinking')
	);

	// Auto-scroll to bottom on new events.
	$effect(() => {
		if (displayEvents.length && autoScroll && scrollEl) {
			tick().then(() => {
				scrollEl?.scrollTo({ top: scrollEl.scrollHeight, behavior: 'smooth' });
			});
		}
	});

	function handleScroll() {
		if (!scrollEl) return;
		const atBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight < 80;
		autoScroll = atBottom;
		userScrolled = !atBottom;
	}

	function jumpToLatest() {
		autoScroll = true;
		scrollEl?.scrollTo({ top: scrollEl.scrollHeight, behavior: 'smooth' });
	}
</script>

<div class="activity-stream" bind:this={scrollEl} onscroll={handleScroll}>
	{#each displayEvents as event (event.timestamp + event.eventType + (event.payload.toolId ?? ''))}
		<EventCard {event} />
	{/each}

	{#if thinkingText}
		<div class="streaming-block thinking-block">
			<span class="text-xs text-muted-foreground italic">{thinkingText.slice(-200)}</span>
		</div>
	{/if}

	{#if streamingText}
		<div class="streaming-block">
			<span class="text-sm">{streamingText}</span>
			<span class="cursor-blink">|</span>
		</div>
	{/if}
</div>

{#if userScrolled}
	<button class="jump-btn" onclick={jumpToLatest}>
		Jump to latest
	</button>
{/if}

<style>
	.activity-stream {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}
	.streaming-block {
		padding: 0.375rem 0.5rem;
		border-radius: 0.375rem;
		background: hsl(var(--muted) / 0.3);
		white-space: pre-wrap;
		word-break: break-word;
	}
	.thinking-block {
		border-left: 2px solid var(--text-tertiary, hsl(var(--muted-foreground)));
	}
	.cursor-blink {
		animation: blink 1s infinite;
		color: var(--cairn-accent, #60a5fa);
	}
	@keyframes blink {
		0%, 50% { opacity: 1; }
		51%, 100% { opacity: 0; }
	}
	.jump-btn {
		position: absolute;
		bottom: 0.5rem;
		left: 50%;
		transform: translateX(-50%);
		padding: 0.25rem 0.75rem;
		font-size: 0.75rem;
		border-radius: 9999px;
		background: var(--cairn-accent, #60a5fa);
		color: white;
		border: none;
		cursor: pointer;
		z-index: 10;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
	}
</style>
