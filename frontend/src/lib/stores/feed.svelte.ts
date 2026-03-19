// Feed store — signal items with unread tracking

import type { FeedItem } from '$lib/types';

let items = $state<FeedItem[]>([]);
let loading = $state(false);
let hasMore = $state(true);

export const feedStore = {
	get items() { return items; },
	get loading() { return loading; },
	get hasMore() { return hasMore; },
	get unreadCount() { return items.filter((i) => !i.isRead).length; },

	setItems(newItems: FeedItem[], more: boolean) {
		items = newItems;
		hasMore = more;
	},

	appendItems(newItems: FeedItem[], more: boolean) {
		const ids = new Set(items.map((i) => i.id));
		const fresh = newItems.filter((i) => !ids.has(i.id));
		items = [...items, ...fresh];
		hasMore = more;
	},

	addItem(item: FeedItem) {
		if (items.some((i) => i.id === item.id)) return;
		items = [item, ...items];
	},

	markItemRead(id: string) {
		items = items.map((i) => (i.id === id ? { ...i, isRead: true } : i));
	},

	markAllItemsRead() {
		items = items.map((i) => ({ ...i, isRead: true }));
	},

	archiveItem(id: string) {
		items = items.filter((i) => i.id !== id);
	},

	removeItem(id: string) {
		items = items.filter((i) => i.id !== id);
	},

	setLoading(v: boolean) { loading = v; },
};
