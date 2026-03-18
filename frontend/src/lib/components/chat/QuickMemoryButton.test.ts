import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import QuickMemoryButton from './QuickMemoryButton.svelte';

const mockCreateMemory = vi.fn().mockResolvedValue({ id: 'mem-1', content: 'test', category: 'fact', status: 'accepted', createdAt: new Date().toISOString() });

vi.mock('$lib/api/client', () => ({
	createMemory: (...args: unknown[]) => mockCreateMemory(...args),
}));

describe('QuickMemoryButton', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders bookmark button in idle state', () => {
		render(QuickMemoryButton, { content: 'test message' });
		expect(screen.getByLabelText('Remember this')).toBeInTheDocument();
	});

	it('shows category picker on click', async () => {
		render(QuickMemoryButton, { content: 'test message' });
		await fireEvent.click(screen.getByLabelText('Remember this'));
		expect(screen.getByLabelText('Memory category')).toBeInTheDocument();
		expect(screen.getByLabelText('Save memory')).toBeInTheDocument();
	});

	it('calls createMemory with content and category on save', async () => {
		render(QuickMemoryButton, { content: 'remember this text' });
		await fireEvent.click(screen.getByLabelText('Remember this'));
		await fireEvent.click(screen.getByLabelText('Save memory'));
		expect(mockCreateMemory).toHaveBeenCalledWith('remember this text', 'fact');
	});

	it('shows done state after save', async () => {
		render(QuickMemoryButton, { content: 'test' });
		await fireEvent.click(screen.getByLabelText('Remember this'));
		await fireEvent.click(screen.getByLabelText('Save memory'));
		await waitFor(() => {
			expect(screen.getByText('Saved')).toBeInTheDocument();
		});
	});

	it('shows error state on failure', async () => {
		mockCreateMemory.mockRejectedValueOnce(new Error('fail'));
		render(QuickMemoryButton, { content: 'test' });
		await fireEvent.click(screen.getByLabelText('Remember this'));
		await fireEvent.click(screen.getByLabelText('Save memory'));
		await waitFor(() => {
			expect(screen.getByText('Failed')).toBeInTheDocument();
		});
	});
});
