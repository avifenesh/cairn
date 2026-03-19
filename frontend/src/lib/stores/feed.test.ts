import { describe, it, expect, beforeEach } from 'vitest';
import { feedStore } from './feed.svelte';
import type { FeedItem } from '$lib/types';

function makeFeedItem(overrides: Partial<FeedItem> = {}): FeedItem {
	return {
		id: 'ev_test1',
		source: 'github',
		kind: 'push',
		title: 'Test event',
		isRead: false,
		isArchived: false,
		createdAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

describe('feedStore', () => {
	beforeEach(() => {
		feedStore.setItems([], true);
	});

	it('starts empty', () => {
		expect(feedStore.items).toEqual([]);
		expect(feedStore.unreadCount).toBe(0);
	});

	it('setItems replaces all items', () => {
		feedStore.setItems([makeFeedItem({ id: 'ev_1' }), makeFeedItem({ id: 'ev_2' })], false);
		expect(feedStore.items).toHaveLength(2);
		expect(feedStore.hasMore).toBe(false);
	});

	it('unreadCount counts unread items', () => {
		feedStore.setItems([
			makeFeedItem({ id: 'ev_1', isRead: false }),
			makeFeedItem({ id: 'ev_2', isRead: true }),
			makeFeedItem({ id: 'ev_3', isRead: false }),
		], true);
		expect(feedStore.unreadCount).toBe(2);
	});

	it('addItem prepends and deduplicates', () => {
		feedStore.setItems([makeFeedItem({ id: 'ev_1' })], true);
		feedStore.addItem(makeFeedItem({ id: 'ev_2', title: 'new' }));
		expect(feedStore.items).toHaveLength(2);
		expect(feedStore.items[0].id).toBe('ev_2');

		// Duplicate should be ignored
		feedStore.addItem(makeFeedItem({ id: 'ev_2', title: 'new' }));
		expect(feedStore.items).toHaveLength(2);
	});

	it('appendItems deduplicates on append', () => {
		feedStore.setItems([makeFeedItem({ id: 'ev_1' })], true);
		feedStore.appendItems([makeFeedItem({ id: 'ev_1' }), makeFeedItem({ id: 'ev_2' })], false);
		expect(feedStore.items).toHaveLength(2);
		expect(feedStore.hasMore).toBe(false);
	});

	it('markItemRead marks a single item', () => {
		feedStore.setItems([makeFeedItem({ id: 'ev_1', isRead: false })], true);
		feedStore.markItemRead('ev_1');
		expect(feedStore.items[0].isRead).toBe(true);
	});

	it('markAllItemsRead marks everything', () => {
		feedStore.setItems([
			makeFeedItem({ id: 'ev_1', isRead: false }),
			makeFeedItem({ id: 'ev_2', isRead: false }),
		], true);
		feedStore.markAllItemsRead();
		expect(feedStore.unreadCount).toBe(0);
	});

	it('archiveItem removes item from list', () => {
		feedStore.setItems([
			makeFeedItem({ id: 'ev_1' }),
			makeFeedItem({ id: 'ev_2' }),
		], true);
		feedStore.archiveItem('ev_1');
		expect(feedStore.items).toHaveLength(1);
		expect(feedStore.items[0].id).toBe('ev_2');
	});

	it('removeItem removes item from list', () => {
		feedStore.setItems([
			makeFeedItem({ id: 'ev_1' }),
			makeFeedItem({ id: 'ev_2' }),
		], true);
		feedStore.removeItem('ev_1');
		expect(feedStore.items).toHaveLength(1);
		expect(feedStore.items[0].id).toBe('ev_2');
	});
});
