// Chat store — sessions, messages, streaming state

import type { ChatMessage, ChatSession, ChatMode, ToolCall, ReasoningStep } from '$lib/types';

interface StreamingMessage {
	taskId: string;
	content: string;
	toolCalls: ToolCall[];
	reasoning: ReasoningStep[];
	isStreaming: boolean;
}

let sessions = $state<ChatSession[]>([]);
let currentSessionId = $state<string | null>(null);
let messages = $state<ChatMessage[]>([]);
let streamingMessages = $state<Map<string, StreamingMessage>>(new Map());
let mode = $state<ChatMode>('talk');
let loading = $state(false);

export const chatStore = {
	get sessions() { return sessions; },
	get currentSessionId() { return currentSessionId; },
	get messages() { return messages; },
	get streamingMessages() { return streamingMessages; },
	get mode() { return mode; },
	get loading() { return loading; },
	get activeStream() {
		for (const sm of streamingMessages.values()) {
			if (sm.isStreaming) return sm;
		}
		return null;
	},

	setSessions(s: ChatSession[]) { sessions = s; },
	setCurrentSession(id: string | null) { currentSessionId = id; },
	setMessages(m: ChatMessage[]) { messages = m; },
	setMode(m: ChatMode) { mode = m; },
	setLoading(v: boolean) { loading = v; },
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
		const updated = new Map(streamingMessages);
		const existing = updated.get(taskId);
		if (existing) {
			updated.set(taskId, { ...existing, content: existing.content + delta });
		} else {
			updated.set(taskId, {
				taskId,
				content: delta,
				toolCalls: [],
				reasoning: [],
				isStreaming: true,
			});
		}
		streamingMessages = updated;
	},

	completeMessage(taskId: string, finalText: string) {
		const streaming = streamingMessages.get(taskId);
		const updated = new Map(streamingMessages);
		updated.delete(taskId);
		streamingMessages = updated;

		const msg: ChatMessage = {
			id: taskId,
			role: 'assistant',
			content: finalText,
			toolCalls: streaming?.toolCalls ?? [],
			reasoning: streaming?.reasoning ?? [],
			createdAt: new Date().toISOString(),
		};
		messages = [...messages, msg];
	},

	appendToolCall(taskId: string, toolName: string, phase: string, args?: Record<string, unknown>, result?: string) {
		const updated = new Map(streamingMessages);
		const existing = updated.get(taskId);
		if (!existing) return;
		const tc: ToolCall = { toolName, phase: phase as 'start' | 'result', args, result };
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
