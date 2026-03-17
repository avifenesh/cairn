import { describe, it, expect, beforeEach } from 'vitest';
import { memoryStore } from './memory.svelte';
import type { Memory } from '$lib/types';

function makeMemory(overrides: Partial<Memory> = {}): Memory {
	return {
		id: 'm1',
		category: 'general',
		status: 'accepted',
		content: 'Test memory',
		createdAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

describe('memoryStore', () => {
	beforeEach(() => {
		memoryStore.setMemories([]);
		memoryStore.setSearchResults([]);
		memoryStore.setSearchQuery('');
	});

	it('starts empty', () => {
		expect(memoryStore.memories).toEqual([]);
		expect(memoryStore.proposedCount).toBe(0);
	});

	it('proposedCount counts proposed memories', () => {
		memoryStore.setMemories([
			makeMemory({ id: 'm1', status: 'proposed' }),
			makeMemory({ id: 'm2', status: 'accepted' }),
			makeMemory({ id: 'm3', status: 'proposed' }),
		]);
		expect(memoryStore.proposedCount).toBe(2);
	});

	it('resolveMemory changes status', () => {
		memoryStore.setMemories([makeMemory({ id: 'm1', status: 'proposed' })]);
		memoryStore.resolveMemory('m1', 'accepted');
		expect(memoryStore.memories[0].status).toBe('accepted');
	});

	it('resolveMemory leaves other memories unchanged', () => {
		memoryStore.setMemories([
			makeMemory({ id: 'm1', status: 'proposed' }),
			makeMemory({ id: 'm2', status: 'proposed' }),
		]);
		memoryStore.resolveMemory('m1', 'rejected');
		expect(memoryStore.memories[0].status).toBe('rejected');
		expect(memoryStore.memories[1].status).toBe('proposed');
	});
});
