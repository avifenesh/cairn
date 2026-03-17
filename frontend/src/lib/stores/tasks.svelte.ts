// Tasks & approvals store

import type { Task, Approval } from '$lib/types';

let tasks = $state<Task[]>([]);
let approvals = $state<Approval[]>([]);
let loading = $state(false);

export const taskStore = {
	get tasks() { return tasks; },
	get approvals() { return approvals; },
	get loading() { return loading; },
	get pendingApprovals() { return approvals.filter((a) => a.status === 'pending'); },
	get activeTasks() { return tasks.filter((t) => t.status === 'running' || t.status === 'pending'); },

	setTasks(t: Task[]) { tasks = t; },
	setApprovals(a: Approval[]) { approvals = a; },
	setLoading(v: boolean) { loading = v; },

	upsertTask(task: Task) {
		const idx = tasks.findIndex((t) => t.id === task.id);
		if (idx >= 0) {
			tasks = tasks.map((t, i) => (i === idx ? task : t));
		} else {
			tasks = [task, ...tasks];
		}
	},

	addApproval(approval: Approval) {
		if (approvals.some((a) => a.id === approval.id)) return;
		approvals = [approval, ...approvals];
	},

	resolveApproval(id: string, status: 'approved' | 'denied') {
		approvals = approvals.map((a) =>
			a.id === id ? { ...a, status, decidedAt: new Date().toISOString() } : a,
		);
	},
};
