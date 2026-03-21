// Subagent lifecycle store — tracks spawned child agents and their progress via SSE

import type { SubagentInfo } from '$lib/types';

let subagents = $state<Map<string, SubagentInfo>>(new Map());

export const subagentStore = {
	get subagents() {
		return subagents;
	},
	get activeSubagents(): SubagentInfo[] {
		return [...subagents.values()].filter((s) => s.status === 'running');
	},

	addSubagent(info: SubagentInfo) {
		const updated = new Map(subagents);
		updated.set(info.id, info);
		subagents = updated;
	},

	updateProgress(id: string, round: number, maxRounds?: number, toolName?: string) {
		const existing = subagents.get(id);
		if (!existing) return;
		const updated = new Map(subagents);
		updated.set(id, {
			...existing,
			round,
			maxRounds: maxRounds ?? existing.maxRounds,
			toolName: toolName ?? existing.toolName,
			status: 'running',
		});
		subagents = updated;
	},

	completeSubagent(
		id: string,
		status: string,
		summary?: string,
		error?: string,
		durationMs?: number,
		toolCalls?: number,
		rounds?: number,
	) {
		const existing = subagents.get(id);
		if (!existing) return;
		const updated = new Map(subagents);
		updated.set(id, {
			...existing,
			status: status as SubagentInfo['status'],
			summary,
			error,
			durationMs,
			toolCalls,
			round: rounds ?? existing.round,
			completedAt: new Date().toISOString(),
		});
		subagents = updated;
	},

	forParentTask(parentTaskId: string): SubagentInfo[] {
		return [...subagents.values()].filter((s) => s.parentTaskId === parentTaskId);
	},

	clear() {
		subagents = new Map();
	},
};
