import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TaskCard from './TaskCard.svelte';
import type { Task } from '$lib/types';

function makeTask(overrides: Partial<Task> = {}): Task {
	return {
		id: 'task-1',
		type: 'chat',
		status: 'pending',
		title: 'Process message',
		createdAt: '2026-03-17T12:00:00Z',
		updatedAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

describe('TaskCard', () => {
	it('renders title, type, and status', () => {
		render(TaskCard, { task: makeTask({ title: 'Build feature', type: 'coding', status: 'running' }) });
		expect(screen.getByText('Build feature')).toBeInTheDocument();
		expect(screen.getByText(/coding/)).toBeInTheDocument();
		expect(screen.getByText(/running/)).toBeInTheDocument();
	});

	it('shows cancel button for running tasks when oncancel provided', () => {
		render(TaskCard, { task: makeTask({ status: 'running' }), oncancel: vi.fn() });
		expect(screen.getByText('Cancel')).toBeInTheDocument();
	});

	it('shows cancel button for pending tasks when oncancel provided', () => {
		render(TaskCard, { task: makeTask({ status: 'pending' }), oncancel: vi.fn() });
		expect(screen.getByText('Cancel')).toBeInTheDocument();
	});

	it('hides cancel button for completed tasks', () => {
		render(TaskCard, { task: makeTask({ status: 'completed' }), oncancel: vi.fn() });
		expect(screen.queryByText('Cancel')).toBeNull();
	});

	it('hides cancel button for failed tasks', () => {
		render(TaskCard, { task: makeTask({ status: 'failed' }), oncancel: vi.fn() });
		expect(screen.queryByText('Cancel')).toBeNull();
	});

	it('hides cancel button when oncancel not provided', () => {
		render(TaskCard, { task: makeTask({ status: 'running' }) });
		expect(screen.queryByText('Cancel')).toBeNull();
	});

	it('hides cancel button for cancelled tasks', () => {
		render(TaskCard, { task: makeTask({ status: 'cancelled' }), oncancel: vi.fn() });
		expect(screen.queryByText('Cancel')).toBeNull();
	});

	it('calls oncancel with task id when clicked', async () => {
		const oncancel = vi.fn();
		render(TaskCard, { task: makeTask({ id: 'task-42', status: 'running' }), oncancel });
		await fireEvent.click(screen.getByText('Cancel'));
		expect(oncancel).toHaveBeenCalledWith('task-42');
	});
});
