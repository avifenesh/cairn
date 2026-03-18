// Status store — system status, budget, poller state

let uptime = $state('');
let version = $state('');
let budgetToday = $state(0);
let budgetWeek = $state(0);
let budgetDailyCap = $state(0);
let budgetWeeklyCap = $state(0);

export const statusStore = {
	get uptime() { return uptime; },
	get version() { return version; },
	get budgetToday() { return budgetToday; },
	get budgetWeek() { return budgetWeek; },
	get budgetDailyCap() { return budgetDailyCap; },
	get budgetWeeklyCap() { return budgetWeeklyCap; },

	setStatus(data: Record<string, unknown>) {
		if (data.uptime) uptime = String(data.uptime);
		if (data.version) version = String(data.version);
	},

	setBudget(data: Record<string, number>) {
		budgetToday = data.todayUsd ?? data.today ?? 0;
		budgetWeek = data.weekUsd ?? data.thisMonth ?? 0;
		budgetDailyCap = data.budgetDailyUsd ?? 0;
		budgetWeeklyCap = data.budgetWeeklyUsd ?? 0;
	},
};
