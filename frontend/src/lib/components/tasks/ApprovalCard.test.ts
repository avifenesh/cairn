import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ApprovalCard from './ApprovalCard.svelte';
import type { Approval } from '$lib/types';

function makeApproval(overrides: Partial<Approval> = {}): Approval {
	return {
		id: 'appr-1',
		type: 'merge_pr',
		status: 'pending',
		title: 'Merge PR #42',
		description: 'All checks passed',
		createdAt: new Date().toISOString(),
		...overrides,
	};
}

describe('ApprovalCard', () => {
	it('renders title and description', () => {
		render(ApprovalCard, {
			approval: makeApproval(),
			onapprove: vi.fn(),
			ondeny: vi.fn(),
		});
		expect(screen.getByText('Merge PR #42')).toBeInTheDocument();
		expect(screen.getByText('All checks passed')).toBeInTheDocument();
	});

	it('calls onapprove with id when Approve clicked', async () => {
		const onapprove = vi.fn();
		render(ApprovalCard, {
			approval: makeApproval({ id: 'test-id' }),
			onapprove,
			ondeny: vi.fn(),
		});
		await fireEvent.click(screen.getByText('Approve'));
		expect(onapprove).toHaveBeenCalledWith('test-id');
	});

	it('calls ondeny with id when Deny clicked', async () => {
		const ondeny = vi.fn();
		render(ApprovalCard, {
			approval: makeApproval({ id: 'test-id' }),
			onapprove: vi.fn(),
			ondeny,
		});
		await fireEvent.click(screen.getByText('Deny'));
		expect(ondeny).toHaveBeenCalledWith('test-id');
	});

	it('shows checkbox when onselect provided', () => {
		const { container } = render(ApprovalCard, {
			approval: makeApproval(),
			onapprove: vi.fn(),
			ondeny: vi.fn(),
			onselect: vi.fn(),
			selected: false,
		});
		expect(container.querySelector('input[type="checkbox"]')).toBeInTheDocument();
	});

	it('does not show checkbox when onselect not provided', () => {
		const { container } = render(ApprovalCard, {
			approval: makeApproval(),
			onapprove: vi.fn(),
			ondeny: vi.fn(),
		});
		expect(container.querySelector('input[type="checkbox"]')).toBeNull();
	});

	it('renders without description', () => {
		render(ApprovalCard, {
			approval: makeApproval({ description: undefined }),
			onapprove: vi.fn(),
			ondeny: vi.fn(),
		});
		expect(screen.getByText('Merge PR #42')).toBeInTheDocument();
	});
});
