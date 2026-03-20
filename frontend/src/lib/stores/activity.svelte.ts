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

	setEntries(e: ActivityEntry[]) { entries = e; },
	setToolStats(s: ToolStatsOverview) { toolStats = s; },
	setLoading(v: boolean) { loading = v; },

	addEntry(entry: ActivityEntry) {
		// Prepend and cap at 200.
		entries = [entry, ...entries].slice(0, 200);
	},
};
