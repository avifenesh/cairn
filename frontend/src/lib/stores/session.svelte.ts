import type { SessionEvent, SessionStatus, FileChange } from '$lib/types';

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

	connect() {
		const base = '';
		this.source = new EventSource(`${base}/v1/sessions/${this.sessionId}/stream`);

		this.source.addEventListener('session_event', (e: MessageEvent) => {
			try {
				const event: SessionEvent = JSON.parse(e.data);
				this.events = [...this.events, event];
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

	async steer(content: string, priority: 'normal' | 'urgent' | 'stop' = 'normal') {
		const base = '';
		const res = await fetch(`${base}/v1/sessions/${this.sessionId}/steer`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
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
