<script lang="ts">
	import type { SessionEvent } from '$lib/types';
	import EventCard from './EventCard.svelte';
	import { tick } from 'svelte';

	let { events, streamingText = '', thinkingText = '', isCompleted = false, onFileClick,
		filter = 'all', searchQuery = '' }: {
		events: SessionEvent[];
		streamingText?: string;
		thinkingText?: string;
		isCompleted?: boolean;
		onFileClick?: (path: string) => void;
		filter?: 'all' | 'tools' | 'errors' | 'messages';
		searchQuery?: string;
	} = $props();

	let scrollEl: HTMLElement | undefined = $state();
	let autoScroll = $state(true);
	let userScrolled = $state(false);

	// Round separator marker type.
	interface RoundMarker {
		kind: 'round_separator';
		round: number;
		toolCalls: number;
		inputTokens: number;
		outputTokens: number;
	}
	type DisplayItem = SessionEvent | RoundMarker;

	// Aggregate, filter, and merge events for display.
	const displayItems = $derived.by((): DisplayItem[] => {
		const result: DisplayItem[] = [];
		let pendingThinking = '';
		let thinkingTimestamp = '';
		const sid = events[0]?.sessionId ?? '';

		const flushThinking = () => {
			if (pendingThinking) {
				result.push({
					sessionId: sid, eventType: 'thinking',
					payload: { text: pendingThinking },
					timestamp: thinkingTimestamp,
				} as SessionEvent);
				pendingThinking = '';
				thinkingTimestamp = '';
			}
		};

		for (const e of events) {
			if (e.eventType === 'text_delta' && !e.payload.author) continue;
			if (e.eventType === 'thinking') {
				if (!thinkingTimestamp) thinkingTimestamp = e.timestamp;
				pendingThinking += (e.payload.text as string) ?? '';
				continue;
			}
			flushThinking();

			// Replace round_complete with a separator marker.
			if (e.eventType === 'round_complete') {
				result.push({
					kind: 'round_separator',
					round: (e.payload.round as number) ?? 0,
					toolCalls: (e.payload.toolCalls as number) ?? 0,
					inputTokens: (e.payload.inputTokens as number) ?? 0,
					outputTokens: (e.payload.outputTokens as number) ?? 0,
				});
				continue;
			}

			result.push(e);
		}
		flushThinking();

		// Apply filter.
		let filtered = result;
		if (filter !== 'all') {
			filtered = filtered.filter(item => {
				if ('kind' in item) return true; // always show separators
				const e = item as SessionEvent;
				if (filter === 'tools') return e.eventType === 'tool_call' || e.eventType === 'tool_result';
				if (filter === 'errors') return e.eventType === 'tool_result' && e.payload.isError;
				if (filter === 'messages') return e.eventType === 'text_delta' || e.eventType === 'user_steer';
				return true;
			});
		}

		// Apply search.
		if (searchQuery.trim()) {
			const q = searchQuery.toLowerCase();
			filtered = filtered.filter(item => {
				if ('kind' in item) return false; // hide separators in search
				const p = (item as SessionEvent).payload;
				return String(p.toolName ?? '').toLowerCase().includes(q) ||
					String(p.text ?? '').toLowerCase().includes(q) ||
					String(p.output ?? '').toLowerCase().includes(q) ||
					String(p.content ?? '').toLowerCase().includes(q) ||
					String(p.path ?? '').toLowerCase().includes(q) ||
					String(p.error ?? '').toLowerCase().includes(q);
			});
		}

		return filtered;
	});

	// Track completed tool IDs.
	const completedToolIds = $derived(
		new Set(events.filter((e) => e.eventType === 'tool_result').map((e) => String(e.payload.toolId ?? '')))
	);

	// Auto-scroll.
	$effect(() => {
		if (displayItems.length && autoScroll && scrollEl) {
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

	function isRoundMarker(item: DisplayItem): item is RoundMarker {
		return 'kind' in item && item.kind === 'round_separator';
	}

	function formatTokens(n: number): string {
		if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
		return String(n);
	}
</script>

<div class="activity-stream" bind:this={scrollEl} onscroll={handleScroll}>
	{#each displayItems as item, i (i)}
		{#if isRoundMarker(item)}
			<div class="round-separator">
				<div class="sep-line"></div>
				<span class="sep-label">
					Round {item.round + 1} - {item.toolCalls} tools
					{#if item.inputTokens > 0}, {formatTokens(item.inputTokens)} in / {formatTokens(item.outputTokens)} out{/if}
				</span>
				<div class="sep-line"></div>
			</div>
		{:else}
			<EventCard event={item} {completedToolIds} {isCompleted} {onFileClick} />
		{/if}
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
	<button class="jump-btn" onclick={jumpToLatest}>Jump to latest</button>
{/if}

<style>
	.activity-stream {
		flex: 1; overflow-y: auto; padding: 0.5rem;
		display: flex; flex-direction: column; gap: 0.125rem;
	}
	.streaming-block {
		padding: 0.375rem 0.5rem; border-radius: 0.375rem;
		background: hsl(var(--muted) / 0.3); white-space: pre-wrap; word-break: break-word;
	}
	.thinking-block { border-left: 2px solid var(--text-tertiary, hsl(var(--muted-foreground))); }
	.cursor-blink { animation: blink 1s infinite; color: var(--cairn-accent, #60a5fa); }
	@keyframes blink { 0%, 50% { opacity: 1; } 51%, 100% { opacity: 0; } }
	.jump-btn {
		position: absolute; bottom: 0.5rem; left: 50%; transform: translateX(-50%);
		padding: 0.25rem 0.75rem; font-size: 0.75rem; border-radius: 9999px;
		background: var(--cairn-accent, #60a5fa); color: white; border: none;
		cursor: pointer; z-index: 10; box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
	}

	/* Round separator */
	.round-separator {
		display: flex; align-items: center; gap: 0.5rem;
		padding: 0.375rem 0; margin: 0.25rem 0;
	}
	.sep-line { flex: 1; height: 1px; background: hsl(var(--border)); }
	.sep-label {
		font-size: 0.625rem; color: var(--text-tertiary, hsl(var(--muted-foreground)));
		white-space: nowrap;
	}
</style>
