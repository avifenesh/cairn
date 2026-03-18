// Typed REST API client for Cairn backend

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
	const token = localStorage.getItem('cairn_api_token');
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
export const getDashboard = async (params?: { limit?: number; source?: string }): Promise<DashboardResponse> => {
	if (useMocks()) return mockDashboard;
	const q = new URLSearchParams();
	if (params?.limit) q.set('limit', String(params.limit));
	if (params?.source) q.set('source', params.source);
	const qs = q.toString();
	const raw = await get<Record<string, unknown>>(`/v1/dashboard${qs ? '?' + qs : ''}`);
	// Normalize: backend may not return all fields the frontend expects
	return {
		feed: (raw.feed as DashboardResponse['feed']) ?? [],
		stats: {
			total: (raw.stats as Record<string, unknown>)?.total as number ?? 0,
			unread: (raw.stats as Record<string, unknown>)?.unread as number ?? 0,
			bySource: (raw.stats as Record<string, unknown>)?.bySource as Record<string, number> ?? {},
		},
		poller: {
			running: (raw.poller as Record<string, unknown>)?.running as boolean ?? false,
			sources: (raw.poller as Record<string, unknown>)?.sources as Record<string, unknown> ?? {},
		} as DashboardResponse['poller'],
		readiness: (raw.readiness as DashboardResponse['readiness']) ?? { ready: true, checks: {} },
	};
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
export const getTasks = async (params?: { status?: string; type?: string }) => {
	if (useMocks()) return { items: mockTasks, hasMore: false };
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	if (params?.type) q.set('type', params.type);
	const qs = q.toString();
	const raw = await get<Record<string, unknown>>(`/v1/tasks${qs ? '?' + qs : ''}`);
	return { items: (raw.tasks ?? raw.items ?? []) as Task[], hasMore: (raw.hasMore ?? false) as boolean };
};

export const cancelTask = (id: string) => post<{ ok: boolean }>(`/v1/tasks/${id}/cancel`);

// Approvals
export const getApprovals = async (params?: { status?: string }) => {
	if (useMocks()) return { items: mockApprovals, hasMore: false };
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	const qs = q.toString();
	const raw = await get<Record<string, unknown>>(`/v1/approvals${qs ? '?' + qs : ''}`);
	return { items: (raw.approvals ?? raw.items ?? []) as Approval[], hasMore: (raw.hasMore ?? false) as boolean };
};

export const approve = (id: string) => post<{ ok: boolean }>(`/v1/approvals/${id}/approve`);
export const deny = (id: string) => post<{ ok: boolean }>(`/v1/approvals/${id}/deny`);

// Assistant / Chat
export const getSessions = async () => {
	if (useMocks()) return { items: mockSessions };
	const raw = await get<Record<string, unknown>>('/v1/assistant/sessions');
	return { items: (raw.sessions ?? raw.items ?? []) as ChatSession[] };
};

export const getSessionMessages = async (sessionId: string) => {
	const raw = await get<Record<string, unknown>>(`/v1/assistant/sessions/${sessionId}`);
	return { items: (raw.messages ?? raw.events ?? raw.items ?? []) as ChatMessage[] };
};

export const sendMessage = (message: string, mode?: ChatMode, sessionId?: string) =>
	post<{ taskId: string }>('/v1/assistant/message', { message, mode, sessionId });

// Voice
export const uploadVoice = async (audio: Blob, mode?: ChatMode, sessionId?: string) => {
	const form = new FormData();
	form.append('audio', audio);
	if (mode) form.append('mode', mode);
	if (sessionId) form.append('sessionId', sessionId);
	const h: HeadersInit = {};
	const token = localStorage.getItem('cairn_api_token');
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
export const getMemories = async (params?: { status?: string; category?: string }) => {
	if (useMocks()) return { items: mockMemories, hasMore: false };
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	if (params?.category) q.set('category', params.category);
	const qs = q.toString();
	const raw = await get<Record<string, unknown>>(`/v1/memories${qs ? '?' + qs : ''}`);
	return { items: (raw.memories ?? raw.items ?? []) as Memory[], hasMore: (raw.hasMore ?? false) as boolean };
};

export const searchMemories = (query: string, limit = 10) =>
	get<{ items: Memory[] }>(`/v1/memories/search?q=${encodeURIComponent(query)}&limit=${limit}`);

export const createMemory = (content: string, category: string) =>
	post<Memory>('/v1/memories', { content, category });
export const acceptMemory = (id: string) => post<{ ok: boolean }>(`/v1/memories/${id}/accept`);
export const rejectMemory = (id: string) => post<{ ok: boolean }>(`/v1/memories/${id}/reject`);

// Fleet / Agents
export const getFleet = async () => {
	if (useMocks()) return { agents: mockAgents, summary: { idle: 1, busy: 1 } };
	try {
		return await get<{ agents: Agent[]; summary: Record<string, number> }>('/v1/fleet');
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) return { agents: [], summary: {} };
		throw e;
	}
};

// Skills
export const getSkills = async () => {
	if (useMocks()) return { items: mockSkills, summary: { total: 3 }, currentlyActive: ['web-search'] };
	const raw = await get<Record<string, unknown>>('/v1/skills');
	return {
		items: (raw.skills ?? raw.items ?? []) as Skill[],
		summary: (raw.summary ?? {}) as Record<string, number>,
		currentlyActive: (raw.currentlyActive ?? []) as string[],
	};
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

// Auth (WebAuthn)
export const authLoginStart = () => post<{ challenge: string }>('/v1/auth/login/start');
export const authLoginComplete = (credential: unknown) =>
	post<{ ok: boolean }>('/v1/auth/login/complete', credential);
export const authRegisterStart = () => post<{ challenge: string }>('/v1/auth/register/start');
export const authRegisterComplete = (credential: unknown) =>
	post<{ ok: boolean }>('/v1/auth/register/complete', credential);
export const authLogout = () => post<{ ok: boolean }>('/v1/auth/logout');
