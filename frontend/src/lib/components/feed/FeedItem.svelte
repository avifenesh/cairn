<script lang="ts">
	import type { FeedItem } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { markRead } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { createSwipeToDismiss } from '$lib/utils/touch';

	let { item }: { item: FeedItem } = $props();

	const swipe = createSwipeToDismiss(() => {
		feedStore.markItemRead(item.id);
		markRead(item.id).catch(() => {});
	});

	async function handleClick() {
		if (!item.isRead) {
			feedStore.markItemRead(item.id);
			await markRead(item.id).catch(() => {});
		}
	}

	const translateStyle = $derived(
		swipe.state.swiping ? `transform: translateX(${swipe.state.offsetX}px); opacity: ${1 - Math.abs(swipe.state.offsetX) / 150}` : '',
	);
</script>

<a
	href={item.url ?? '#'}
	target={item.url ? '_blank' : undefined}
	rel="noopener"
	class="flex items-start gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 transition-colors duration-[var(--dur-fast)] hover:bg-[var(--bg-2)]"
	class:opacity-60={item.isRead}
	style={translateStyle}
	onclick={handleClick}
	ontouchstart={swipe.handleTouchStart}
	ontouchmove={swipe.handleTouchMove}
	ontouchend={swipe.handleTouchEnd}
>
	<span
		class="mt-0.5 h-2 w-2 flex-shrink-0 rounded-full"
		style="background: var(--src-{item.source}, var(--text-tertiary))"
	></span>
	<div class="min-w-0 flex-1">
		<p class="truncate text-sm text-[var(--text-primary)]">{item.title}</p>
		<p class="mt-0.5 text-xs text-[var(--text-tertiary)]">
			{item.source} &middot; {relativeTime(item.createdAt)}
		</p>
	</div>
	{#if !item.isRead}
		<span class="mt-1.5 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-[var(--pub-accent)]"></span>
	{/if}
</a>
