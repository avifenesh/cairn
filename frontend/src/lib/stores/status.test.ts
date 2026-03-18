import { describe, it, expect, beforeEach } from 'vitest';
import { statusStore } from './status.svelte';

describe('statusStore', () => {
	beforeEach(() => {
		statusStore.setStatus({ uptime: '', version: '' });
		statusStore.setBudget({ todayUsd: 0, weekUsd: 0, budgetDailyUsd: 0, budgetWeeklyUsd: 0 });
	});
	it('sets status data', () => {
		statusStore.setStatus({ uptime: '5h30m', version: '0.1.0' });
		expect(statusStore.uptime).toBe('5h30m');
		expect(statusStore.version).toBe('0.1.0');
	});

	it('sets budget data', () => {
		statusStore.setBudget({ todayUsd: 0.5, weekUsd: 3.2, budgetDailyUsd: 10, budgetWeeklyUsd: 50 });
		expect(statusStore.budgetToday).toBe(0.5);
		expect(statusStore.budgetWeek).toBe(3.2);
		expect(statusStore.budgetDailyCap).toBe(10);
		expect(statusStore.budgetWeeklyCap).toBe(50);
	});

	it('handles alternative field names', () => {
		statusStore.setBudget({ today: 1.0, thisMonth: 5.0 });
		expect(statusStore.budgetToday).toBe(1.0);
		expect(statusStore.budgetWeek).toBe(5.0);
	});

	it('sets MCP status', () => {
		statusStore.setMcpStatus({ enabled: true, port: 3001, transport: 'http' });
		expect(statusStore.mcpEnabled).toBe(true);
		expect(statusStore.mcpPort).toBe(3001);
		expect(statusStore.mcpTransport).toBe('http');
	});

	it('defaults MCP status when partial', () => {
		statusStore.setMcpStatus({});
		expect(statusStore.mcpEnabled).toBe(false);
		expect(statusStore.mcpPort).toBe(0);
		expect(statusStore.mcpTransport).toBe('');
	});
});
