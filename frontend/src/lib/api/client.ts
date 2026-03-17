// Typed REST API client for Pub backend

import type {
	FeedItem,
	DashboardResponse,
	Task,
	Approval,
	ChatSession,
	ChatMessage,
	ChatMode,
	Memory,
	Skill,
	SoulContent,
	SoulHistoryEntry,
	CostData,
	Agent,
} from '$lib/types';

import {
	useMocks,
	mockDashboard,
	mockFeedItems,
	mockTasks,
	mockApprovals,
	mockSessions,
	mockMemories,
	mockAgents,
	mockSkills,
	mockCosts,
} from './mock';

const BASE_URL = '';

function headers(): HeadersInit {
	const h: HeadersInit = { 'Content-Type': 'application/json' };
	const token = localStorage.getItem('pub_api_token');
	if (token) h['X-Api-Token'] = token;
	return h;
}

async function get<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE_URL}${path}`, {
		credentials: 'include',
		headers: headers(),
	});
	if (!res.ok) throw new ApiError(res.status, await res.text());
	return res.json();
}

async function post<T>(path: string, body?: unknown): Promise<T> {
	const res = await fetch(`${BASE_URL}${path}`, {
		method: 'POST',
		credentials: 'include',
		headers: headers(),
		body: body ? JSON.stringify(body) : undefined,
	});
	if (!res.ok) throw new ApiError(res.status, await res.text());
	return res.json();
}

async function put<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE_URL}${path}`, {
		method: 'PUT',
		credentials: 'include',
		headers: headers(),
		body: JSON.stringify(body),
	});
	if (!res.ok) throw new ApiError(res.status, await res.text());
	return res.json();
}

export class ApiError extends Error {
	constructor(
		public status: number,
		public body: string,
	) {
		super(`API ${status}: ${body}`);
	}
}

// Health
export const health = () => {
	if (useMocks()) return Promise.resolve({ ok: true });
	return get<{ ok: boolean }>('/health');
};

// Dashboard
export const getDashboard = (params?: { limit?: number; source?: string }) => {
	if (useMocks()) return Promise.resolve(mockDashboard);
	const q = new URLSearchParams();
	if (params?.limit) q.set('limit', String(params.limit));
	if (params?.source) q.set('source', params.source);
	const qs = q.toString();
	return get<DashboardResponse>(`/v1/dashboard${qs ? '?' + qs : ''}`);
};

// Feed
export const getFeed = (params?: {
	limit?: number;
	before?: string;
	source?: string;
	unread?: boolean;
}) => {
	if (useMocks()) return Promise.resolve({ items: mockFeedItems, hasMore: false });
	const q = new URLSearchParams();
	if (params?.limit) q.set('limit', String(params.limit));
	if (params?.before) q.set('before', params.before);
	if (params?.source) q.set('source', params.source);
	if (params?.unread !== undefined) q.set('unread', String(params.unread));
	const qs = q.toString();
	return get<{ items: FeedItem[]; hasMore: boolean }>(`/v1/feed${qs ? '?' + qs : ''}`);
};

export const markRead = (id: number) => {
	if (useMocks()) return Promise.resolve({ ok: true });
	return post<{ ok: boolean }>(`/v1/feed/${id}/read`);
};
export const markAllRead = () => {
	if (useMocks()) return Promise.resolve({ changed: 0 });
	return post<{ changed: number }>('/v1/feed/read-all');
};

// Tasks
export const getTasks = (params?: { status?: string; type?: string }) => {
	if (useMocks()) return Promise.resolve({ items: mockTasks, hasMore: false });
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	if (params?.type) q.set('type', params.type);
	const qs = q.toString();
	return get<{ items: Task[]; hasMore: boolean }>(`/v1/tasks${qs ? '?' + qs : ''}`);
};

export const cancelTask = (id: string) => post<{ ok: boolean }>(`/v1/tasks/${id}/cancel`);

