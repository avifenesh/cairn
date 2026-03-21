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
	// Default to 'completed'. Flipped to 'running' when live events arrive
	// (handles mid-flight sessions). Pre-existing sessions without events stay 'completed'.
	status = $state<SessionStatus>('completed');
	private receivedLiveEvent = false;
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
				// If we receive live events and status is still the default 'completed',
				// flip to 'running' (handles opening a session mid-flight).
				if (!this.receivedLiveEvent && this.status === 'completed') {
					this.status = 'running';
				}
				this.receivedLiveEvent = true;
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
			const res = await fetch(`/v1/sessions/${this.sessionId}/events?limit=500`, {
				headers: getAuthHeaders(),
				credentials: 'include',
			});
			if (!res.ok) return;
			const data = await res.json();
			if (!Array.isArray(data.events) || data.events.length === 0) return;

			// Aggregate raw token-level events into displayable SessionEvents.
			// Raw events have parts: [{text:"word"}, {toolName:"...", status:"..."}]
			// We need to: group consecutive text tokens by author into messages,
			// and emit tool events individually.
			let currentText = '';
			let currentAuthor = '';
			let currentTimestamp = '';

			const flush = () => {
				if (currentText.trim()) {
					this.addEvent({
						sessionId: this.sessionId,
						eventType: 'text_delta',
						payload: { text: currentText.trim(), author: currentAuthor },
						timestamp: currentTimestamp,
					});
				}
				currentText = '';
				currentTimestamp = '';
			};

			for (const ev of data.events) {
				if (!Array.isArray(ev.parts)) continue;
				const author = ev.author ?? '';
				const ts = ev.timestamp ?? '';

				for (const part of ev.parts) {
					if (part.toolName !== undefined) {
						// Flush any pending text before tool events.
						flush();
						const isResult = part.status === 'completed' || part.status === 'failed';
						const payload: Record<string, unknown> = {
							toolId: part.callId, toolName: part.toolName,
							input: part.input,
							isError: part.status === 'failed',
							durationMs: part.duration ? Math.round(part.duration / 1000000) : undefined,
						};
						// Include output and error for tool results.
						if (part.output) payload.output = part.output;
						if (part.error) payload.error = part.error;
						this.addEvent({
							sessionId: this.sessionId,
							eventType: isResult ? 'tool_result' : 'tool_call',
							payload,
							timestamp: ts,
						});
						if (isResult) this.totalToolCalls++;
					} else if (part.text !== undefined) {
						// Aggregate text tokens. Flush on author change.
						if (author !== currentAuthor && currentText) {
							flush();
						}
						currentAuthor = author;
						if (!currentTimestamp) currentTimestamp = ts;
						currentText += part.text;
					}
				}
			}
			flush();
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
