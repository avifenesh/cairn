// Touch gesture utilities for mobile: pull-to-refresh, swipe-to-dismiss
// Plan 10.11: pull-to-refresh (mobile touch gesture), swipe-to-dismiss (feed cards)
// Uses $state for Svelte 5 reactivity so components can derive from state properties.

export interface PullToRefreshState {
	pulling: boolean;
	distance: number;
	triggered: boolean;
	refreshing: boolean;
}

const PULL_THRESHOLD = 80;
const PULL_DAMPING = 0.5;
const PULL_MAX_DISTANCE = 120;
const DIRECTION_LOCK_THRESHOLD = 5;

export function createPullToRefresh(
	onRefresh: () => Promise<void>,
): {
	handleTouchStart: (e: TouchEvent) => void;
	handleTouchMove: (e: TouchEvent) => void;
	handleTouchEnd: () => void;
	handleTouchCancel: () => void;
	state: PullToRefreshState;
	reset: () => void;
} {
	let state = $state<PullToRefreshState>({ pulling: false, distance: 0, triggered: false, refreshing: false });
	let startY = 0;
	let startX = 0;
	let directionLocked = false;
	let isVertical = false;

	function handleTouchStart(e: TouchEvent) {
		if (state.refreshing) return;
		const el = e.currentTarget as HTMLElement;
		if (el.scrollTop > 0) return;
		startY = e.touches[0].clientY;
		startX = e.touches[0].clientX;
		state.pulling = true;
		state.distance = 0;
		state.triggered = false;
		directionLocked = false;
		isVertical = false;
	}

	function handleTouchMove(e: TouchEvent) {
		if (!state.pulling || state.refreshing) return;
		const dy = e.touches[0].clientY - startY;
		const dx = e.touches[0].clientX - startX;

		// Lock direction on first significant movement - ignore horizontal gestures
		if (!directionLocked && (Math.abs(dx) > DIRECTION_LOCK_THRESHOLD || Math.abs(dy) > DIRECTION_LOCK_THRESHOLD)) {
			directionLocked = true;
			isVertical = Math.abs(dy) > Math.abs(dx);
			if (!isVertical) {
				state.pulling = false;
				state.distance = 0;
				return;
			}
		}

		if (!isVertical && directionLocked) return;

		if (dy < 0) {
			state.distance = 0;
			return;
		}
		state.distance = Math.min(dy * PULL_DAMPING, PULL_MAX_DISTANCE);
		state.triggered = state.distance >= PULL_THRESHOLD;
	}

	function handleTouchEnd() {
		if (!state.pulling) return;
		if (state.triggered && !state.refreshing) {
			state.refreshing = true;
			// Keep triggered true during refresh so spinner stays visible
			onRefresh().finally(() => {
				state.refreshing = false;
				state.pulling = false;
				state.distance = 0;
				state.triggered = false;
			});
		} else {
			state.pulling = false;
			state.distance = 0;
			state.triggered = false;
		}
	}

	function handleTouchCancel() {
		if (state.refreshing) return;
		state.pulling = false;
		state.distance = 0;
		state.triggered = false;
	}

	function reset() {
		state.pulling = false;
		state.distance = 0;
		state.triggered = false;
		state.refreshing = false;
	}

	return { handleTouchStart, handleTouchMove, handleTouchEnd, handleTouchCancel, state, reset };
}

export interface SwipeState {
	swiping: boolean;
	offsetX: number;
	dismissed: boolean;
}

export const SWIPE_THRESHOLD = 100;

export function createSwipeToDismiss(
	onDismiss: () => void,
): {
	handleTouchStart: (e: TouchEvent) => void;
	handleTouchMove: (e: TouchEvent) => void;
	handleTouchEnd: () => void;
	handleTouchCancel: () => void;
	state: SwipeState;
	reset: () => void;
} {
	let state = $state<SwipeState>({ swiping: false, offsetX: 0, dismissed: false });
	let startX = 0;
	let startY = 0;
	let locked = false;

	function handleTouchStart(e: TouchEvent) {
		startX = e.touches[0].clientX;
		startY = e.touches[0].clientY;
		state.swiping = false;
		state.offsetX = 0;
		state.dismissed = false;
		locked = false;
	}

	function handleTouchMove(e: TouchEvent) {
		const dx = e.touches[0].clientX - startX;
		const dy = e.touches[0].clientY - startY;

		if (!locked && (Math.abs(dx) > DIRECTION_LOCK_THRESHOLD || Math.abs(dy) > DIRECTION_LOCK_THRESHOLD)) {
			locked = true;
			if (Math.abs(dy) > Math.abs(dx)) return;
			state.swiping = true;
		}

		if (!state.swiping) return;
		state.offsetX = dx;
		state.dismissed = Math.abs(dx) >= SWIPE_THRESHOLD;
	}

	function handleTouchEnd() {
		if (state.dismissed) {
			onDismiss();
		}
		state.swiping = false;
		state.offsetX = 0;
		state.dismissed = false;
	}

	function handleTouchCancel() {
		state.swiping = false;
		state.offsetX = 0;
		state.dismissed = false;
	}

	function reset() {
		state.swiping = false;
		state.offsetX = 0;
		state.dismissed = false;
		locked = false;
	}

	return { handleTouchStart, handleTouchMove, handleTouchEnd, handleTouchCancel, state, reset };
}
