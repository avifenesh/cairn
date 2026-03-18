import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import SystemStatus from './SystemStatus.svelte';

vi.mock('$lib/api/client', () => ({
	getStatus: vi.fn().mockResolvedValue({ ok: true, uptime: '5h30m', version: '0.1.0', agent: 'cairn' }),
	getCosts: vi.fn().mockResolvedValue({ todayUsd: 0.0042, weekUsd: 0.5, budgetDailyUsd: 10, budgetWeeklyUsd: 50 }),
}));

vi.mock('$lib/stores/app.svelte', () => ({
	appStore: {
		get sseConnected() { return true; },
	},
}));

describe('SystemStatus', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders system status heading', () => {
		render(SystemStatus);
		expect(screen.getByText('System Status')).toBeInTheDocument();
	});

	it('shows connected status when SSE is connected', () => {
		render(SystemStatus);
		expect(screen.getByText('Connected')).toBeInTheDocument();
	});

	it('shows uptime from API', async () => {
		render(SystemStatus);
		await waitFor(() => {
			expect(screen.getByText('5h30m')).toBeInTheDocument();
		});
	});

	it('shows version from API', async () => {
		render(SystemStatus);
		await waitFor(() => {
			expect(screen.getByText('0.1.0')).toBeInTheDocument();
		});
	});

	it('shows cost data from API', async () => {
		render(SystemStatus);
		await waitFor(() => {
			expect(screen.getByText('$0.0042')).toBeInTheDocument();
			expect(screen.getByText('$0.5000')).toBeInTheDocument();
		});
	});
});
