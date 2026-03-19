import { describe, it, expect } from 'vitest';
import { cronToHuman } from './cron';

describe('cronToHuman', () => {
	it('converts every N minutes', () => {
		expect(cronToHuman('*/30 * * * *')).toBe('Every 30 minutes');
		expect(cronToHuman('*/5 * * * *')).toBe('Every 5 minutes');
	});

	it('converts daily at specific time', () => {
		expect(cronToHuman('0 9 * * *')).toBe('Daily at 9:00 AM');
		expect(cronToHuman('30 14 * * *')).toBe('Daily at 2:30 PM');
		expect(cronToHuman('0 0 * * *')).toBe('Daily at 12:00 AM');
	});

	it('converts weekdays', () => {
		expect(cronToHuman('0 9 * * 1-5')).toBe('Weekdays at 9:00 AM');
	});

	it('converts specific day', () => {
		expect(cronToHuman('0 10 * * 0')).toBe('Sundays at 10:00 AM');
		expect(cronToHuman('0 17 * * 5')).toBe('Fridays at 5:00 PM');
	});

	it('converts every hour', () => {
		expect(cronToHuman('0 * * * *')).toBe('Every hour at :00');
		expect(cronToHuman('30 * * * *')).toBe('Every hour at :30');
	});

	it('returns raw for complex expressions', () => {
		expect(cronToHuman('0 9,17 * * *')).toBe('0 9,17 * * *');
	});

	it('handles day of month', () => {
		expect(cronToHuman('0 9 1 * *')).toBe('1st of each month at 9:00 AM');
		expect(cronToHuman('0 9 15 * *')).toBe('15th of each month at 9:00 AM');
	});
});
