import { describe, it, expect, beforeEach, vi } from 'vitest';
import { keyboardNav } from './keyboard-nav.svelte';

describe('keyboardNav', () => {
	beforeEach(() => {
		keyboardNav.reset();
	});

	it('starts with focusedIndex -1', () => {
		expect(keyboardNav.focusedIndex).toBe(-1);
	});

	it('moveDown cycles through items', () => {
		keyboardNav.setItemCount(3);
		keyboardNav.moveDown();
		expect(keyboardNav.focusedIndex).toBe(0);
		keyboardNav.moveDown();
		expect(keyboardNav.focusedIndex).toBe(1);
		keyboardNav.moveDown();
		expect(keyboardNav.focusedIndex).toBe(2);
		// Wraps around
		keyboardNav.moveDown();
		expect(keyboardNav.focusedIndex).toBe(0);
	});

	it('moveUp cycles backwards', () => {
		keyboardNav.setItemCount(3);
		keyboardNav.moveUp();
		expect(keyboardNav.focusedIndex).toBe(2);
		keyboardNav.moveUp();
		expect(keyboardNav.focusedIndex).toBe(1);
		keyboardNav.moveUp();
		expect(keyboardNav.focusedIndex).toBe(0);
		keyboardNav.moveUp();
		expect(keyboardNav.focusedIndex).toBe(2);
	});

	it('moveDown is no-op with 0 items', () => {
		keyboardNav.setItemCount(0);
		keyboardNav.moveDown();
		expect(keyboardNav.focusedIndex).toBe(-1);
	});

	it('setItemCount clamps focusedIndex', () => {
		keyboardNav.setItemCount(5);
		keyboardNav.moveDown();
		keyboardNav.moveDown();
		keyboardNav.moveDown(); // index = 2
		keyboardNav.setItemCount(2); // should clamp to 1
		expect(keyboardNav.focusedIndex).toBe(1);
	});

	it('reset clears state', () => {
		keyboardNav.setItemCount(5);
		keyboardNav.moveDown();
		keyboardNav.reset();
		expect(keyboardNav.focusedIndex).toBe(-1);
	});

	it('toggleSelection calls callback with focused index', () => {
		const cb = vi.fn();
		keyboardNav.setItemCount(3);
		keyboardNav.setSelectionCallback(cb);
		keyboardNav.moveDown(); // index 0
		keyboardNav.toggleSelection();
		expect(cb).toHaveBeenCalledWith(0);
	});

	it('toggleSelection is no-op without callback', () => {
		keyboardNav.setItemCount(3);
		keyboardNav.moveDown();
		keyboardNav.toggleSelection(); // should not throw
	});

	it('toggleSelection is no-op when no item focused', () => {
		const cb = vi.fn();
		keyboardNav.setSelectionCallback(cb);
		keyboardNav.toggleSelection();
		expect(cb).not.toHaveBeenCalled();
	});

	it('reset clears selection callback', () => {
		const cb = vi.fn();
		keyboardNav.setSelectionCallback(cb);
		keyboardNav.reset();
		keyboardNav.setItemCount(3);
		keyboardNav.moveDown();
		keyboardNav.toggleSelection();
		expect(cb).not.toHaveBeenCalled();
	});
});
