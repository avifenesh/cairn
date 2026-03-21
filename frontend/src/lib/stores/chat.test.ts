import { describe, it, expect, beforeEach } from 'vitest';
import { chatStore } from './chat.svelte';

describe('chatStore', () => {
	beforeEach(() => {
		chatStore.setMessages([]);
		chatStore.setSessions([]);
		chatStore.setCurrentSession(null);
		chatStore.setMode('talk');
		chatStore.clearStreaming();
	});

	describe('messages', () => {
		it('starts empty', () => {
			expect(chatStore.messages).toEqual([]);
		});

		it('addUserMessage appends a user message', () => {
			chatStore.addUserMessage('hello');
			expect(chatStore.messages).toHaveLength(1);
			expect(chatStore.messages[0].role).toBe('user');
			expect(chatStore.messages[0].content).toBe('hello');
			expect(chatStore.messages[0].mode).toBe('talk');
		});

		it('addUserMessage uses current mode', () => {
			chatStore.setMode('coding');
			chatStore.addUserMessage('fix bug');
			expect(chatStore.messages[0].mode).toBe('coding');
		});
	});

	describe('streaming', () => {
		it('startStreaming creates a streaming entry', () => {
			chatStore.startStreaming('task-1');
			expect(chatStore.streamingMessages.size).toBe(1);
			const sm = chatStore.streamingMessages.get('task-1');
			expect(sm?.content).toBe('');
			expect(sm?.isStreaming).toBe(true);
		});

		it('appendDelta accumulates text', () => {
			chatStore.startStreaming('task-1');
			chatStore.appendDelta('task-1', 'Hello');
			chatStore.appendDelta('task-1', ' world');
			expect(chatStore.streamingMessages.get('task-1')?.content).toBe('Hello world');
		});

		it('appendDelta ignores delta if streaming not started', () => {
			chatStore.appendDelta('task-new', 'surprise');
			expect(chatStore.streamingMessages.get('task-new')).toBeUndefined();
		});

		it('completeMessage moves streaming to committed messages', () => {
			chatStore.startStreaming('task-1');
			chatStore.appendDelta('task-1', 'partial');
			chatStore.completeMessage('task-1', 'final text');

			expect(chatStore.streamingMessages.size).toBe(0);
			expect(chatStore.messages).toHaveLength(1);
			expect(chatStore.messages[0].content).toBe('final text');
			expect(chatStore.messages[0].role).toBe('assistant');
			expect(chatStore.messages[0].id).toBe('task-1');
		});

		it('completeMessage preserves tool calls from streaming', () => {
			chatStore.startStreaming('task-1');
			chatStore.appendToolCall('task-1', 'readFile', 'start', { path: '/foo' });
			chatStore.appendToolCall('task-1', 'readFile', 'result', undefined, 'file contents');
			chatStore.completeMessage('task-1', 'done');

			expect(chatStore.messages[0].toolCalls).toHaveLength(2);
			expect(chatStore.messages[0].toolCalls![0].toolName).toBe('readFile');
			expect(chatStore.messages[0].toolCalls![1].phase).toBe('result');
		});

		it('completeMessage preserves reasoning from streaming', () => {
			chatStore.startStreaming('task-1');
			chatStore.appendReasoning('task-1', 1, 'thinking...');
			chatStore.appendReasoning('task-1', 2, 'decided');
			chatStore.completeMessage('task-1', 'answer');

			expect(chatStore.messages[0].reasoning).toHaveLength(2);
			expect(chatStore.messages[0].reasoning![0].thought).toBe('thinking...');
		});

		it('activeStream returns the streaming entry', () => {
			expect(chatStore.activeStream).toBeNull();
			chatStore.startStreaming('task-1');
			expect(chatStore.activeStream?.taskId).toBe('task-1');
		});
	});

	describe('tool calls on non-existent stream', () => {
		it('appendToolCall is a no-op for unknown taskId', () => {
			chatStore.appendToolCall('ghost', 'tool', 'start');
			expect(chatStore.streamingMessages.size).toBe(0);
		});

		it('appendReasoning is a no-op for unknown taskId', () => {
			chatStore.appendReasoning('ghost', 1, 'thought');
			expect(chatStore.streamingMessages.size).toBe(0);
		});
	});

	describe('sessions', () => {
		it('setSessions and currentSession', () => {
			chatStore.setSessions([
				{ id: 's1', messageCount: 5, updatedAt: '2026-01-01', createdAt: '2026-01-01' },
			]);
			expect(chatStore.sessions).toHaveLength(1);
			chatStore.setCurrentSession('s1');
			expect(chatStore.currentSessionId).toBe('s1');
		});
	});
});
