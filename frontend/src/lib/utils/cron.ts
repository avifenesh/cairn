// Cron expression → human-readable description.
// Covers common patterns, falls back to raw expression.

const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const SHORT_DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

export function cronToHuman(expr: string): string {
	const parts = expr.trim().split(/\s+/);
	if (parts.length !== 5) return expr;
	const [min, hour, dom, mon, dow] = parts;

	// Every N minutes
	if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
		return `Every ${min.slice(2)} minutes`;
	}

	// Every hour at :MM
	if (!min.includes('*') && !min.includes('/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
		return `Every hour at :${min.padStart(2, '0')}`;
	}

	// Specific time (single values only, not lists like "9,17")
	if (!min.includes('*') && !min.includes('/') && !min.includes(',') &&
		!hour.includes('*') && !hour.includes('/') && !hour.includes(',')) {
		const h = parseInt(hour);
		const m = parseInt(min);
		const time = formatTime(h, m);

		// Every day
		if (dom === '*' && mon === '*' && dow === '*') {
			return `Daily at ${time}`;
		}

		// Weekdays
		if (dom === '*' && mon === '*' && dow === '1-5') {
			return `Weekdays at ${time}`;
		}

		// Weekends
		if (dom === '*' && mon === '*' && (dow === '0,6' || dow === '6,0')) {
			return `Weekends at ${time}`;
		}

		// Specific day of week
		if (dom === '*' && mon === '*' && /^\d$/.test(dow)) {
			return `${DAYS[parseInt(dow)]}s at ${time}`;
		}

		// Day range
		if (dom === '*' && mon === '*' && /^\d-\d$/.test(dow)) {
			const [start, end] = dow.split('-').map(Number);
			return `${SHORT_DAYS[start]}-${SHORT_DAYS[end]} at ${time}`;
		}

		// Specific day of month
		if (!dom.includes('*') && mon === '*' && dow === '*') {
			return `${ordinal(parseInt(dom))} of each month at ${time}`;
		}
	}

	return expr;
}

function formatTime(h: number, m: number): string {
	const ampm = h >= 12 ? 'PM' : 'AM';
	const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
	return `${h12}:${String(m).padStart(2, '0')} ${ampm}`;
}

function ordinal(n: number): string {
	const s = ['th', 'st', 'nd', 'rd'];
	const v = n % 100;
	return n + (s[(v - 20) % 10] || s[v] || s[0]);
}
