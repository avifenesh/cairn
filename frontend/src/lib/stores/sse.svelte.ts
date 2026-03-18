// SSE connection store — auto-reconnect with exponential backoff
// Plan 10.2: Last-Event-ID reconnection replay, backoff base 5s max 60s + jitter

import { feedStore } from './feed.svelte';
import { chatStore } from './chat.svelte';
import { taskStore } from './tasks.svelte';
import { appStore } from './app.svelte';
import { skillStore } from './skills.svelte';
import { statusStore } from './status.svelte';
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

function safeParse(eventName: string, data: string): unknown | null {
	try {
		return JSON.parse(data);
	} catch {
		console.warn('[sse] Failed to parse %s event data:', eventName, data);
		return null;
	}
}

function getStreamUrl(): string {
	const token = localStorage.getItem('cairn_api_token');
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

		// Helper: track event ID + safe parse in one step
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		function handle(name: string, source: EventSource, fn: (data: Record<string, any>) => void) {
			source.addEventListener(name, (e) => {
				if (e.lastEventId) lastEventId = e.lastEventId;
				const parsed = safeParse(name, e.data);
				if (parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)) {
					fn(parsed as Record<string, any>);
				}
			});
		}

		handle('ready', source, (d) => appStore.setClientId(d.clientId));

		// Feed
		handle('feed_update', source, (d) => feedStore.addItem(d.item ?? d));
		handle('poll_completed', source, (d) => appStore.setPollStatus(d.source, d.newCount));

		// Tasks & Approvals
		handle('task_update', source, (d) => taskStore.upsertTask(d.task ?? d));
		handle('approval_required', source, (d) => taskStore.addApproval(d.approval ?? d));

		// Chat streaming — batch deltas with requestAnimationFrame for performance
		let pendingDelta = '';
		let pendingDeltaTaskId = '';
		let deltaFlushHandle: number | null = null;
		const completedTaskIds = new Set<string>();

		function flushDelta() {
			deltaFlushHandle = null;
			if (pendingDelta && pendingDeltaTaskId && !completedTaskIds.has(pendingDeltaTaskId)) {
				chatStore.appendDelta(pendingDeltaTaskId, pendingDelta);
				pendingDelta = '';
			}
		}

		handle('assistant_delta', source, (d) => {
			// Ignore deltas for already-completed tasks
			if (completedTaskIds.has(d.taskId)) return;
			if (pendingDeltaTaskId && pendingDeltaTaskId !== d.taskId) {
				flushDelta();
			}
			pendingDeltaTaskId = d.taskId;
			pendingDelta += d.deltaText;
			if (deltaFlushHandle === null) {
				deltaFlushHandle = requestAnimationFrame(flushDelta);
			}
		});
		handle('assistant_end', source, (d) => {
			// Resolve taskId: from event, from pending buffer, or from active stream
			let taskId = d.taskId;
			if (!taskId) taskId = pendingDeltaTaskId;
			if (!taskId) {
				const active = chatStore.activeStream;
				if (active) taskId = active.taskId;
			}
			if (!taskId) return;

			// Mark as completed BEFORE any async work — prevents RAF race
			completedTaskIds.add(taskId);

			// Build final content: store content + any unflushed buffer
			const streaming = chatStore.streamingMessages.get(taskId);
			const fullContent = (streaming?.content ?? '') + pendingDelta;

			// Clear the pending buffer (don't flush to store — we're completing directly)
			if (deltaFlushHandle !== null) {
				cancelAnimationFrame(deltaFlushHandle);
				deltaFlushHandle = null;
			}
			pendingDelta = '';
			pendingDeltaTaskId = '';

			// Complete: prefer explicit messageText from backend, else accumulated content
			const text = d.messageText ?? d.text ?? fullContent;
			chatStore.completeMessage(taskId, text);
		});
		handle('assistant_reasoning', source, (d) => chatStore.appendReasoning(d.taskId, d.round, d.thought));
		handle('assistant_tool_call', source, (d) => chatStore.appendToolCall(d.taskId, d.toolName, d.phase, d.args, d.result, d.error, d.durationMs));

		// Memory
		handle('memory_proposed', source, (d) => appStore.addNotification('memory', `New memory proposed: ${d.memory?.content?.slice(0, 50)}...`));
		handle('memory_accepted', source, () => {});

		// Soul
		handle('soul_updated', source, (d) => appStore.addNotification('soul', `SOUL.md updated (${d.sha?.slice(0, 7)})`));

		// Digest
		handle('digest_ready', source, () => appStore.addNotification('digest', 'New digest available'));

		// Coding sessions
		handle('coding_session_event', source, () => {});

		// Agent
		handle('agent_progress', source, (d) => appStore.setAgentProgress(d.agentId, d.message));

		// Skills
		handle('skill_activated', source, (d) => {
			skillStore.activateSkill(d.skillName);
			appStore.addNotification('skill', `Skill activated: ${d.skillName}`);
		});

		// Budget
		handle('budget_update', source, (d) => statusStore.setBudget(d as Record<string, number>));
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
