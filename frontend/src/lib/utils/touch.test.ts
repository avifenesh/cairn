import { describe, it, expect, vi } from 'vitest';
import { createPullToRefresh, createSwipeToDismiss } from './touch';

function mockTouchEvent(clientX: number, clientY: number): TouchEvent {
	return {
		touches: [{ clientX, clientY }],
		currentTarget: { scrollTop: 0 },
	} as unknown as TouchEvent;
}

describe('createPullToRefresh', () => {
	it('tracks pull distance', () => {
		const onRefresh = vi.fn(() => Promise.resolve());
		const ptr = createPullToRefresh(onRefresh);

		ptr.handleTouchStart(mockTouchEvent(0, 100));
		expect(ptr.state.pulling).toBe(true);

		ptr.handleTouchMove(mockTouchEvent(0, 200));
		expect(ptr.state.distance).toBeGreaterThan(0);
		expect(ptr.state.distance).toBeLessThanOrEqual(120);
	});

	it('triggers refresh when pulled past threshold', async () => {
		let resolveRefresh: () => void;
		const refreshPromise = new Promise<void>((r) => { resolveRefresh = r; });
		const onRefresh = vi.fn(() => refreshPromise);
		const ptr = createPullToRefresh(onRefresh);

		ptr.handleTouchStart(mockTouchEvent(0, 0));
		ptr.handleTouchMove(mockTouchEvent(0, 200)); // 200 * 0.5 = 100, above 80 threshold
		expect(ptr.state.triggered).toBe(true);

		ptr.handleTouchEnd();
		expect(onRefresh).toHaveBeenCalledOnce();

		resolveRefresh!();
		await refreshPromise;
	});

	it('does not trigger on small pull', () => {
		const onRefresh = vi.fn(() => Promise.resolve());
		const ptr = createPullToRefresh(onRefresh);

		ptr.handleTouchStart(mockTouchEvent(0, 100));
		ptr.handleTouchMove(mockTouchEvent(0, 120)); // 20 * 0.5 = 10, below threshold
		expect(ptr.state.triggered).toBe(false);

		ptr.handleTouchEnd();
		expect(onRefresh).not.toHaveBeenCalled();
		expect(ptr.state.pulling).toBe(false);
	});

	it('ignores pull-up (negative distance)', () => {
		const onRefresh = vi.fn(() => Promise.resolve());
		const ptr = createPullToRefresh(onRefresh);

		ptr.handleTouchStart(mockTouchEvent(0, 100));
		ptr.handleTouchMove(mockTouchEvent(0, 50)); // pulling up
		expect(ptr.state.distance).toBe(0);
	});

	it('does not start if scrollTop > 0', () => {
		const onRefresh = vi.fn(() => Promise.resolve());
		const ptr = createPullToRefresh(onRefresh);

		const event = {
			touches: [{ clientX: 0, clientY: 100 }],
			currentTarget: { scrollTop: 50 },
		} as unknown as TouchEvent;

		ptr.handleTouchStart(event);
		expect(ptr.state.pulling).toBe(false);
	});

	it('reset clears state', () => {
		const onRefresh = vi.fn(() => Promise.resolve());
		const ptr = createPullToRefresh(onRefresh);

		ptr.handleTouchStart(mockTouchEvent(0, 0));
		ptr.handleTouchMove(mockTouchEvent(0, 100));
		ptr.reset();
		expect(ptr.state.pulling).toBe(false);
		expect(ptr.state.distance).toBe(0);
	});
});

describe('createSwipeToDismiss', () => {
	it('tracks horizontal swipe', () => {
		const onDismiss = vi.fn();
		const swipe = createSwipeToDismiss(onDismiss);

		swipe.handleTouchStart(mockTouchEvent(100, 100));
		swipe.handleTouchMove(mockTouchEvent(150, 102)); // horizontal > vertical
		expect(swipe.state.swiping).toBe(true);
		expect(swipe.state.offsetX).toBe(50);
	});

	it('dismisses on large swipe', () => {
		const onDismiss = vi.fn();
		const swipe = createSwipeToDismiss(onDismiss);

		swipe.handleTouchStart(mockTouchEvent(100, 100));
		swipe.handleTouchMove(mockTouchEvent(250, 100)); // 150px > 100 threshold
		expect(swipe.state.dismissed).toBe(true);

		swipe.handleTouchEnd();
		expect(onDismiss).toHaveBeenCalledOnce();
	});

	it('does not dismiss on small swipe', () => {
		const onDismiss = vi.fn();
		const swipe = createSwipeToDismiss(onDismiss);

		swipe.handleTouchStart(mockTouchEvent(100, 100));
		swipe.handleTouchMove(mockTouchEvent(140, 102)); // 40px < 100 threshold
		expect(swipe.state.dismissed).toBe(false);

		swipe.handleTouchEnd();
		expect(onDismiss).not.toHaveBeenCalled();
	});

	it('ignores vertical swipe', () => {
		const onDismiss = vi.fn();
		const swipe = createSwipeToDismiss(onDismiss);

		swipe.handleTouchStart(mockTouchEvent(100, 100));
		swipe.handleTouchMove(mockTouchEvent(102, 200)); // vertical > horizontal
		expect(swipe.state.swiping).toBe(false);
	});

	it('resets state after touch end', () => {
		const onDismiss = vi.fn();
		const swipe = createSwipeToDismiss(onDismiss);

		swipe.handleTouchStart(mockTouchEvent(100, 100));
		swipe.handleTouchMove(mockTouchEvent(180, 100));
		swipe.handleTouchEnd();
		expect(swipe.state.swiping).toBe(false);
		expect(swipe.state.offsetX).toBe(0);
	});
});
