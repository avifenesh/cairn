import { describe, it, expect, beforeEach } from 'vitest';
import { taskStore } from './tasks.svelte';
import type { Task, Approval } from '$lib/types';

function makeTask(overrides: Partial<Task> = {}): Task {
	return {
		id: 'task-1',
		type: 'coding',
		status: 'pending',
		title: 'Test task',
		createdAt: '2026-03-17T12:00:00Z',
		updatedAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

function makeApproval(overrides: Partial<Approval> = {}): Approval {
	return {
		id: 'appr-1',
		type: 'merge_pr',
		status: 'pending',
		title: 'Merge PR #42',
		createdAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

describe('taskStore', () => {
	beforeEach(() => {
		taskStore.setTasks([]);
		taskStore.setApprovals([]);
	});

	describe('tasks', () => {
		it('upsertTask inserts new task at front', () => {
			taskStore.upsertTask(makeTask({ id: 't1' }));
			expect(taskStore.tasks).toHaveLength(1);
			expect(taskStore.tasks[0].id).toBe('t1');
		});

		it('upsertTask updates existing task', () => {
			taskStore.setTasks([makeTask({ id: 't1', status: 'pending' })]);
			taskStore.upsertTask(makeTask({ id: 't1', status: 'running' }));
			expect(taskStore.tasks).toHaveLength(1);
			expect(taskStore.tasks[0].status).toBe('running');
		});

		it('activeTasks filters pending and running', () => {
			taskStore.setTasks([
				makeTask({ id: 't1', status: 'pending' }),
				makeTask({ id: 't2', status: 'running' }),
				makeTask({ id: 't3', status: 'completed' }),
				makeTask({ id: 't4', status: 'failed' }),
			]);
			expect(taskStore.activeTasks).toHaveLength(2);
		});
	});

	describe('approvals', () => {
		it('addApproval prepends and deduplicates', () => {
			taskStore.addApproval(makeApproval({ id: 'a1' }));
			expect(taskStore.approvals).toHaveLength(1);

			taskStore.addApproval(makeApproval({ id: 'a1' }));
			expect(taskStore.approvals).toHaveLength(1);

			taskStore.addApproval(makeApproval({ id: 'a2' }));
			expect(taskStore.approvals).toHaveLength(2);
		});

		it('pendingApprovals filters by status', () => {
			taskStore.setApprovals([
				makeApproval({ id: 'a1', status: 'pending' }),
				makeApproval({ id: 'a2', status: 'approved' }),
				makeApproval({ id: 'a3', status: 'pending' }),
			]);
			expect(taskStore.pendingApprovals).toHaveLength(2);
		});

		it('resolveApproval changes status optimistically', () => {
			taskStore.setApprovals([makeApproval({ id: 'a1', status: 'pending' })]);
			taskStore.resolveApproval('a1', 'approved');
			expect(taskStore.approvals[0].status).toBe('approved');
			expect(taskStore.approvals[0].decidedAt).toBeTruthy();
		});

		it('resolveApproval leaves other approvals unchanged', () => {
			taskStore.setApprovals([
				makeApproval({ id: 'a1', status: 'pending' }),
				makeApproval({ id: 'a2', status: 'pending' }),
			]);
			taskStore.resolveApproval('a1', 'denied');
			expect(taskStore.approvals[0].status).toBe('denied');
			expect(taskStore.approvals[1].status).toBe('pending');
		});
	});
});
