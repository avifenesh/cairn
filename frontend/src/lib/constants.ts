// Shared constants — single source of truth for values used across components

export const MEMORY_CATEGORIES = [
	{ value: 'fact', label: 'Fact' },
	{ value: 'preference', label: 'Preference' },
	{ value: 'hard_rule', label: 'Hard Rule' },
	{ value: 'decision', label: 'Decision' },
	{ value: 'writing_style', label: 'Writing Style' },
] as const;
