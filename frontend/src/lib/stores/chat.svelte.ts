// Chat store — sessions, messages, streaming state

import type { ChatMessage, ChatSession, ChatMode, ToolCall, ReasoningStep } from '$lib/types';

function restoreSessionId(): string | null {
	try { return localStorage.getItem('cairn_current_session'); } catch { return null; }
}

interface StreamingMessage {
	taskId: string;
	content: string;
	toolCalls: ToolCall[];
	reasoning: ReasoningStep[];
	isStreaming: boolean;
}

let sessions = $state<ChatSession[]>([]);
let currentSessionId = $state<string | null>(restoreSessionId());
let messages = $state<ChatMessage[]>([]);
let streamingMessages = $state<Map<string, StreamingMessage>>(new Map());
let mode = $state<ChatMode>('talk');
let loading = $state(false);
let pendingMessage = $state<string | null>(null);

export const chatStore = {
	get sessions() { return sessions; },
	get currentSessionId() { return currentSessionId; },
	get messages() { return messages; },
	get streamingMessages() { return streamingMessages; },
	get mode() { return mode; },
	get loading() { return loading; },
	get pendingMessage() { return pendingMessage; },
	get activeStream() {
		for (const sm of streamingMessages.values()) {
			if (sm.isStreaming) return sm;
		}
		return null;
	},

	setSessions(s: ChatSession[]) { sessions = s; },
	setCurrentSession(id: string | null) {
		currentSessionId = id;
		try {
			if (id) localStorage.setItem('cairn_current_session', id);
			else localStorage.removeItem('cairn_current_session');
		} catch (err) {
			console.warn('[chat] localStorage unavailable:', err);
		}
	},
	setMessages(m: ChatMessage[]) { messages = m; },
	setMode(m: ChatMode) { mode = m; },
	setLoading(v: boolean) { loading = v; },
	setPendingMessage(msg: string | null) { pendingMessage = msg; },
	consumePendingMessage(): string | null { const m = pendingMessage; pendingMessage = null; return m; },
	clearStreaming() { streamingMessages = new Map(); },

	startStreaming(taskId: string) {
		const updated = new Map(streamingMessages);
		updated.set(taskId, {
			taskId,
			content: '',
			toolCalls: [],
			reasoning: [],
			isStreaming: true,
		});
		streamingMessages = updated;
	},

	appendDelta(taskId: string, delta: string) {
		const existing = streamingMessages.get(taskId);
		// Only append to an existing streaming entry — never create orphan entries
		if (!existing) return;
		const updated = new Map(streamingMessages);
		updated.set(taskId, { ...existing, content: existing.content + delta });
		streamingMessages = updated;
	},

	completeMessage(taskId: string, finalText: string) {
		const streaming = streamingMessages.get(taskId);
		const updated = new Map(streamingMessages);
		updated.delete(taskId);
		streamingMessages = updated;

		// Dedup: don't add if message with this taskId already exists
		if (messages.some((m) => m.id === taskId)) return;

		const msg: ChatMessage = {
			id: taskId,
			role: 'assistant',
			content: finalText || streaming?.content || '',
			toolCalls: streaming?.toolCalls ?? [],
			reasoning: streaming?.reasoning ?? [],
			createdAt: new Date().toISOString(),
		};
		messages = [...messages, msg];
	},

	appendToolCall(taskId: string, toolName: string, phase: string, args?: Record<string, unknown>, result?: string, error?: string, durationMs?: number) {
		const updated = new Map(streamingMessages);
		const existing = updated.get(taskId);
		if (!existing) return;
		const tc: ToolCall = { toolName, phase: phase as 'start' | 'result', args, result, error, durationMs };
		updated.set(taskId, { ...existing, toolCalls: [...existing.toolCalls, tc] });
		streamingMessages = updated;
	},

	appendReasoning(taskId: string, round: number, thought: string) {
		const updated = new Map(streamingMessages);
		const existing = updated.get(taskId);
		if (!existing) return;
		const step: ReasoningStep = { round, thought };
		updated.set(taskId, { ...existing, reasoning: [...existing.reasoning, step] });
		streamingMessages = updated;
	},

	addUserMessage(content: string) {
		const msg: ChatMessage = {
			id: crypto.randomUUID(),
			role: 'user',
			content,
			mode: mode,
			createdAt: new Date().toISOString(),
		};
		messages = [...messages, msg];
	},
};
