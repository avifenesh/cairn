import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import CronManager from './CronManager.svelte';

vi.mock('$lib/api/client', () => ({
	getCrons: vi.fn(() => Promise.resolve({ items: [], count: 0 })),
	createCron: vi.fn(() => Promise.resolve({ id: 'cron_test', name: 'test', schedule: '0 9 * * *', instruction: 'do stuff', enabled: true, timezone: 'UTC', priority: 3, cooldownMs: 3600000, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() })),
	updateCron: vi.fn(() => Promise.resolve({ ok: true, job: {} })),
	deleteCron: vi.fn(() => Promise.resolve({ ok: true })),
	getCronDetail: vi.fn(() => Promise.resolve({ job: {}, executions: [] })),
}));

describe('CronManager', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders empty state when no jobs', async () => {
		render(CronManager);
		expect(await screen.findByText('No scheduled jobs')).toBeInTheDocument();
	});

	it('renders create form with required fields', () => {
		render(CronManager);
		expect(screen.getByPlaceholderText('Morning digest')).toBeInTheDocument();
		expect(screen.getByPlaceholderText('0 9 * * 1-5')).toBeInTheDocument();
		expect(screen.getByPlaceholderText(/What should cairn do/)).toBeInTheDocument();
	});

	it('shows create button', () => {
		render(CronManager);
		expect(screen.getByText('Create Job')).toBeInTheDocument();
	});
});
