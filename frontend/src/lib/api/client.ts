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
	CronJob,
	CronExecution,
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

async function del<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE_URL}${path}`, {
		method: 'DELETE',
		credentials: 'include',
		headers: headers(),
	});
	if (!res.ok) throw new ApiError(res.status, await res.text());
	return res.json();
}

async function patch<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE_URL}${path}`, {
		method: 'PATCH',
		credentials: 'include',
		headers: headers(),
		body: JSON.stringify(body),
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
	kind?: string;
	unread?: boolean;
}) => {
	if (useMocks()) return Promise.resolve({ items: mockFeedItems, hasMore: false });
	const q = new URLSearchParams();
	if (params?.limit) q.set('limit', String(params.limit));
	if (params?.before) q.set('before', params.before);
	if (params?.source) q.set('source', params.source);
	if (params?.kind) q.set('kind', params.kind);
	if (params?.unread !== undefined) q.set('unread', String(params.unread));
	const qs = q.toString();
	return get<{ items: FeedItem[]; hasMore: boolean }>(`/v1/feed${qs ? '?' + qs : ''}`);
};

export const markRead = (id: string) => {
	if (useMocks()) return Promise.resolve({ ok: true });
	return post<{ ok: boolean }>(`/v1/feed/${id}/read`);
};
export const markAllRead = () => {
	if (useMocks()) return Promise.resolve({ changed: 0 });
	return post<{ changed: number }>('/v1/feed/read-all');
};
export const archiveFeedItem = (id: string) => {
	if (useMocks()) return Promise.resolve({ ok: true });
	return post<{ ok: boolean }>(`/v1/feed/${id}/archive`);
};
export const deleteFeedItem = (id: string) => {
	if (useMocks()) return Promise.resolve({ ok: true });
	return del<{ ok: boolean }>(`/v1/feed/${id}`);
};

// Tasks
export const getTasks = async (params?: { status?: string; type?: string }) => {
	if (useMocks()) return { items: mockTasks, hasMore: false };
	const q = new URLSearchParams();
	if (params?.status) q.set('status', params.status);
	if (params?.type) q.set('type', params.type);
	const qs = q.toString();
	const raw = await get<Record<string, unknown>>(`/v1/tasks${qs ? '?' + qs : ''}`);
	const tasks = ((raw.tasks ?? raw.items ?? []) as Array<Record<string, unknown>>).map((t) => {
		const input = t.input as { message?: string } | undefined;
		return {
			...t,
			title: t.title || t.description || input?.message || t.type || 'Task',
			status: t.status === 'queued' ? 'pending' : t.status === 'canceled' ? 'cancelled' : t.status,
			input,
		};
	}) as Task[];
	return { items: tasks, hasMore: (raw.hasMore ?? false) as boolean };
};

export const cancelTask = (id: string) => post<{ ok: boolean }>(`/v1/tasks/${id}/cancel`);
export const deleteTask = (id: string) => del<{ ok: boolean }>(`/v1/tasks/${id}`);
export const createTask = async (description: string, type = 'general', priority = 2) => {
	const raw = await post<Record<string, unknown>>('/v1/tasks', { description, type, priority });
	return {
		...raw,
		title: (raw.title ?? raw.description ?? description) as string,
		status: raw.status === 'queued' ? 'pending' : raw.status,
	} as Task;
};

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
	// If backend returns pre-formatted messages, use them directly
	if (raw.messages) return { items: raw.messages as ChatMessage[] };
	// Otherwise normalize events (parts-based format) into ChatMessage
	const events = (raw.events ?? raw.items ?? []) as Array<Record<string, unknown>>;
	const messages: ChatMessage[] = [];
	let currentText = '';
	let currentAuthor = '';
	let currentTimestamp = '';
	let currentId = '';
	for (const ev of events) {
		const author = (ev.author as string) ?? '';
		const parts = (ev.parts as Array<{ type: string; text?: string }>) ?? [];
		const timestamp = (ev.timestamp as string) ?? (ev.createdAt as string) ?? '';
		const textParts = parts.filter((p) => p.type === 'text').map((p) => p.text ?? '');
		if (textParts.length === 0) continue;
		// Aggregate consecutive events from the same author
		if (author === currentAuthor && currentText) {
			currentText += textParts.join('');
			currentTimestamp = timestamp || currentTimestamp;
		} else {
			if (currentText) {
				messages.push({ id: currentId, role: currentAuthor === 'user' ? 'user' : 'assistant', content: currentText, createdAt: currentTimestamp });
			}
			currentText = textParts.join('');
			currentAuthor = author;
			currentTimestamp = timestamp;
			currentId = (ev.id as string) ?? crypto.randomUUID();
		}
	}
	if (currentText) {
		messages.push({ id: currentId, role: currentAuthor === 'user' ? 'user' : 'assistant', content: currentText, createdAt: currentTimestamp });
	}
	return { items: messages };
};

