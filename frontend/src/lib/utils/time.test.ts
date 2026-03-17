import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { relativeTime } from './time';

describe('relativeTime', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-03-17T12:00:00Z'));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('returns "just now" for < 10 seconds ago', () => {
		const d = new Date('2026-03-17T11:59:55Z');
		expect(relativeTime(d)).toBe('just now');
	});

	it('returns seconds for < 1 minute', () => {
		const d = new Date('2026-03-17T11:59:30Z');
		expect(relativeTime(d)).toBe('30s ago');
	});

	it('returns minutes for < 1 hour', () => {
		const d = new Date('2026-03-17T11:45:00Z');
		expect(relativeTime(d)).toBe('15m ago');
	});

	it('returns hours for < 1 day', () => {
		const d = new Date('2026-03-17T09:00:00Z');
		expect(relativeTime(d)).toBe('3h ago');
	});

	it('returns "yesterday" for 1 day', () => {
		const d = new Date('2026-03-16T12:00:00Z');
		expect(relativeTime(d)).toBe('yesterday');
	});

	it('returns days for < 30 days', () => {
		const d = new Date('2026-03-10T12:00:00Z');
		expect(relativeTime(d)).toBe('7d ago');
	});

	it('returns formatted date for >= 30 days', () => {
		const d = new Date('2026-01-01T12:00:00Z');
		const result = relativeTime(d);
		// toLocaleDateString output varies by locale, just check it's not a relative format
		expect(result).not.toContain('ago');
		expect(result).not.toBe('yesterday');
	});

	it('accepts ISO string input', () => {
		expect(relativeTime('2026-03-17T11:59:55Z')).toBe('just now');
	});
});
