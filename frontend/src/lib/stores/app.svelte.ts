// Global application state

export type View = 'today' | 'ops' | 'chat' | 'memory' | 'agents' | 'skills' | 'soul' | 'settings';
export type Theme = 'dark' | 'light';
export type Density = 'comfortable' | 'balanced' | 'dense';
export type Mood = 'default' | 'dawn' | 'ocean' | 'night';

interface Notification {
	id: string;
	type: string;
	message: string;
	timestamp: number;
}

function getToastDuration(): number {
	try { return (Number(localStorage.getItem('pub_toast_duration')) || 5) * 1000; }
	catch { return 5000; }
}

let autoMoodEnabled = $state(localStorage.getItem('pub_auto_mood') === 'true');

let sseConnected = $state(false);
let clientId = $state<string | null>(null);
let theme = $state<Theme>((localStorage.getItem('pub_theme') as Theme) || 'dark');
let density = $state<Density>((localStorage.getItem('pub_density') as Density) || 'comfortable');
let mood = $state<Mood>((localStorage.getItem('pub_mood') as Mood) || 'default');
let commandPaletteOpen = $state(false);
let helpModalOpen = $state(false);
let contextPanelOpen = $state(true);
let notifications = $state<Notification[]>([]);
let pollStatuses = $state<Record<string, { newCount: number; at: number }>>({});
let agentProgresses = $state<Record<string, string>>({});
let sidebarCollapsed = $state(false);
let budgetTodayUsd = $state<number | null>(null);
let budgetDailyLimitUsd = $state<number | null>(null);

export const appStore = {
	get sseConnected() { return sseConnected; },
	get clientId() { return clientId; },
	get theme() { return theme; },
	get density() { return density; },
	get mood() { return mood; },
	get commandPaletteOpen() { return commandPaletteOpen; },
	get notifications() { return notifications; },
	get pollStatuses() { return pollStatuses; },
	get agentProgresses() { return agentProgresses; },
	get sidebarCollapsed() { return sidebarCollapsed; },
	get autoMoodEnabled() { return autoMoodEnabled; },
	get helpModalOpen() { return helpModalOpen; },
	get contextPanelOpen() { return contextPanelOpen; },
	get budgetTodayUsd() { return budgetTodayUsd; },
	get budgetDailyLimitUsd() { return budgetDailyLimitUsd; },

	setSSEConnected(v: boolean) { sseConnected = v; },
	setClientId(id: string) { clientId = id; },

	setTheme(t: Theme) {
		theme = t;
		localStorage.setItem('pub_theme', t);
		document.documentElement.setAttribute('data-theme', t);
	},

	setDensity(d: Density) {
		density = d;
		localStorage.setItem('pub_density', d);
		document.documentElement.setAttribute('data-density', d);
	},

	setMood(m: Mood) {
		mood = m;
		localStorage.setItem('pub_mood', m);
		if (m === 'default') {
			document.documentElement.removeAttribute('data-mood');
		} else {
			document.documentElement.setAttribute('data-mood', m);
		}
	},

	toggleTheme() {
		appStore.setTheme(theme === 'dark' ? 'light' : 'dark');
	},

	toggleCommandPalette() { commandPaletteOpen = !commandPaletteOpen; },
	openCommandPalette() { commandPaletteOpen = true; },
	closeCommandPalette() { commandPaletteOpen = false; },

	toggleHelpModal() { helpModalOpen = !helpModalOpen; },
	closeHelpModal() { helpModalOpen = false; },

	toggleSidebar() { sidebarCollapsed = !sidebarCollapsed; },

	toggleContextPanel() { contextPanelOpen = !contextPanelOpen; },
	closeContextPanel() { contextPanelOpen = false; },
	openContextPanel() { contextPanelOpen = true; },

	setBudget(todayUsd: number, dailyLimitUsd: number) {
		budgetTodayUsd = todayUsd;
		budgetDailyLimitUsd = dailyLimitUsd;
	},

	addNotification(type: string, message: string) {
		const id = crypto.randomUUID();
		notifications = [...notifications, { id, type, message, timestamp: Date.now() }];
		const duration = getToastDuration();
		setTimeout(() => {
			notifications = notifications.filter((n) => n.id !== id);
		}, duration);
	},

	dismissNotification(id: string) {
		notifications = notifications.filter((n) => n.id !== id);
	},

	setPollStatus(source: string, newCount: number) {
		pollStatuses = { ...pollStatuses, [source]: { newCount, at: Date.now() } };
	},

	setAgentProgress(agentId: string, message: string) {
		agentProgresses = { ...agentProgresses, [agentId]: message };
	},

	initTheme() {
		document.documentElement.setAttribute('data-theme', theme);
		if (density !== 'comfortable') document.documentElement.setAttribute('data-density', density);
		if (mood !== 'default') document.documentElement.setAttribute('data-mood', mood);
	},

	setAutoMood(enabled: boolean) {
		autoMoodEnabled = enabled;
		try { localStorage.setItem('pub_auto_mood', String(enabled)); } catch {}
		if (enabled) appStore.applyAutoMood();
	},

	applyAutoMood() {
		const hour = new Date().getHours();
		let autoMood: Mood;
		if (hour >= 6 && hour < 10) autoMood = 'dawn';
		else if (hour >= 10 && hour < 18) autoMood = 'default';
		else if (hour >= 18 && hour < 22) autoMood = 'ocean';
		else autoMood = 'night';
		appStore.setMood(autoMood);
	},
};
