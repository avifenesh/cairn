import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TaskCreateForm from './TaskCreateForm.svelte';

describe('TaskCreateForm', () => {
	it('shows "New task" button when closed', () => {
		render(TaskCreateForm, { oncreate: vi.fn() });
		expect(screen.getByText('New task')).toBeInTheDocument();
	});

	it('shows form when opened', async () => {
		render(TaskCreateForm, { oncreate: vi.fn() });
		await fireEvent.click(screen.getByText('New task'));
		expect(screen.getByPlaceholderText('What needs to be done?')).toBeInTheDocument();
		expect(screen.getByLabelText('Task type')).toBeInTheDocument();
		expect(screen.getByLabelText('Task priority')).toBeInTheDocument();
	});

	it('calls oncreate with description, type, priority', async () => {
		const oncreate = vi.fn();
		render(TaskCreateForm, { oncreate });
		await fireEvent.click(screen.getByText('New task'));
		const textarea = screen.getByPlaceholderText('What needs to be done?');
		await fireEvent.input(textarea, { target: { value: 'Fix the bug' } });
		await fireEvent.click(screen.getByText('Create'));
		expect(oncreate).toHaveBeenCalledWith('Fix the bug', 'general', 2);
	});

	it('disables create when description is empty', async () => {
		render(TaskCreateForm, { oncreate: vi.fn() });
		await fireEvent.click(screen.getByText('New task'));
		expect(screen.getByText('Create')).toBeDisabled();
	});

	it('closes form when X is clicked', async () => {
		render(TaskCreateForm, { oncreate: vi.fn() });
		await fireEvent.click(screen.getByText('New task'));
		expect(screen.getByPlaceholderText('What needs to be done?')).toBeInTheDocument();
		await fireEvent.click(screen.getByLabelText('Close create task'));
		expect(screen.getByText('New task')).toBeInTheDocument();
	});
});
