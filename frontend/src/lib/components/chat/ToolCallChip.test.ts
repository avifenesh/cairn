import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ToolCallChip from './ToolCallChip.svelte';

describe('ToolCallChip', () => {
	it('renders tool name', () => {
		render(ToolCallChip, { toolName: 'readFile', phase: 'result' });
		expect(screen.getByText('readFile')).toBeInTheDocument();
	});

	it('shows pulse indicator for start phase', () => {
		const { container } = render(ToolCallChip, { toolName: 'shell', phase: 'start' });
		const pulse = container.querySelector('.animate-pulse');
		expect(pulse).toBeInTheDocument();
	});

	it('does not show pulse for result phase', () => {
		const { container } = render(ToolCallChip, { toolName: 'shell', phase: 'result' });
		const pulse = container.querySelector('.animate-pulse');
		expect(pulse).toBeNull();
	});
});
