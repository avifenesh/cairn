// Activity store — agent observability

import type { ActivityEntry, ToolStatsOverview } from '$lib/types';

let entries = $state<ActivityEntry[]>([]);
let toolStats = $state<ToolStatsOverview | null>(null);
let loading = $state(true);

export const activityStore = {
	get entries() { return entries; },
	get toolStats() { return toolStats; },
	get loading() { return loading; },
	get errorCount() { return entries.filter((e) => e.errors && e.errors.length > 0).length; },

	setEntries(e: ActivityEntry[]) {
		// Merge: keep SSE-streamed entries not in the fetched set to avoid losing live rows.
		const fetchedIds = new Set(e.map((x) => x.id));
		const streamed = entries.filter((x) => !fetchedIds.has(x.id));
		entries = [...streamed, ...e]
			.sort((a, b) => b.createdAt.localeCompare(a.createdAt))
			.slice(0, 200);
	},
	setToolStats(s: ToolStatsOverview) { toolStats = s; },
	setLoading(v: boolean) { loading = v; },

	addEntry(entry: ActivityEntry) {
		// Dedup check.
		if (entries.some((e) => e.id === entry.id)) return;
		// Prepend and cap at 200.
		entries = [entry, ...entries].slice(0, 200);
		// Update tool stats live.
		if (toolStats && entry.toolCount > 0) {
			toolStats = { ...toolStats, totalCalls: toolStats.totalCalls + entry.toolCount };
		}
		if (toolStats && entry.errors && entry.errors.length > 0) {
			toolStats = { ...toolStats, totalErrors: toolStats.totalErrors + entry.errors.length };
		}
	},
};
