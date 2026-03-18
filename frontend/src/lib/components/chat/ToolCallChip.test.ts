import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ToolCallChip from './ToolCallChip.svelte';

describe('ToolCallChip', () => {
	it('renders tool name', () => {
		render(ToolCallChip, { toolName: 'readFile', phase: 'result' });
		expect(screen.getByText('readFile')).toBeInTheDocument();
	});

	it('shows spin indicator for start phase', () => {
		const { container } = render(ToolCallChip, { toolName: 'shell', phase: 'start' });
		const spin = container.querySelector('.animate-spin');
		expect(spin).toBeInTheDocument();
	});

	it('does not show spin for result phase', () => {
		const { container } = render(ToolCallChip, { toolName: 'shell', phase: 'result' });
		const spin = container.querySelector('.animate-spin');
		expect(spin).toBeNull();
	});
});
