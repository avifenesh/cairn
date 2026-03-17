import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ErrorBoundary from './ErrorBoundary.svelte';

describe('ErrorBoundary', () => {
	it('renders nothing when error is null', () => {
		const { container } = render(ErrorBoundary, { error: null });
		expect(container.querySelector('[role="alert"]')).toBeNull();
	});

	it('renders error message', () => {
		render(ErrorBoundary, { error: 'Something broke' });
		expect(screen.getByRole('alert')).toBeInTheDocument();
		expect(screen.getByText('Something broke')).toBeInTheDocument();
		expect(screen.getByText('Something went wrong')).toBeInTheDocument();
	});

	it('renders retry button when onretry provided', () => {
		const onretry = vi.fn();
		render(ErrorBoundary, { error: 'fail', onretry });
		const btn = screen.getByText('Retry');
		expect(btn).toBeInTheDocument();
	});

	it('calls onretry when retry clicked', async () => {
		const onretry = vi.fn();
		render(ErrorBoundary, { error: 'fail', onretry });
		await fireEvent.click(screen.getByText('Retry'));
		expect(onretry).toHaveBeenCalledOnce();
	});

	it('does not render retry when onretry not provided', () => {
		render(ErrorBoundary, { error: 'fail' });
		expect(screen.queryByText('Retry')).toBeNull();
	});
});