export const sendMessage = (message: string, mode?: ChatMode, sessionId?: string) =>
	post<{ taskId: string; sessionId?: string }>('/v1/assistant/message', { message, mode, sessionId });

// Voice — transcribe audio to text via Whisper STT
export const transcribeVoice = async (audio: Blob): Promise<string> => {
	const form = new FormData();
	form.append('audio', audio);
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
	const data = await res.json() as { ok: boolean; text: string };
	return data.text;
};

// Voice — kept for backward compat, chains transcribe → sendMessage
export const uploadVoice = async (audio: Blob, mode?: ChatMode, sessionId?: string) => {
	const transcript = await transcribeVoice(audio);
	const res = await sendMessage(transcript, mode, sessionId);
	return { taskId: res.taskId, sessionId: res.sessionId, transcript };
};

// File upload (for vision tools)
export const uploadFile = async (file: File): Promise<{ path: string; name: string; size: number; mimeType: string }> => {
	const form = new FormData();
	form.append('file', file);
	const h: HeadersInit = {};
	const token = localStorage.getItem('cairn_api_token');
	if (token) h['X-Api-Token'] = token;
	const res = await fetch(`${BASE_URL}/v1/upload`, {
		method: 'POST',
		credentials: 'include',
		headers: h,
		body: form,
	});
	if (!res.ok) throw new ApiError(res.status, await res.text());
	return res.json();
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

export const searchMemories = async (query: string, limit = 10) => {
	const raw = await get<Record<string, unknown>>(`/v1/memories/search?q=${encodeURIComponent(query)}&limit=${limit}`);
	const results = (raw.results ?? []) as Array<{ memory: Memory; score: number }>;
	return { items: results.map((r) => ({ ...r.memory, confidence: r.score })) as Memory[] };
};

export const createMemory = (content: string, category: string) =>
	post<Memory>('/v1/memories', { content, category });
export const acceptMemory = (id: string) => post<{ ok: boolean }>(`/v1/memories/${id}/accept`);
export const rejectMemory = (id: string) => post<{ ok: boolean }>(`/v1/memories/${id}/reject`);
export const deleteMemory = (id: string) => del<{ ok: boolean }>(`/v1/memories/${id}`);
export const updateMemory = (id: string, content: string, category?: string) =>
	put<{ ok: boolean; memory: Memory }>(`/v1/memories/${id}`, { content, category });

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

export const getSkillDetail = async (name: string) => {
	const raw = await get<Record<string, unknown>>(`/v1/skills/${encodeURIComponent(name)}`);
	return raw as unknown as Skill & { content: string };
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

export const getMcpStatus = async () => {
	try {
		const raw = await get<Record<string, unknown>>('/v1/status');
		const mcp = raw.mcp as Record<string, unknown> | undefined;
		if (!mcp) return null;
		return { enabled: !!mcp.enabled, port: (mcp.port ?? 3001) as number, transport: (mcp.transport ?? 'http') as string };
	} catch { return null; }
};

export const getStatusDetails = async () => {
	try {
		const raw = await get<Record<string, unknown>>('/v1/status');
		const mcp = raw.mcp as Record<string, unknown> | undefined;
		const ch = raw.channels as Record<string, unknown> | undefined;
		const emb = raw.embeddings as Record<string, unknown> | undefined;
		const comp = raw.compaction as Record<string, unknown> | undefined;
		return {
			mcp: mcp ? { enabled: !!mcp.enabled, port: (mcp.port ?? 3001) as number, transport: (mcp.transport ?? 'http') as string } : null,
			channels: ch ? { items: ((ch.items ?? []) as { name: string; connected: boolean }[]), sessionTimeout: (ch.sessionTimeout ?? 240) as number } : null,
			embeddings: emb ? { enabled: !!emb.enabled, model: (emb.model ?? '') as string, dimensions: (emb.dimensions ?? 0) as number } : null,
			compaction: comp ? { triggerTokens: (comp.triggerTokens ?? 80000) as number, keepRecent: (comp.keepRecent ?? 10) as number, maxToolOutput: (comp.maxToolOutput ?? 8000) as number } : null,
		};
	} catch { return { mcp: null, channels: null }; }
};

// Config (runtime-editable)
export interface EditableConfig {
	compactionTriggerTokens?: number;
	compactionKeepRecent?: number;
	compactionMaxToolOutput?: number;
	ghOwner?: string;
	ghTrackedRepos?: string;
	ghBotFilter?: string;
	ghMetricsInterval?: number;
	gmailEnabled?: boolean;
	calendarEnabled?: boolean;
	gmailFilterQuery?: string;
	calendarLookaheadH?: number;
	rssEnabled?: boolean;
	rssFeeds?: string;
	soEnabled?: boolean;
	soTags?: string;
	devtoEnabled?: boolean;
	devtoTags?: string;
	devtoUsername?: string;
	npmPackages?: string;
	cratesPackages?: string;
	budgetDailyCap?: number;
	budgetWeeklyCap?: number;
	channelSessionTimeout?: number;
	preferredChannel?: string;
	quietHoursStart?: number;
	quietHoursEnd?: number;
	quietHoursTZ?: string;
	mutedSources?: string;
	notifMinPriority?: string;
	channelRouting?: string;
}
export const getEditableConfig = () => get<EditableConfig>('/v1/config');
export const patchConfig = (cfg: Partial<EditableConfig>) =>
	patch<{ ok: boolean; config: EditableConfig }>('/v1/config', cfg);

// Poll
export const triggerPoll = () => post<{ ok: boolean }>('/v1/poll/run');

// Cron Jobs
export const getCrons = () => get<{ items: CronJob[]; count: number }>('/v1/crons');
export const createCron = (body: {
	name: string;
	schedule: string;
	instruction: string;
	description?: string;
	priority?: number;
	timezone?: string;
	cooldownMs?: number;
}) => post<CronJob>('/v1/crons', body);
export const getCronDetail = (id: string) =>
	get<{ job: CronJob; executions: CronExecution[] }>(`/v1/crons/${id}`);
export const updateCron = (id: string, body: {
	enabled?: boolean;
	schedule?: string;
	instruction?: string;
	description?: string;
	priority?: number;
}) => patch<{ ok: boolean; job: CronJob }>(`/v1/crons/${id}`, body);
export const deleteCron = (id: string) => del<{ ok: boolean }>(`/v1/crons/${id}`);

// Auth (WebAuthn)
export const authLoginStart = () => post<{ challenge: string }>('/v1/auth/login/start');
export const authLoginComplete = (credential: unknown) =>
	post<{ ok: boolean }>('/v1/auth/login/complete', credential);
export const authRegisterStart = () => post<{ challenge: string }>('/v1/auth/register/start');
export const authRegisterComplete = (credential: unknown) =>
	post<{ ok: boolean }>('/v1/auth/register/complete', credential);
export const authLogout = () => post<{ ok: boolean }>('/v1/auth/logout');