// Approvals
export const getApprovals = (params?: { status?: string }) => {
	if (useMocks()) return Promise.resolve({ items: mockApprovals, hasMore: false });
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	const qs = q.toString();
	return get<{ items: Approval[]; hasMore: boolean }>(`/v1/approvals${qs ? '?' + qs : ''}`);
};

export const approve = (id: string) => post<{ ok: boolean }>(`/v1/approvals/${id}/approve`);
export const deny = (id: string) => post<{ ok: boolean }>(`/v1/approvals/${id}/deny`);

// Assistant / Chat
export const getSessions = () => {
	if (useMocks()) return Promise.resolve({ items: mockSessions });
	return get<{ items: ChatSession[] }>('/v1/assistant/sessions');
};

export const getSessionMessages = (sessionId: string) =>
	get<{ items: ChatMessage[] }>(`/v1/assistant/sessions/${sessionId}`);

export const sendMessage = (message: string, mode?: ChatMode, sessionId?: string) =>
	post<{ taskId: string }>('/v1/assistant/message', { message, mode, sessionId });

// Voice
export const uploadVoice = async (audio: Blob, mode?: ChatMode, sessionId?: string) => {
	const form = new FormData();
	form.append('audio', audio);
	if (mode) form.append('mode', mode);
	if (sessionId) form.append('sessionId', sessionId);
	const h: HeadersInit = {};
	const token = localStorage.getItem('pub_api_token');
	if (token) h['X-Api-Token'] = token;
	const res = await fetch(`${BASE_URL}/v1/assistant/voice`, {
		method: 'POST',
		credentials: 'include',
		headers: h,
		body: form,
	});
	if (!res.ok) throw new ApiError(res.status, await res.text());
	return res.json() as Promise<{ taskId: string; transcript: string }>;
};

// Memories
export const getMemories = (params?: { status?: string; category?: string }) => {
	if (useMocks()) return Promise.resolve({ items: mockMemories, hasMore: false });
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	if (params?.category) q.set('category', params.category);
	const qs = q.toString();
	return get<{ items: Memory[]; hasMore: boolean }>(`/v1/memories${qs ? '?' + qs : ''}`);
};

export const searchMemories = (query: string, limit = 10) =>
	get<{ items: Memory[] }>(`/v1/memories/search?q=${encodeURIComponent(query)}&limit=${limit}`);

export const createMemory = (content: string, category: string) =>
	post<Memory>('/v1/memories', { content, category });
export const acceptMemory = (id: string) => post<{ ok: boolean }>(`/v1/memories/${id}/accept`);
export const rejectMemory = (id: string) => post<{ ok: boolean }>(`/v1/memories/${id}/reject`);

// Fleet / Agents
export const getFleet = () => {
	if (useMocks()) return Promise.resolve({ agents: mockAgents, summary: { idle: 1, busy: 1 } });
	return get<{ agents: Agent[]; summary: Record<string, number> }>('/v1/fleet');
};

// Skills
export const getSkills = () => {
	if (useMocks()) return Promise.resolve({ items: mockSkills, summary: { total: 3 }, currentlyActive: ['web-search'] });
	return get<{ items: Skill[]; summary: Record<string, number>; currentlyActive?: string[] }>('/v1/skills');
};

// Soul
export const getSoul = () => get<SoulContent>('/v1/soul');
export const updateSoul = (content: string) => put<{ ok: boolean; sha: string }>('/v1/soul', { content });
export const getSoulHistory = () => get<{ items: SoulHistoryEntry[] }>('/v1/soul/history');
export const getSoulPatches = () => get<{ items: unknown[] }>('/v1/soul/patches');

// Metrics / Costs
export const getCosts = () => {
	if (useMocks()) return Promise.resolve(mockCosts);
	return get<CostData>('/v1/costs');
};
export const getMetrics = () => get<Record<string, unknown>>('/v1/metrics');
export const getStatus = () => get<Record<string, unknown>>('/v1/status');

// Poll
export const triggerPoll = () => post<{ ok: boolean }>('/v1/poll/run');
