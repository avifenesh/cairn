// Memory store

import type { Memory } from '$lib/types';

let memories = $state<Memory[]>([]);
let searchResults = $state<Memory[]>([]);
let searchQuery = $state('');
let loading = $state(false);

export const memoryStore = {
	get memories() { return memories; },
	get searchResults() { return searchResults; },
	get searchQuery() { return searchQuery; },
	get loading() { return loading; },
	get proposedCount() { return memories.filter((m) => m.status === 'proposed').length; },

	setMemories(m: Memory[]) { memories = m; },
	setSearchResults(r: Memory[]) { searchResults = r; },
	setSearchQuery(q: string) { searchQuery = q; },
	setLoading(v: boolean) { loading = v; },

	resolveMemory(id: string, status: 'accepted' | 'rejected') {
		memories = memories.map((m) => (m.id === id ? { ...m, status } : m));
	},
};
