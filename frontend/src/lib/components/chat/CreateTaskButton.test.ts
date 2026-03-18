import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CreateTaskButton from './CreateTaskButton.svelte';

const mockCreateTask = vi.fn().mockResolvedValue({ id: 'task-1', type: 'general', status: 'queued', priority: 2 });

vi.mock('$lib/api/client', () => ({
	createTask: (...args: unknown[]) => mockCreateTask(...args),
}));

describe('CreateTaskButton', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders create task button in idle state', () => {
		render(CreateTaskButton, { content: 'test message' });
		expect(screen.getByLabelText('Create task')).toBeInTheDocument();
	});

	it('shows priority picker on click', async () => {
		render(CreateTaskButton, { content: 'test message' });
		await fireEvent.click(screen.getByLabelText('Create task'));
		expect(screen.getByLabelText('Task priority')).toBeInTheDocument();
		expect(screen.getByLabelText('Save task')).toBeInTheDocument();
	});

	it('calls createTask with truncated content', async () => {
		render(CreateTaskButton, { content: 'create this as a task' });
		await fireEvent.click(screen.getByLabelText('Create task'));
		await fireEvent.click(screen.getByLabelText('Save task'));
		expect(mockCreateTask).toHaveBeenCalledWith('create this as a task', 'general', 2);
	});

	it('shows done state after save', async () => {
		render(CreateTaskButton, { content: 'test' });
		await fireEvent.click(screen.getByLabelText('Create task'));
		await fireEvent.click(screen.getByLabelText('Save task'));
		await waitFor(() => {
			expect(screen.getByText('Created')).toBeInTheDocument();
		});
	});

	it('shows error state on failure', async () => {
		mockCreateTask.mockRejectedValueOnce(new Error('fail'));
		render(CreateTaskButton, { content: 'test' });
		await fireEvent.click(screen.getByLabelText('Create task'));
		await fireEvent.click(screen.getByLabelText('Save task'));
		await waitFor(() => {
			expect(screen.getByText('Failed')).toBeInTheDocument();
		});
	});
});
