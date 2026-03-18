import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import MemoryCard from './MemoryCard.svelte';
import type { Memory } from '$lib/types';

function makeMemory(overrides: Partial<Memory> = {}): Memory {
	return {
		id: 'mem-1',
		category: 'semantic',
		status: 'proposed',
		content: 'User prefers dark mode',
		createdAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

describe('MemoryCard', () => {
	it('renders content and category', () => {
		render(MemoryCard, { memory: makeMemory({ content: 'Always use bun', category: 'preference' }) });
		expect(screen.getByText('Always use bun')).toBeInTheDocument();
		expect(screen.getByText('preference')).toBeInTheDocument();
	});

	it('renders status text', () => {
		render(MemoryCard, { memory: makeMemory({ status: 'accepted' }) });
		expect(screen.getByText('accepted')).toBeInTheDocument();
	});

	it('shows accept/reject buttons for proposed memories when callbacks provided', () => {
		render(MemoryCard, {
			memory: makeMemory({ status: 'proposed' }),
			onaccept: vi.fn(),
			onreject: vi.fn(),
		});
		expect(screen.getByText('Accept')).toBeInTheDocument();
		expect(screen.getByText('Reject')).toBeInTheDocument();
	});

	it('hides action buttons for accepted memories', () => {
		render(MemoryCard, {
			memory: makeMemory({ status: 'accepted' }),
			onaccept: vi.fn(),
			onreject: vi.fn(),
		});
		expect(screen.queryByText('Accept')).toBeNull();
		expect(screen.queryByText('Reject')).toBeNull();
	});

	it('hides action buttons when callbacks not provided', () => {
		render(MemoryCard, { memory: makeMemory({ status: 'proposed' }) });
		expect(screen.queryByText('Accept')).toBeNull();
		expect(screen.queryByText('Reject')).toBeNull();
	});

	it('calls onaccept with memory id', async () => {
		const onaccept = vi.fn();
		render(MemoryCard, {
			memory: makeMemory({ id: 'mem-99' }),
			onaccept,
			onreject: vi.fn(),
		});
		await fireEvent.click(screen.getByText('Accept'));
		expect(onaccept).toHaveBeenCalledWith('mem-99');
	});

	it('hides action buttons for rejected memories', () => {
		render(MemoryCard, {
			memory: makeMemory({ status: 'rejected' }),
			onaccept: vi.fn(),
			onreject: vi.fn(),
		});
		expect(screen.queryByText('Accept')).toBeNull();
		expect(screen.queryByText('Reject')).toBeNull();
	});

	it('calls onreject with memory id', async () => {
		const onreject = vi.fn();
		render(MemoryCard, {
			memory: makeMemory({ id: 'mem-99' }),
			onaccept: vi.fn(),
			onreject,
		});
		await fireEvent.click(screen.getByText('Reject'));
		expect(onreject).toHaveBeenCalledWith('mem-99');
	});
});
