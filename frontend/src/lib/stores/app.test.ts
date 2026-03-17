import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { appStore } from './app.svelte';

describe('appStore', () => {
	beforeEach(() => {
		localStorage.clear();
		// Reset DOM attributes
		document.documentElement.removeAttribute('data-theme');
		document.documentElement.removeAttribute('data-density');
		document.documentElement.removeAttribute('data-mood');
		// Reset store state to defaults (singleton persists across tests)
		appStore.setTheme('dark');
		appStore.setDensity('comfortable');
		appStore.setMood('default');
		appStore.closeCommandPalette();
		appStore.closeHelpModal();
		appStore.setSSEConnected(false);
	});

	describe('theme', () => {
		it('defaults to dark', () => {
			expect(appStore.theme).toBe('dark');
		});

		it('setTheme persists to localStorage and sets DOM attribute', () => {
			appStore.setTheme('light');
			expect(appStore.theme).toBe('light');
			expect(localStorage.getItem('pub_theme')).toBe('light');
			expect(document.documentElement.getAttribute('data-theme')).toBe('light');
		});

		it('toggleTheme switches between dark and light', () => {
			appStore.setTheme('dark');
			appStore.toggleTheme();
			expect(appStore.theme).toBe('light');
			appStore.toggleTheme();
			expect(appStore.theme).toBe('dark');
		});
	});

	describe('density', () => {
		it('defaults to comfortable', () => {
			expect(appStore.density).toBe('comfortable');
		});

		it('setDensity persists and sets DOM attribute', () => {
			appStore.setDensity('dense');
			expect(appStore.density).toBe('dense');
			expect(localStorage.getItem('pub_density')).toBe('dense');
			expect(document.documentElement.getAttribute('data-density')).toBe('dense');
		});
	});

	describe('mood', () => {
		it('defaults to default', () => {
			expect(appStore.mood).toBe('default');
		});

		it('setMood persists and sets DOM attribute for non-default moods', () => {
			appStore.setMood('ocean');
			expect(document.documentElement.getAttribute('data-mood')).toBe('ocean');
			expect(localStorage.getItem('pub_mood')).toBe('ocean');
		});

		it('setMood removes DOM attribute for default mood', () => {
			appStore.setMood('ocean');
			appStore.setMood('default');
			expect(document.documentElement.getAttribute('data-mood')).toBeNull();
		});
	});

	describe('auto-mood', () => {
		it('applyAutoMood selects dawn for 6-10', () => {
			vi.setSystemTime(new Date('2026-03-17T07:00:00'));
			appStore.applyAutoMood();
			expect(appStore.mood).toBe('dawn');
		});

		it('applyAutoMood selects default for 10-18', () => {
			vi.setSystemTime(new Date('2026-03-17T14:00:00'));
			appStore.applyAutoMood();
			expect(appStore.mood).toBe('default');
		});

		it('applyAutoMood selects ocean for 18-22', () => {
			vi.setSystemTime(new Date('2026-03-17T20:00:00'));
			appStore.applyAutoMood();
			expect(appStore.mood).toBe('ocean');
		});

		it('applyAutoMood selects night for 22-6', () => {
			vi.setSystemTime(new Date('2026-03-17T23:00:00'));
			appStore.applyAutoMood();
			expect(appStore.mood).toBe('night');
		});

		afterEach(() => {
			vi.useRealTimers();
		});
	});

	describe('notifications', () => {
		beforeEach(() => {
			vi.useFakeTimers();
			// Clear any leftover notifications from other tests
			for (const n of [...appStore.notifications]) {
				appStore.dismissNotification(n.id);
			}
		});

		afterEach(() => {
			vi.useRealTimers();
		});

		it('addNotification creates a notification', () => {
			appStore.addNotification('info', 'Test message');
			expect(appStore.notifications).toHaveLength(1);
			expect(appStore.notifications[0].message).toBe('Test message');
			expect(appStore.notifications[0].type).toBe('info');
		});

		it('notifications auto-expire after toast duration', () => {
			appStore.addNotification('info', 'Expires soon');
			expect(appStore.notifications).toHaveLength(1);
			// Default toast duration is 5s
			vi.advanceTimersByTime(5500);
			expect(appStore.notifications).toHaveLength(0);
		});

		it('dismissNotification removes immediately', () => {
			appStore.addNotification('info', 'Dismiss me');
			const id = appStore.notifications[0].id;
			appStore.dismissNotification(id);
			expect(appStore.notifications).toHaveLength(0);
		});
	});

	describe('command palette', () => {
		it('starts closed', () => {
			expect(appStore.commandPaletteOpen).toBe(false);
		});

		it('toggleCommandPalette opens and closes', () => {
			appStore.toggleCommandPalette();
			expect(appStore.commandPaletteOpen).toBe(true);
			appStore.toggleCommandPalette();
			expect(appStore.commandPaletteOpen).toBe(false);
		});

		it('closeCommandPalette always closes', () => {
			appStore.openCommandPalette();
			appStore.closeCommandPalette();
			expect(appStore.commandPaletteOpen).toBe(false);
		});
	});

	describe('SSE state', () => {
		it('setSSEConnected updates connected state', () => {
			appStore.setSSEConnected(true);
			expect(appStore.sseConnected).toBe(true);
			appStore.setSSEConnected(false);
			expect(appStore.sseConnected).toBe(false);
		});
	});

	describe('budget', () => {
		it('setBudget stores values', () => {
			appStore.setBudget(1.5, 10);
			expect(appStore.budgetTodayUsd).toBe(1.5);
			expect(appStore.budgetDailyLimitUsd).toBe(10);
		});
	});
});
