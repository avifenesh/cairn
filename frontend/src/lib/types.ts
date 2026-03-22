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
	archivedBySource?: Record<string, number>;
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
	mode?: string;
	metadata?: Record<string, unknown>;
	messageCount: number;
	updatedAt: string;
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

export interface MarketplaceSearchResult {
	score: number;
	slug: string;
	displayName: string;
	summary: string;
	version: string;
	updatedAt: number;
}

export interface MarketplaceSkill {
	slug: string;
	displayName: string;
	summary: string;
	stats: { downloads: number; stars: number; versions: number; installsAllTime: number };
	owner: { handle: string; displayName: string; image: string };
	latestVersion: { version: string; changelog: string };
	tags?: Record<string, string>;
	metadata?: Record<string, unknown>;
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

export interface SoulPatch {
	id: string;
	content: string;
	source: string;
	createdAt: string;
	preview: string;
}

export interface SkillSuggestion {
	slug: string;
	displayName: string;
	summary: string;
	reason: string;
	signal: string;
	score: number;
	createdAt: string;
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

export interface MCPConnection {
	name: string;
	transport: string;
	status: 'connected' | 'connecting' | 'disconnected' | 'error';
	toolCount: number;
	error?: string;
	connectedAt?: string;
}

export interface MCPServerConfig {
	name: string;
	transport: 'stdio' | 'http';
	command?: string;
	args?: string[];
	env?: string[];
	url?: string;
	headers?: Record<string, string>;
	disabled?: boolean;
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

export interface SubagentInfo {
	id: string;
	parentTaskId: string;
	type: 'researcher' | 'coder' | 'reviewer' | 'executor' | string;
	execMode: 'foreground' | 'background';
	status: 'running' | 'completed' | 'failed' | 'canceled';
	instruction: string;
	summary?: string;
	error?: string;
	round?: number;
	maxRounds?: number;
	toolName?: string;
	toolCalls?: number;
	durationMs?: number;
	createdAt: string;
	completedAt?: string;
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
	| 'budget_update'
	| 'subagent_started'
	| 'subagent_progress'
	| 'subagent_completed';

export interface CronJob {
	id: string;
	enabled: boolean;
	name: string;
	description?: string;
	schedule: string;
	instruction: string;
	timezone: string;
	priority: number;
	cooldownMs: number;
	agentType?: string;
	createdAt: string;
	updatedAt: string;
	lastRunAt?: string;
	nextRunAt?: string;
}

export interface ActivityEntry {
	id: string;
	type: 'task' | 'idle' | 'reflection' | 'cron' | 'error';
	summary: string;
	details?: string;
	errors: string[];
	toolCount: number;
	durationMs: number;
	createdAt: string;
}

export interface ToolStatsOverview {
	totalCalls: number;
	totalErrors: number;
	byTool: Record<string, number>;
	errorsByTool: Record<string, number>;
	tools: { toolName: string; calls: number; errors: number; totalMs: number; lastError?: string }[];
}

export interface CronExecution {
	id: string;
	cronJobId: string;
	taskId?: string;
	status: 'fired' | 'completed' | 'failed' | 'skipped_cooldown';
	error?: string;
	createdAt: string;
}

// --- Coding Session Panel ---

export type SessionEventType =
	| 'text_delta'
	| 'thinking'
	| 'tool_call'
	| 'tool_result'
	| 'file_change'
	| 'state_change'
	| 'round_complete'
	| 'approval_request'
	| 'approval_response'
	| 'user_steer'
	| 'session_start'
	| 'session_end';

export interface SessionEvent {
	sessionId: string;
	eventType: SessionEventType;
	payload: Record<string, unknown>;
	timestamp: string;
}

export type SessionStatus = 'running' | 'paused' | 'waiting_approval' | 'completed' | 'failed' | 'stopped';

export interface FileChange {
	path: string;
	operation: 'write' | 'delete' | 'rename';
	diff?: string;
}

export interface SessionRoundInfo {
	round: number;
	toolCalls: number;
	inputTokens: number;
	outputTokens: number;
}

// --- Automation Rules ---

export interface RuleTrigger {
	type: 'event' | 'cron';
	eventType?: string;
	filter?: Record<string, string>;
	schedule?: string;
}

export interface RuleAction {
	type: 'notify' | 'task';
	params: Record<string, string>;
}

export interface Rule {
	id: string;
	name: string;
	description: string;
	enabled: boolean;
	trigger: RuleTrigger;
	condition: string;
	actions: RuleAction[];
	throttleMs: number;
	createdAt: string;
	updatedAt: string;
	lastFiredAt?: string;
}

export interface RuleExecution {
	id: string;
	ruleId: string;
	triggerEvent?: string;
	status: 'success' | 'error' | 'throttled' | 'condition_false';
	error?: string;
	durationMs: number;
	createdAt: string;
}
