// Offline queue - write actions queued when SSE disconnected
// Plan: 50 cap, 10min expiry, drain on reconnect

interface QueuedAction {
	id: string;
	fn: () => Promise<unknown>;
	createdAt: number;
}

const MAX_QUEUE = 50;
const EXPIRY_MS = 10 * 60 * 1000; // 10 minutes

let queue = $state<QueuedAction[]>([]);
let draining = $state(false);

function prune() {
	const now = Date.now();
	queue = queue.filter((a) => now - a.createdAt < EXPIRY_MS);
}

export const offlineQueue = {
	get length() { return queue.length; },
	get draining() { return draining; },

	enqueue(fn: () => Promise<unknown>): boolean {
		prune();
		if (queue.length >= MAX_QUEUE) return false;
		queue = [...queue, { id: crypto.randomUUID(), fn, createdAt: Date.now() }];
		return true;
	},

	async drain(): Promise<{ succeeded: number; failed: number }> {
		prune();
		if (queue.length === 0 || draining) return { succeeded: 0, failed: 0 };
		draining = true;
		let succeeded = 0;
		let failed = 0;
		const pending = [...queue];
		queue = [];
		for (const action of pending) {
			try {
				await action.fn();
				succeeded++;
			} catch (err) {
				failed++;
				console.warn('[offline-queue] Action %s failed:', action.id, err);
			}
		}
		draining = false;
		return { succeeded, failed };
	},

	clear() {
		queue = [];
	},
};
