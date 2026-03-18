import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// We can't import sseStore directly because it creates an EventSource on connect().
// Instead, test the pure logic: backoff calculation, safeParse, and connection lifecycle
// via a mock EventSource.

// Mock dependent stores before importing
vi.mock('./feed.svelte', () => ({
	feedStore: { addItem: vi.fn() },
}));
vi.mock('./chat.svelte', () => ({
	chatStore: {
		appendDelta: vi.fn(),
		completeMessage: vi.fn(),
		appendReasoning: vi.fn(),
		appendToolCall: vi.fn(),
	},
}));
vi.mock('./tasks.svelte', () => ({
	taskStore: { upsertTask: vi.fn(), addApproval: vi.fn() },
}));
vi.mock('./app.svelte', () => ({
	appStore: {
		setSSEConnected: vi.fn(),
		setClientId: vi.fn(),
		setPollStatus: vi.fn(),
		addNotification: vi.fn(),
		setAgentProgress: vi.fn(),
	},
}));
vi.mock('./offline-queue.svelte', () => ({
	offlineQueue: { drain: vi.fn(() => Promise.resolve({ succeeded: 0, failed: 0 })) },
}));

// Mock EventSource
let mockEventSource: MockEventSource;
let eventSourceListeners: Record<string, ((e: any) => void)[]>;

class MockEventSource {
	static OPEN = 1;
	static CONNECTING = 0;
	static CLOSED = 2;
	OPEN = 1;
	CONNECTING = 0;
	CLOSED = 2;

	url: string;
	readyState = 0;
	withCredentials: boolean;
	onopen: ((e: Event) => void) | null = null;
	onerror: ((e: Event) => void) | null = null;
	onmessage: ((e: MessageEvent) => void) | null = null;

	constructor(url: string, opts?: { withCredentials?: boolean }) {
		this.url = url;
		this.withCredentials = opts?.withCredentials ?? false;
		mockEventSource = this;
		eventSourceListeners = {};
	}

	addEventListener(name: string, fn: (e: any) => void) {
		if (!eventSourceListeners[name]) eventSourceListeners[name] = [];
		eventSourceListeners[name].push(fn);
	}

	close() {
		this.readyState = 2;
	}

	// Test helpers
	simulateOpen() {
		this.readyState = 1;
		this.onopen?.(new Event('open'));
	}

	simulateError() {
		this.onerror?.(new Event('error'));
	}

	simulateEvent(name: string, data: string, lastEventId = '') {
		const handlers = eventSourceListeners[name] ?? [];
		for (const fn of handlers) {
			fn({ data, lastEventId });
		}
	}
}

(globalThis as any).EventSource = MockEventSource;

