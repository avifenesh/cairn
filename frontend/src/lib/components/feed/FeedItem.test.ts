import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import FeedItem from './FeedItem.svelte';
import type { FeedItem as FeedItemType } from '$lib/types';

// Mock the API client
vi.mock('$lib/api/client', () => ({
	markRead: vi.fn(() => Promise.resolve({ ok: true })),
	archiveFeedItem: vi.fn(() => Promise.resolve({ ok: true })),
}));

function makeItem(overrides: Partial<FeedItemType> = {}): FeedItemType {
	return {
		id: 'ev_test1',
		source: 'github',
		kind: 'push',
		title: 'Test push event',
		url: 'https://github.com/test',
		isRead: false,
		isArchived: false,
		createdAt: new Date().toISOString(),
		...overrides,
	};
}

describe('FeedItem', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('renders item title and source', () => {
		render(FeedItem, { item: makeItem({ title: 'My PR', source: 'github' }) });
		expect(screen.getByText('My PR')).toBeInTheDocument();
		expect(screen.getByText(/github/)).toBeInTheDocument();
	});

	it('renders as a link with correct href', () => {
		const { container } = render(FeedItem, { item: makeItem({ url: 'https://example.com' }) });
		const link = container.querySelector('a');
		expect(link?.getAttribute('href')).toBe('https://example.com');
		expect(link?.getAttribute('target')).toBe('_blank');
	});

	it('shows unread indicator for unread items', () => {
		const { container } = render(FeedItem, { item: makeItem({ isRead: false }) });
		// Unread dot is a small accent-colored span
		const dots = container.querySelectorAll('span');
		const accentDot = Array.from(dots).find((s) => s.className.includes('bg-[var(--cairn-accent)]'));
		expect(accentDot).toBeTruthy();
	});

	it('does not show unread indicator for read items', () => {
		const { container } = render(FeedItem, { item: makeItem({ isRead: true }) });
		const dots = container.querySelectorAll('span');
		const accentDot = Array.from(dots).find((s) => s.className.includes('bg-[var(--cairn-accent)]'));
		expect(accentDot).toBeFalsy();
	});

	it('applies opacity class for read items', () => {
		const { container } = render(FeedItem, { item: makeItem({ isRead: true }) });
		const link = container.querySelector('a');
		expect(link?.className).toContain('opacity-50');
	});

	it('shows mark-read button for unread items', () => {
		const { container } = render(FeedItem, { item: makeItem({ isRead: false }) });
		const button = container.querySelector('button[title="Mark as read"]');
		expect(button).toBeTruthy();
	});

	it('does not show mark-read button for read items', () => {
		const { container } = render(FeedItem, { item: makeItem({ isRead: true }) });
		const button = container.querySelector('button[title="Mark as read"]');
		expect(button).toBeFalsy();
	});

	it('shows archive button', () => {
		const { container } = render(FeedItem, { item: makeItem() });
		const button = container.querySelector('button[title="Archive"]');
		expect(button).toBeTruthy();
	});

	it('shows delete button when ondelete prop is provided', () => {
		const { container } = render(FeedItem, { item: makeItem(), ondelete: () => {} });
		const button = container.querySelector('button[title="Delete"]');
		expect(button).toBeTruthy();
	});

	it('does not show delete button when ondelete is not provided', () => {
		const { container } = render(FeedItem, { item: makeItem() });
		const button = container.querySelector('button[title="Delete"]');
		expect(button).toBeFalsy();
	});
});
