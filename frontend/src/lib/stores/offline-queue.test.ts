import { describe, it, expect, vi, beforeEach } from 'vitest';
import { offlineQueue } from './offline-queue.svelte';

describe('offlineQueue', () => {
	beforeEach(() => {
		offlineQueue.clear();
	});

	it('starts empty', () => {
		expect(offlineQueue.length).toBe(0);
	});

	it('enqueue adds an action', () => {
		offlineQueue.enqueue(() => Promise.resolve());
		expect(offlineQueue.length).toBe(1);
	});

	it('enforces 50-item cap', () => {
		for (let i = 0; i < 50; i++) {
			expect(offlineQueue.enqueue(() => Promise.resolve())).toBe(true);
		}
		expect(offlineQueue.enqueue(() => Promise.resolve())).toBe(false);
		expect(offlineQueue.length).toBe(50);
	});

	it('drain executes all actions', async () => {
		const fn1 = vi.fn(() => Promise.resolve());
		const fn2 = vi.fn(() => Promise.resolve());
		offlineQueue.enqueue(fn1);
		offlineQueue.enqueue(fn2);
		const result = await offlineQueue.drain();
		expect(fn1).toHaveBeenCalledOnce();
		expect(fn2).toHaveBeenCalledOnce();
		expect(result).toEqual({ succeeded: 2, failed: 0 });
		expect(offlineQueue.length).toBe(0);
	});

	it('drain reports failures', async () => {
		offlineQueue.enqueue(() => Promise.resolve());
		offlineQueue.enqueue(() => Promise.reject(new Error('fail')));
		const result = await offlineQueue.drain();
		expect(result).toEqual({ succeeded: 1, failed: 1 });
	});

	it('drain is no-op when empty', async () => {
		const result = await offlineQueue.drain();
		expect(result).toEqual({ succeeded: 0, failed: 0 });
	});

	it('clear empties the queue', () => {
		offlineQueue.enqueue(() => Promise.resolve());
		offlineQueue.enqueue(() => Promise.resolve());
		offlineQueue.clear();
		expect(offlineQueue.length).toBe(0);
	});
});
