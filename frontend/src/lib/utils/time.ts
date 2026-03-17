const MINUTE = 60;
const HOUR = 3600;
const DAY = 86400;

export function relativeTime(date: string | Date): string {
	const d = typeof date === 'string' ? new Date(date) : date;
	const seconds = Math.floor((Date.now() - d.getTime()) / 1000);

	if (seconds < 10) return 'just now';
	if (seconds < MINUTE) return `${seconds}s ago`;
	if (seconds < HOUR) return `${Math.floor(seconds / MINUTE)}m ago`;
	if (seconds < DAY) return `${Math.floor(seconds / HOUR)}h ago`;
	const days = Math.floor(seconds / DAY);
	if (days === 1) return 'yesterday';
	if (days < 30) return `${days}d ago`;
	return d.toLocaleDateString();
}
