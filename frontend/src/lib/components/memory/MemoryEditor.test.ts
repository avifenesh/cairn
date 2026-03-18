import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import MemoryEditor from './MemoryEditor.svelte';

describe('MemoryEditor', () => {
	it('shows "New memory" button when closed', () => {
		render(MemoryEditor, { oncreate: vi.fn() });
		expect(screen.getByText('New memory')).toBeInTheDocument();
	});

	it('shows form when opened', async () => {
		render(MemoryEditor, { oncreate: vi.fn() });
		await fireEvent.click(screen.getByText('New memory'));
		expect(screen.getByPlaceholderText('What should I remember?')).toBeInTheDocument();
		expect(screen.getByLabelText('Memory category')).toBeInTheDocument();
	});

	it('has correct category options', async () => {
		render(MemoryEditor, { oncreate: vi.fn() });
		await fireEvent.click(screen.getByText('New memory'));
		const select = screen.getByLabelText('Memory category') as HTMLSelectElement;
		const options = Array.from(select.options).map((o) => o.value);
		expect(options).toEqual(['fact', 'preference', 'hard_rule', 'decision', 'writing_style']);
	});

	it('calls oncreate with content and category', async () => {
		const oncreate = vi.fn();
		render(MemoryEditor, { oncreate });
		await fireEvent.click(screen.getByText('New memory'));
		const textarea = screen.getByPlaceholderText('What should I remember?');
		await fireEvent.input(textarea, { target: { value: 'I prefer dark mode' } });
		await fireEvent.click(screen.getByText('Create'));
		expect(oncreate).toHaveBeenCalledWith('I prefer dark mode', 'fact');
	});

	it('disables create button when content is empty', async () => {
		render(MemoryEditor, { oncreate: vi.fn() });
		await fireEvent.click(screen.getByText('New memory'));
		const btn = screen.getByText('Create');
		expect(btn).toBeDisabled();
	});
});
