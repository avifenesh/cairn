import type { SessionEvent, SessionStatus, FileChange } from '$lib/types';

const MAX_EVENTS = 2000;

function getAuthHeaders(): Record<string, string> {
	const h: Record<string, string> = { 'Content-Type': 'application/json' };
	const token = localStorage.getItem('cairn_api_token');
	if (token) h['X-Api-Token'] = token;
	return h;
}

function getStreamUrl(path: string): string {
	const token = localStorage.getItem('cairn_api_token');
	const params = new URLSearchParams();
	if (token) params.set('token', token);
	const qs = params.toString();
	return qs ? `${path}?${qs}` : path;
}

// Session store state — created per session panel instance.
export class SessionStore {
	events = $state<SessionEvent[]>([]);
	status = $state<SessionStatus>('running');
	currentRound = $state(0);
	totalToolCalls = $state(0);
	totalTokensIn = $state(0);
	totalTokensOut = $state(0);
	streamingText = $state('');
	thinkingText = $state('');
	error = $state<string | null>(null);

	private source: EventSource | null = null;
	private sessionId: string;

	constructor(sessionId: string) {
		this.sessionId = sessionId;
	}

	get fileChanges(): FileChange[] {
		return this.events
			.filter((e) => e.eventType === 'file_change')
			.map((e) => e.payload as unknown as FileChange);
	}

	get pendingApprovals(): SessionEvent[] {
		const requests = this.events.filter((e) => e.eventType === 'approval_request');
		const responses = new Set(
			this.events
				.filter((e) => e.eventType === 'approval_response')
				.map((e) => e.payload.approvalId as string)
		);
		return requests.filter((e) => !responses.has(e.payload.approvalId as string));
	}

	get toolEvents(): SessionEvent[] {
		return this.events.filter(
			(e) => e.eventType === 'tool_call' || e.eventType === 'tool_result'
		);
	}

	get steeringMessages(): SessionEvent[] {
		return this.events.filter((e) => e.eventType === 'user_steer');
	}

	async connect() {
		// Hydrate prior events before opening the live stream.
		await this.hydrate();

		const url = getStreamUrl(`/v1/sessions/${this.sessionId}/stream`);
		this.source = new EventSource(url);

		this.source.addEventListener('session_event', (e: MessageEvent) => {
			try {
				const event: SessionEvent = JSON.parse(e.data);
				this.addEvent(event);
				this.processEvent(event);
			} catch {
				// Ignore parse errors.
			}
		});

		this.source.onerror = () => {
			// EventSource auto-reconnects. Update UI if closed permanently.
			if (this.source?.readyState === EventSource.CLOSED) {
				this.error = 'Connection lost';
			}
		};
	}

	disconnect() {
		this.source?.close();
		this.source = null;
	}

	private addEvent(event: SessionEvent) {
		this.events = [...this.events, event];
		// Cap stored events to prevent unbounded memory growth.
		if (this.events.length > MAX_EVENTS) {
			this.events = this.events.slice(-MAX_EVENTS);
		}
	}

	private async hydrate() {
		try {
			const res = await fetch(`/v1/sessions/${this.sessionId}/events?limit=200`, {
				headers: getAuthHeaders(),
				credentials: 'include',
			});
			if (!res.ok) return;
			const data = await res.json();
			// Convert session history events to SessionEvent format for display.
			// The events endpoint returns the raw agent Event format with parts.
			// We reconstruct SessionEvent-compatible entries from them.
			if (Array.isArray(data.events)) {
				for (const ev of data.events) {
					if (!Array.isArray(ev.parts)) continue;
					for (const part of ev.parts) {
						const partType = part.text !== undefined ? 'text' :
							part.toolName !== undefined ? 'tool' :
							part.text !== undefined ? 'reasoning' : null;
						if (partType === 'tool') {
							const eventType = part.status === 'running' || part.status === 'pending' ? 'tool_call' : 'tool_result';
							this.addEvent({
								sessionId: this.sessionId,
								eventType,
								payload: {
									toolId: part.callId, toolName: part.toolName,
									isError: part.status === 'failed', durationMs: part.duration,
								},
								timestamp: ev.timestamp,
							});
							if (part.status === 'completed' || part.status === 'failed') {
								this.totalToolCalls++;
							}
						}
					}
				}
			}
		} catch {
			// Hydration is best-effort; the live stream will provide new events.
		}
	}

	private processEvent(event: SessionEvent) {
		const p = event.payload;
		switch (event.eventType) {
			case 'state_change':
				this.status = (p.state as SessionStatus) ?? this.status;
				break;
			case 'text_delta':
				this.streamingText += (p.text as string) ?? '';
				break;
			case 'thinking':
				this.thinkingText += (p.text as string) ?? '';
				break;
			case 'tool_call':
				// Reset streaming text when agent switches to tool calls.
				this.streamingText = '';
				this.thinkingText = '';
				break;
			case 'tool_result':
				this.totalToolCalls++;
				break;
			case 'round_complete':
				this.currentRound = (p.round as number) ?? this.currentRound;
				this.totalTokensIn += (p.inputTokens as number) ?? 0;
				this.totalTokensOut += (p.outputTokens as number) ?? 0;
				this.streamingText = '';
				this.thinkingText = '';
				break;
		}
	}

	async steer(content: string, priority: 'normal' | 'stop' = 'normal') {
		const res = await fetch(`/v1/sessions/${this.sessionId}/steer`, {
			method: 'POST',
			headers: getAuthHeaders(),
			credentials: 'include',
			body: JSON.stringify({ content, priority }),
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: 'Failed to send steering message' }));
			throw new Error(err.error ?? 'Failed to steer');
		}
	}

	async stop() {
		return this.steer('', 'stop');
	}
}
