import '@testing-library/jest-dom/vitest';

// Mock localStorage for stores that read from it on module load
const store: Record<string, string> = {};
Object.defineProperty(globalThis, 'localStorage', {
	value: {
		getItem: (key: string) => store[key] ?? null,
		setItem: (key: string, value: string) => { store[key] = value; },
		removeItem: (key: string) => { delete store[key]; },
		clear: () => { Object.keys(store).forEach((k) => delete store[k]); },
	},
});

// Mock crypto.randomUUID
if (!globalThis.crypto?.randomUUID) {
	let counter = 0;
	Object.defineProperty(globalThis, 'crypto', {
		value: {
			randomUUID: () => `test-uuid-${++counter}`,
		},
	});
}
