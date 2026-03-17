// SSE connection store — auto-reconnect with exponential backoff
// Plan 10.2: Last-Event-ID reconnection replay, backoff base 5s max 60s + jitter

import { feedStore } from './feed.svelte';
import { chatStore } from './chat.svelte';
import { taskStore } from './tasks.svelte';
import { appStore } from './app.svelte';
import { offlineQueue } from './offline-queue.svelte';

let eventSource: EventSource | null = $state(null);
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = $state(0);
let lastEventId: string | null = null;

const BASE_DELAY = 5000;
const MAX_DELAY = 60000;

function backoffDelay(): number {
	const delay = Math.min(BASE_DELAY * Math.pow(2, attempt), MAX_DELAY);
	return delay + Math.random() * 1000; // jitter
}

function getStreamUrl(): string {
	const token = localStorage.getItem('pub_api_token');
	const base = '/v1/stream';
	const params = new URLSearchParams();
	if (token) params.set('token', token);
	if (lastEventId) params.set('lastEventId', lastEventId);
	const qs = params.toString();
	return qs ? `${base}?${qs}` : base;
}

export const sseStore = {
	get connected() {
		return eventSource?.readyState === EventSource.OPEN;
	},
	get reconnecting() {
		return eventSource?.readyState === EventSource.CONNECTING || attempt > 0;
	},
	get attempt() {
		return attempt;
	},

	connect() {
		if (eventSource) {
			eventSource.close();
		}

		const source = new EventSource(getStreamUrl(), { withCredentials: true });
		eventSource = source;

		source.onopen = () => {
			attempt = 0;
			appStore.setSSEConnected(true);
			// Drain offline queue on reconnect
			offlineQueue.drain().then(({ succeeded, failed }) => {
				if (succeeded > 0) {
					appStore.addNotification('queue', `Synced ${succeeded} queued action${succeeded > 1 ? 's' : ''}`);
				}
				if (failed > 0) {
					appStore.addNotification('error', `${failed} queued action${failed > 1 ? 's' : ''} failed`);
				}
			});
		};

		source.onerror = () => {
			appStore.setSSEConnected(false);
			source.close();
			eventSource = null;
			attempt++;
			const delay = backoffDelay();
			reconnectTimer = setTimeout(() => sseStore.connect(), delay);
		};

		// Track Last-Event-ID for reconnection replay
		source.addEventListener('message', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
		});

		// Ready
		source.addEventListener('ready', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
			const data = JSON.parse(e.data);
			appStore.setClientId(data.clientId);
		});

		// Feed
		source.addEventListener('feed_update', (e) => {
			const data = JSON.parse(e.data);
			feedStore.addItem(data.item ?? data);
		});

		source.addEventListener('poll_completed', (e) => {
			const data = JSON.parse(e.data);
			appStore.setPollStatus(data.source, data.newCount);
		});

		// Tasks & Approvals
		source.addEventListener('task_update', (e) => {
			const data = JSON.parse(e.data);
			taskStore.upsertTask(data.task ?? data);
		});

		source.addEventListener('approval_required', (e) => {
			const data = JSON.parse(e.data);
			taskStore.addApproval(data.approval ?? data);
		});

		// Chat streaming
		source.addEventListener('assistant_delta', (e) => {
			const data = JSON.parse(e.data);
			chatStore.appendDelta(data.taskId, data.deltaText);
		});

		source.addEventListener('assistant_end', (e) => {
			const data = JSON.parse(e.data);
			chatStore.completeMessage(data.taskId, data.messageText);
		});

		source.addEventListener('assistant_reasoning', (e) => {
			const data = JSON.parse(e.data);
			chatStore.appendReasoning(data.taskId, data.round, data.thought);
		});

		source.addEventListener('assistant_tool_call', (e) => {
			const data = JSON.parse(e.data);
			chatStore.appendToolCall(data.taskId, data.toolName, data.phase, data.args, data.result);
		});

		// Memory
		source.addEventListener('memory_proposed', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
			const data = JSON.parse(e.data);
			appStore.addNotification('memory', `New memory proposed: ${data.memory?.content?.slice(0, 50)}...`);
		});

		source.addEventListener('memory_accepted', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
		});

		// Soul
		source.addEventListener('soul_updated', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
			const data = JSON.parse(e.data);
			appStore.addNotification('soul', `SOUL.md updated (${data.sha?.slice(0, 7)})`);
		});

		// Digest
		source.addEventListener('digest_ready', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
			appStore.addNotification('digest', 'New digest available');
		});

		// Coding sessions
		source.addEventListener('coding_session_event', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
		});

		// Agent
		source.addEventListener('agent_progress', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
			const data = JSON.parse(e.data);
			appStore.setAgentProgress(data.agentId, data.message);
		});

		// Skills
		source.addEventListener('skill_activated', (e) => {
			if (e.lastEventId) lastEventId = e.lastEventId;
			const data = JSON.parse(e.data);
			appStore.addNotification('skill', `Skill activated: ${data.skillName}`);
		});
	},

	disconnect() {
		if (reconnectTimer) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		if (eventSource) {
			eventSource.close();
			eventSource = null;
		}
		attempt = 0;
		appStore.setSSEConnected(false);
	},
};
