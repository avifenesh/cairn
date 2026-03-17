// Keyboard navigation state for j/k/o/r/x/s/a/d triage shortcuts
// Plan: j/k navigate items, o open URL, r mark read, x toggle selection, s sync, a approve, d deny

let focusedIndex = $state(-1);
let itemCount = $state(0);
let selectionToggleCallback: ((index: number) => void) | null = null;

export const keyboardNav = {
	get focusedIndex() { return focusedIndex; },

	setItemCount(count: number) {
		itemCount = count;
		if (focusedIndex >= count) focusedIndex = Math.max(0, count - 1);
	},

	moveDown() {
		if (itemCount === 0) return;
		focusedIndex = focusedIndex < itemCount - 1 ? focusedIndex + 1 : 0;
	},

	moveUp() {
		if (itemCount === 0) return;
		focusedIndex = focusedIndex > 0 ? focusedIndex - 1 : itemCount - 1;
	},

	toggleSelection() {
		if (focusedIndex >= 0 && selectionToggleCallback) {
			selectionToggleCallback(focusedIndex);
		}
	},

	setSelectionCallback(cb: ((index: number) => void) | null) {
		selectionToggleCallback = cb;
	},

	reset() {
		focusedIndex = -1;
		itemCount = 0;
		selectionToggleCallback = null;
	},
};
