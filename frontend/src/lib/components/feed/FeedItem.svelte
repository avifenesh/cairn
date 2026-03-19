<script lang="ts">
	import type { FeedItem } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { markRead } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { createSwipeToDismiss, SWIPE_THRESHOLD } from '$lib/utils/touch.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Check } from '@lucide/svelte';

	let { item }: { item: FeedItem } = $props();

	const VISUAL_RANGE = SWIPE_THRESHOLD * 1.5;

	function markItemRead() {
		if (item.isRead) return;
		feedStore.markItemRead(item.id);
		markRead(item.id).catch(() => {});
	}

	const swipe = createSwipeToDismiss(markItemRead);

	function handleClick() {
		markItemRead();
	}

	const translateStyle = $derived(
		swipe.state.swiping ? `transform: translateX(${swipe.state.offsetX}px); opacity: ${Math.max(0, 1 - Math.abs(swipe.state.offsetX) / VISUAL_RANGE)}` : '',
	);
</script>

<a
	href={item.url ?? '#'}
	target={item.url ? '_blank' : undefined}
	rel="noopener"
	class="group flex items-start gap-3 rounded-lg border border-transparent px-3 py-2.5 transition-all duration-[var(--dur-fast)] hover:bg-[var(--bg-1)] hover:border-border-subtle"
	class:opacity-50={item.isRead}
	style={translateStyle}
	onclick={handleClick}
	ontouchstart={swipe.handleTouchStart}
	ontouchmove={swipe.handleTouchMove}
	ontouchend={swipe.handleTouchEnd}
	ontouchcancel={swipe.handleTouchCancel}
>
	<!-- Source indicator -->
	<span
		class="mt-1 h-2 w-2 flex-shrink-0 rounded-full ring-2 ring-[var(--bg-0)]"
		style="background: var(--src-{item.source}, var(--text-tertiary))"
	></span>

	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-2">
			<p class="truncate text-sm font-medium text-[var(--text-primary)] group-hover:text-[var(--cairn-accent)] transition-colors">{item.title}</p>
		</div>
		<div class="mt-0.5 flex items-center gap-1.5 text-[11px] text-[var(--text-tertiary)]">
			<Badge variant="outline" class="h-4 px-1 text-[10px] font-normal border-border-subtle">
				{item.source}
			</Badge>
			<span>{item.kind}</span>
			<span>&middot;</span>
			<time datetime={item.createdAt}>{relativeTime(item.createdAt)}</time>
		</div>
	</div>

	{#if !item.isRead}
		<button
			class="flex-shrink-0 rounded-md p-1 opacity-0 group-hover:opacity-100 transition-opacity duration-[var(--dur-fast)]
				text-[var(--text-tertiary)] hover:text-[var(--cairn-accent)] hover:bg-[var(--bg-2)]"
			title="Mark as read"
			onclick={(e) => { e.preventDefault(); e.stopPropagation(); markItemRead(); }}
		>
			<Check class="h-3.5 w-3.5" />
		</button>
		<span class="mt-2 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-[var(--cairn-accent)]"></span>
	{/if}
</a>
