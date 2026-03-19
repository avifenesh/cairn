// Core domain types matching the Go backend API contract

export interface FeedItem {
	id: string;
	source: string;
	kind: string;
	title: string;
	body?: string;
	url?: string;
	author?: string;
	avatarUrl?: string;
	repoFullName?: string;
	isRead: boolean;
	isArchived: boolean;
	groupKey?: string;
	metadata?: Record<string, unknown>;
	createdAt: string;
}

export interface DashboardResponse {
	stats: FeedStats;
	feed: FeedItem[];
	poller: PollerStatus;
	readiness: ReadinessCheck;
}

export interface FeedStats {
	total: number;
	unread: number;
	bySource: Record<string, number>;
}

export interface PollerStatus {
	running: boolean;
	lastPollAt?: string;
	sources: Record<string, SourceStatus>;
}

export interface SourceStatus {
	lastPollAt?: string;
	lastError?: string;
	itemCount: number;
}

export interface ReadinessCheck {
	ready: boolean;
	checks: Record<string, boolean>;
}

export interface Task {
	id: string;
	type: string;
	status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
	title: string;
	description?: string;
	progress?: number;
	result?: string;
	error?: string;
	sessionId?: string;
	mode?: string;
	input?: { message?: string };
	createdAt: string;
	updatedAt: string;
}

export interface Approval {
	id: string;
	type: string;
	status: 'pending' | 'approved' | 'denied';
	title: string;
	description?: string;
	context?: Record<string, unknown>;
	createdAt: string;
	decidedAt?: string;
}

export interface ChatMessage {
	id: string;
	role: 'user' | 'assistant';
	content: string;
	mode?: ChatMode;
	toolCalls?: ToolCall[];
	reasoning?: ReasoningStep[];
	createdAt: string;
}

export interface ToolCall {
	toolName: string;
	phase: 'start' | 'result';
	args?: Record<string, unknown>;
	result?: string;
	error?: string;
	durationMs?: number;
	isExternal?: boolean;
}

export interface ReasoningStep {
	round: number;
	thought: string;
}

export interface ChatSession {
	id: string;
	title?: string;
	messageCount: number;
	lastMessageAt: string;
	createdAt: string;
}

export type ChatMode = 'talk' | 'work' | 'coding';

export interface Memory {
	id: string;
	category: string;
	status: 'proposed' | 'accepted' | 'rejected';
	content: string;
	source?: string;
	confidence?: number;
	createdAt: string;
}

export interface Agent {
	id: string;
	name: string;
	type: string;
	status: 'idle' | 'busy' | 'offline';
	currentTask?: string;
	lastHeartbeat?: string;
}

export interface Skill {
	name: string;
	description: string;
	scope: string;
	inclusion: string;
	disableModelInvocation: boolean;
	userInvocable: boolean;
	allowedTools?: string[];
}

export interface SoulContent {
	content: string;
	sha?: string;
}

export interface SoulHistoryEntry {
	sha: string;
	message: string;
	date: string;
}

export interface CostData {
	todayUsd: number;
	weekUsd: number;
	budgetDailyUsd: number;
	budgetWeeklyUsd: number;
}

export interface McpStatus {
	enabled: boolean;
	port: number;
	transport: string;
}

export interface ChannelInfo {
	name: string;
	connected: boolean;
}

export interface ChannelStatus {
	items: ChannelInfo[];
	sessionTimeout: number;
}

export interface Attachment {
	path: string;
	name: string;
	size: number;
	mimeType: string;
}

// SSE event types
export type SSEEventType =
	| 'ready'
	| 'feed_update'
	| 'poll_completed'
	| 'task_update'
	| 'approval_required'
	| 'assistant_delta'
	| 'assistant_end'
	| 'assistant_reasoning'
	| 'assistant_tool_call'
	| 'memory_proposed'
	| 'memory_accepted'
	| 'soul_updated'
	| 'digest_ready'
	| 'coding_session_event'
	| 'agent_progress'
	| 'skill_activated'
	| 'budget_update';
