import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
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

	it('shows duration when provided', () => {
		render(ToolCallChip, { toolName: 'webSearch', phase: 'result', durationMs: 1500 });
		expect(screen.getByText('1.5s')).toBeInTheDocument();
	});

	it('shows ms for short durations', () => {
		render(ToolCallChip, { toolName: 'readFile', phase: 'result', durationMs: 42 });
		expect(screen.getByText('42ms')).toBeInTheDocument();
	});

	it('does not show duration during start phase', () => {
		render(ToolCallChip, { toolName: 'shell', phase: 'start', durationMs: 500 });
		expect(screen.queryByText('500ms')).toBeNull();
	});

	it('shows error styling when error is provided', () => {
		const { container } = render(ToolCallChip, { toolName: 'shell', phase: 'result', error: 'command failed' });
		const btn = container.querySelector('button');
		expect(btn?.className).toContain('color-error');
	});

	it('expands to show result on click', async () => {
		render(ToolCallChip, { toolName: 'readFile', phase: 'result', result: 'file contents here' });
		const btn = screen.getByRole('button');
		await fireEvent.click(btn);
		expect(screen.getByText('file contents here')).toBeInTheDocument();
	});

	it('expands to show args on click', async () => {
		render(ToolCallChip, { toolName: 'webSearch', phase: 'result', args: { query: 'test' } });
		const btn = screen.getByRole('button');
		await fireEvent.click(btn);
		expect(screen.getByText(/test/)).toBeInTheDocument();
	});

	it('shows error text when expanded', async () => {
		render(ToolCallChip, { toolName: 'shell', phase: 'result', error: 'exit code 1' });
		const btn = screen.getByRole('button');
		await fireEvent.click(btn);
		expect(screen.getByText('exit code 1')).toBeInTheDocument();
	});

	it('does not show expand chevron when no details', () => {
		const { container } = render(ToolCallChip, { toolName: 'readFile', phase: 'result' });
		// No ChevronDown SVG should be present
		const svgs = container.querySelectorAll('svg');
		// Only the Check icon should be present
		expect(svgs.length).toBe(1);
	});
});
