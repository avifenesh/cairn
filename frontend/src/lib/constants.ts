// Shared constants — single source of truth for values used across components

export const MEMORY_CATEGORIES = [
	{ value: 'fact', label: 'Fact' },
	{ value: 'preference', label: 'Preference' },
	{ value: 'hard_rule', label: 'Hard Rule' },
	{ value: 'decision', label: 'Decision' },
	{ value: 'writing_style', label: 'Writing Style' },
] as const;

export const TASK_TYPES = [
	{ value: 'general', label: 'General' },
	{ value: 'chat', label: 'Chat' },
	{ value: 'coding', label: 'Coding' },
	{ value: 'triage', label: 'Triage' },
	{ value: 'workflow', label: 'Workflow' },
	{ value: 'digest', label: 'Digest' },
] as const;

export const TASK_PRIORITIES = [
	{ value: 0, label: 'Critical' },
	{ value: 1, label: 'High' },
	{ value: 2, label: 'Normal' },
	{ value: 3, label: 'Low' },
	{ value: 4, label: 'Idle' },
] as const;