describe('sseStore', () => {
	let sseStore: typeof import('./sse.svelte').sseStore;
	let appStore: typeof import('./app.svelte').appStore;
	let feedStore: typeof import('./feed.svelte').feedStore;
	let chatStore: typeof import('./chat.svelte').chatStore;
	let taskStore: typeof import('./tasks.svelte').taskStore;
	let offlineQueue: typeof import('./offline-queue.svelte').offlineQueue;

	beforeEach(async () => {
		vi.useFakeTimers();
		vi.clearAllMocks();
		vi.spyOn(Math, 'random').mockReturnValue(0.5);

		// Dynamic import to get fresh mocked modules
		const sseMod = await import('./sse.svelte');
		sseStore = sseMod.sseStore;
		appStore = (await import('./app.svelte')).appStore;
		feedStore = (await import('./feed.svelte')).feedStore;
		chatStore = (await import('./chat.svelte')).chatStore;
		taskStore = (await import('./tasks.svelte')).taskStore;
		offlineQueue = (await import('./offline-queue.svelte')).offlineQueue;
	});

	afterEach(() => {
		sseStore.disconnect();
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	it('starts disconnected', () => {
		expect(sseStore.connected).toBe(false);
		expect(sseStore.reconnecting).toBe(false);
	});

	it('connect creates EventSource and sets connected on open', () => {
		sseStore.connect();
		expect(mockEventSource).toBeDefined();
		expect(mockEventSource.withCredentials).toBe(true);

		mockEventSource.simulateOpen();
		expect(appStore.setSSEConnected).toHaveBeenCalledWith(true);
	});

	it('drains offline queue on successful connection', async () => {
		sseStore.connect();
		mockEventSource.simulateOpen();

		// Let the drain promise resolve
		await vi.advanceTimersByTimeAsync(0);
		expect(offlineQueue.drain).toHaveBeenCalled();
	});

	it('reconnects with backoff on error', () => {
		sseStore.connect();
		mockEventSource.simulateError();

		expect(appStore.setSSEConnected).toHaveBeenCalledWith(false);
		expect(sseStore.attempt).toBe(1);

		// Should schedule reconnect (base 5s + jitter)
		vi.advanceTimersByTime(6500);
		// A new EventSource should have been created for reconnect
		expect(mockEventSource.url).toContain('/v1/stream');
	});

	it('resets attempt counter on successful reconnect', () => {
		sseStore.connect();
		mockEventSource.simulateError();
		expect(sseStore.attempt).toBe(1);

		vi.advanceTimersByTime(6500);
		mockEventSource.simulateOpen();
		expect(sseStore.attempt).toBe(0);
	});

	it('disconnect clears everything', () => {
		sseStore.connect();
		mockEventSource.simulateOpen();
		sseStore.disconnect();

		expect(mockEventSource.readyState).toBe(2); // CLOSED
		expect(appStore.setSSEConnected).toHaveBeenCalledWith(false);
		expect(sseStore.attempt).toBe(0);
	});

	it('routes ready event to appStore.setClientId', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('ready', '{"clientId":"abc-123"}');
		expect(appStore.setClientId).toHaveBeenCalledWith('abc-123');
	});

	it('routes feed_update to feedStore.addItem', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('feed_update', '{"item":{"id":1,"title":"test"}}');
		expect(feedStore.addItem).toHaveBeenCalledWith({ id: 1, title: 'test' });
	});

	it('routes task_update to taskStore.upsertTask', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('task_update', '{"task":{"id":"t1","status":"running"}}');
		expect(taskStore.upsertTask).toHaveBeenCalledWith({ id: 't1', status: 'running' });
	});

	it('routes assistant_delta to chatStore.appendDelta', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('assistant_delta', '{"taskId":"t1","deltaText":"hello"}');
		expect(chatStore.appendDelta).toHaveBeenCalledWith('t1', 'hello');
	});

	it('routes assistant_end to chatStore.completeMessage', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('assistant_end', '{"taskId":"t1","messageText":"done"}');
		expect(chatStore.completeMessage).toHaveBeenCalledWith('t1', 'done');
	});

	it('ignores invalid JSON gracefully', () => {
		const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
		sseStore.connect();
		mockEventSource.simulateEvent('feed_update', 'not-json{{{');
		expect(feedStore.addItem).not.toHaveBeenCalled();
		expect(warnSpy).toHaveBeenCalled();
		expect(warnSpy.mock.calls[0][0]).toContain('Failed to parse');
		warnSpy.mockRestore();
	});

	it('ignores array payloads (must be object)', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('feed_update', '[1,2,3]');
		expect(feedStore.addItem).not.toHaveBeenCalled();
	});

	it('ignores null payloads', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('feed_update', 'null');
		expect(feedStore.addItem).not.toHaveBeenCalled();
	});

	it('tracks lastEventId from events', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('ready', '{"clientId":"x"}', 'evt-42');

		// Disconnect and reconnect - URL should include lastEventId
		sseStore.disconnect();
		sseStore.connect();
		expect(mockEventSource.url).toContain('lastEventId=evt-42');
	});

	it('includes auth token in URL when set', () => {
		localStorage.setItem('pub_api_token', 'test-token');
		sseStore.connect();
		expect(mockEventSource.url).toContain('token=test-token');
		localStorage.removeItem('pub_api_token');
	});

	it('reports reconnecting during backoff window', () => {
		sseStore.connect();
		mockEventSource.simulateError();
		expect(sseStore.reconnecting).toBe(true);
		sseStore.disconnect();
		expect(sseStore.reconnecting).toBe(false);
	});

	it('routes poll_completed to appStore.setPollStatus', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('poll_completed', '{"source":"github","newCount":5}');
		expect(appStore.setPollStatus).toHaveBeenCalledWith('github', 5);
	});

	it('routes approval_required to taskStore.addApproval', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('approval_required', '{"approval":{"id":"a1"}}');
		expect(taskStore.addApproval).toHaveBeenCalledWith({ id: 'a1' });
	});

	it('routes assistant_reasoning to chatStore.appendReasoning', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('assistant_reasoning', '{"taskId":"t1","round":1,"thought":"thinking"}');
		expect(chatStore.appendReasoning).toHaveBeenCalledWith('t1', 1, 'thinking');
	});

	it('routes assistant_tool_call to chatStore.appendToolCall', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('assistant_tool_call', '{"taskId":"t1","toolName":"shell","phase":"start"}');
		expect(chatStore.appendToolCall).toHaveBeenCalledWith('t1', 'shell', 'start', undefined, undefined);
	});

	it('routes agent_progress to appStore.setAgentProgress', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('agent_progress', '{"agentId":"a1","message":"working"}');
		expect(appStore.setAgentProgress).toHaveBeenCalledWith('a1', 'working');
	});

	it('routes skill_activated to appStore.addNotification', () => {
		sseStore.connect();
		mockEventSource.simulateEvent('skill_activated', '{"skillName":"web-search"}');
		expect(appStore.addNotification).toHaveBeenCalledWith('skill', 'Skill activated: web-search');
	});
});
