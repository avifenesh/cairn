// SSE connection store — auto-reconnect with exponential backoff

import { feedStore } from './feed.svelte';
import { chatStore } from './chat.svelte';
import { taskStore } from './tasks.svelte';
import { appStore } from './app.svelte';

let eventSource: EventSource | null = $state(null);
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = $state(0);

const BASE_DELAY = 5000;
const MAX_DELAY = 60000;

function backoffDelay(): number {
	const delay = Math.min(BASE_DELAY * Math.pow(2, attempt), MAX_DELAY);
	return delay + Math.random() * 1000; // jitter
}

function getStreamUrl(): string {
	const token = localStorage.getItem('pub_api_token');
	const base = '/v1/stream';
	return token ? `${base}?token=${encodeURIComponent(token)}` : base;
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
		};

		source.onerror = () => {
			appStore.setSSEConnected(false);
			source.close();
			eventSource = null;
			attempt++;
			const delay = backoffDelay();
			reconnectTimer = setTimeout(() => sseStore.connect(), delay);
		};

		// Ready
		source.addEventListener('ready', (e) => {
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
			const data = JSON.parse(e.data);
			appStore.addNotification('memory', `New memory proposed: ${data.memory?.content?.slice(0, 50)}...`);
		});

		// Agent
		source.addEventListener('agent_progress', (e) => {
			const data = JSON.parse(e.data);
			appStore.setAgentProgress(data.agentId, data.message);
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
