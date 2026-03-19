<script lang="ts">
	import type { FeedItem } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { markRead, archiveFeedItem } from '$lib/api/client';
	import { feedStore } from '$lib/stores/feed.svelte';
	import { createSwipeToDismiss, SWIPE_THRESHOLD } from '$lib/utils/touch.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Check, Archive, Trash2 } from '@lucide/svelte';

	let { item, ondelete }: { item: FeedItem; ondelete?: (id: string) => void } = $props();

	const VISUAL_RANGE = SWIPE_THRESHOLD * 1.5;

	function markItemRead() {
		if (item.isRead) return;
		feedStore.markItemRead(item.id);
		markRead(item.id).catch(() => {});
	}

	function archiveItem(e: Event) {
		e.preventDefault();
		e.stopPropagation();
		feedStore.archiveItem(item.id);
		archiveFeedItem(item.id).catch(() => {});
	}

	function deleteItem(e: Event) {
		e.preventDefault();
		e.stopPropagation();
		ondelete?.(item.id);
	}

	const swipe = createSwipeToDismiss(markItemRead);

	function handleClick() {
		markItemRead();
	}

	const translateStyle = $derived(
		swipe.state.swiping ? `transform: translateX(${swipe.state.offsetX}px); opacity: ${Math.max(0, 1 - Math.abs(swipe.state.offsetX) / VISUAL_RANGE)}` : '',
	);

	const sourceColors: Record<string, string> = {
		github: 'var(--src-github, #6e40c9)',
		hn: 'var(--src-hn, #ff6600)',
		reddit: 'var(--src-reddit, #ff4500)',
		npm: 'var(--src-npm, #cb3837)',
		crates: 'var(--src-crates, #e57324)',
		gmail: 'var(--src-gmail, #ea4335)',
		calendar: 'var(--src-calendar, #4285f4)',
		webhook: 'var(--src-webhook, #888)',
	};
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
		style="background: {sourceColors[item.source] ?? 'var(--text-tertiary)'}"
	></span>

	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-2">
			<p class="truncate text-sm font-medium text-[var(--text-primary)] group-hover:text-[var(--cairn-accent)] transition-colors">{item.title}</p>
		</div>
		{#if item.body}
			<p class="mt-0.5 truncate text-xs text-[var(--text-secondary)]">{item.body}</p>
		{/if}
		<div class="mt-0.5 flex items-center gap-1.5 text-[11px] text-[var(--text-tertiary)]">
			<Badge variant="outline" class="h-4 px-1 text-[10px] font-normal border-border-subtle">
				{item.source}
			</Badge>
			<span>{item.kind}</span>
			{#if item.author}
				<span>&middot;</span>
				<span>{item.author}</span>
			{/if}
			<span>&middot;</span>
			<time datetime={item.createdAt}>{relativeTime(item.createdAt)}</time>
		</div>
	</div>

	<!-- Action buttons (hover/focus reveal) -->
	<div class="flex items-center gap-0.5 flex-shrink-0 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto transition-opacity duration-[var(--dur-fast)]">
		{#if !item.isRead}
			<button
				class="rounded-md p-1 text-[var(--text-tertiary)] hover:text-[var(--cairn-accent)] hover:bg-[var(--bg-2)]"
				title="Mark as read"
				onclick={(e) => { e.preventDefault(); e.stopPropagation(); markItemRead(); }}
			>
				<Check class="h-3.5 w-3.5" />
			</button>
		{/if}
		<button
			class="rounded-md p-1 text-[var(--text-tertiary)] hover:text-[var(--color-warning)] hover:bg-[var(--bg-2)]"
			title="Archive"
			onclick={archiveItem}
		>
			<Archive class="h-3.5 w-3.5" />
		</button>
		{#if ondelete}
			<button
				class="rounded-md p-1 text-[var(--text-tertiary)] hover:text-[var(--color-error)] hover:bg-[var(--bg-2)]"
				title="Delete"
				onclick={deleteItem}
			>
				<Trash2 class="h-3.5 w-3.5" />
			</button>
		{/if}
	</div>

	{#if !item.isRead}
		<span class="mt-2 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-[var(--cairn-accent)]"></span>
	{/if}
</a>
