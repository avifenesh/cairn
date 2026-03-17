// Mock data fixtures for standalone frontend development (no backend needed)
// Plan Phase 1 step 6: "Mock data for development"

import type {
	FeedItem,
	DashboardResponse,
	Task,
	Approval,
	ChatSession,
	Memory,
	Agent,
	Skill,
	CostData,
} from '$lib/types';

export const mockFeedItems: FeedItem[] = [
	{ id: 1, source: 'github', kind: 'pull_request', title: 'feat: add streaming SSE support #142', url: 'https://github.com/avifenesh/cairn/pull/142', author: 'avifenesh', isRead: false, isArchived: false, createdAt: new Date(Date.now() - 300_000).toISOString() },
	{ id: 2, source: 'github', kind: 'issue', title: 'Bug: SQLite WAL checkpoint stalls under load', url: 'https://github.com/avifenesh/cairn/issues/87', author: 'contributor', isRead: false, isArchived: false, createdAt: new Date(Date.now() - 1_200_000).toISOString() },
	{ id: 3, source: 'reddit', kind: 'post', title: 'Show HN: Personal agent OS in Go - single binary', url: 'https://reddit.com/r/golang/foo', author: 'u/avifenesh', isRead: true, isArchived: false, createdAt: new Date(Date.now() - 3_600_000).toISOString() },
	{ id: 4, source: 'hackernews', kind: 'story', title: 'Why We Switched from TypeScript to Go for Our Agent Runtime', url: 'https://news.ycombinator.com/item?id=99999', isRead: false, isArchived: false, createdAt: new Date(Date.now() - 7_200_000).toISOString() },
	{ id: 5, source: 'npm', kind: 'release', title: '@anthropic-ai/sdk@4.2.0 released', isRead: true, isArchived: false, createdAt: new Date(Date.now() - 14_400_000).toISOString() },
	{ id: 6, source: 'gmail', kind: 'email', title: 'Re: Cairn open-source launch timeline', author: 'collaborator@example.com', isRead: false, isArchived: false, createdAt: new Date(Date.now() - 18_000_000).toISOString() },
	{ id: 7, source: 'github', kind: 'push', title: 'Push to main: 3 commits (event bus + LLM client)', author: 'avifenesh', isRead: true, isArchived: false, createdAt: new Date(Date.now() - 43_200_000).toISOString() },
	{ id: 8, source: 'crates', kind: 'release', title: 'mcp-sdk 0.3.0 - breaking: new transport API', isRead: false, isArchived: false, createdAt: new Date(Date.now() - 86_400_000).toISOString() },
];

export const mockDashboard: DashboardResponse = {
	stats: { total: 247, unread: 12, bySource: { github: 89, reddit: 34, hackernews: 41, npm: 28, gmail: 19, crates: 15, x: 11, agent: 10 } },
	feed: mockFeedItems,
	poller: { running: true, sources: { github: { itemCount: 89 }, reddit: { itemCount: 34 } } },
	readiness: { ready: true, checks: { database: true, poller: true, writeToken: true } },
};

export const mockTasks: Task[] = [
	{ id: 'task-001', type: 'coding', status: 'running', title: 'Implement tool registry with mode filtering', createdAt: new Date(Date.now() - 600_000).toISOString(), updatedAt: new Date(Date.now() - 60_000).toISOString() },
	{ id: 'task-002', type: 'chat', status: 'completed', title: 'Explain SQLite WAL mode trade-offs', createdAt: new Date(Date.now() - 3_600_000).toISOString(), updatedAt: new Date(Date.now() - 3_000_000).toISOString() },
	{ id: 'task-003', type: 'coding', status: 'pending', title: 'Add worktree isolation to task engine', createdAt: new Date(Date.now() - 7_200_000).toISOString(), updatedAt: new Date(Date.now() - 7_200_000).toISOString() },
	{ id: 'task-004', type: 'work', status: 'failed', title: 'Generate weekly digest', error: 'LLM rate limit exceeded', createdAt: new Date(Date.now() - 86_400_000).toISOString(), updatedAt: new Date(Date.now() - 82_800_000).toISOString() },
];

export const mockApprovals: Approval[] = [
	{ id: 'appr-001', type: 'merge_pr', status: 'pending', title: 'Merge PR #142: SSE streaming support', description: 'All checks passed. 3 files changed, +247 -12.', createdAt: new Date(Date.now() - 300_000).toISOString() },
	{ id: 'appr-002', type: 'budget_override', status: 'pending', title: 'Budget override: coding task exceeding $2.50 daily limit', description: 'Current spend: $2.41. Requested additional: $1.00.', createdAt: new Date(Date.now() - 900_000).toISOString() },
];

export const mockSessions: ChatSession[] = [
	{ id: 'sess-001', title: 'Tool system design', messageCount: 14, lastMessageAt: new Date(Date.now() - 1_800_000).toISOString(), createdAt: new Date(Date.now() - 86_400_000).toISOString() },
	{ id: 'sess-002', title: 'SQLite migration debugging', messageCount: 8, lastMessageAt: new Date(Date.now() - 43_200_000).toISOString(), createdAt: new Date(Date.now() - 172_800_000).toISOString() },
];

export const mockMemories: Memory[] = [
	{ id: 'mem-001', category: 'preference', status: 'accepted', content: 'Prefers concise, direct communication without emojis.', createdAt: new Date(Date.now() - 604_800_000).toISOString() },
	{ id: 'mem-002', category: 'project', status: 'accepted', content: 'Cairn uses Go 1.25 with modernc SQLite (pure Go, no CGO).', createdAt: new Date(Date.now() - 259_200_000).toISOString() },
	{ id: 'mem-003', category: 'process', status: 'proposed', content: 'Always run go vet before committing. Pre-push checklist includes /deslop and /drift-detect.', createdAt: new Date(Date.now() - 3_600_000).toISOString() },
	{ id: 'mem-004', category: 'general', status: 'proposed', content: 'The frontend uses Svelte 5 runes with .svelte.ts store files.', createdAt: new Date(Date.now() - 7_200_000).toISOString() },
];

export const mockAgents: Agent[] = [
	{ id: 'agent-main', name: 'cairn-main', type: 'assistant', status: 'idle', lastHeartbeat: new Date(Date.now() - 30_000).toISOString() },
	{ id: 'agent-coder', name: 'cairn-coder', type: 'coding', status: 'busy', currentTask: 'Implement tool registry', lastHeartbeat: new Date(Date.now() - 5_000).toISOString() },
];

export const mockSkills: Skill[] = [
	{ name: 'web-search', description: 'Search the web via SearXNG', scope: 'global', inclusion: 'on-demand', disableModelInvocation: false, userInvocable: true },
	{ name: 'deploy', description: 'Deploy backend via systemd restart', scope: 'global', inclusion: 'on-demand', disableModelInvocation: true, userInvocable: true },
	{ name: 'email-triage', description: 'Triage incoming emails with dual-LLM pattern', scope: 'global', inclusion: 'always', disableModelInvocation: false, userInvocable: false },
];

export const mockCosts: CostData = {
	todayUsd: 1.47,
	weekUsd: 8.23,
	budgetDailyUsd: 5.00,
	budgetWeeklyUsd: 25.00,
};

export const useMocks = (): boolean => {
	try {
		return localStorage.getItem('pub_use_mocks') === 'true';
	} catch {
		return false;
	}
};
