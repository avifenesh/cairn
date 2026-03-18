import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ChatPanel from './ChatPanel.svelte';

vi.mock('$lib/api/client', () => ({
	getSessions: vi.fn().mockResolvedValue({ items: [] }),
	getSessionMessages: vi.fn().mockResolvedValue({ items: [] }),
	sendMessage: vi.fn().mockResolvedValue({ taskId: 'test-task' }),
}));

vi.mock('$lib/stores/chat.svelte', () => {
	const store = {
		sessions: [],
		currentSessionId: null,
		messages: [],
		streamingMessages: new Map(),
		mode: 'talk',
		loading: false,
		get activeStream() { return null; },
		setSessions: vi.fn(),
		setCurrentSession: vi.fn(),
		setMessages: vi.fn(),
		setMode: vi.fn(),
		setLoading: vi.fn(),
		clearStreaming: vi.fn(),
		startStreaming: vi.fn(),
		appendDelta: vi.fn(),
		completeMessage: vi.fn(),
		appendToolCall: vi.fn(),
		appendReasoning: vi.fn(),
		addUserMessage: vi.fn(),
	};
	return { chatStore: store };
});

describe('ChatPanel', () => {
	it('shows empty state heading when no messages', () => {
		render(ChatPanel);
		expect(screen.getByText('What can I help with?')).toBeInTheDocument();
	});

	it('shows suggestion chips in empty state', () => {
		render(ChatPanel);
		expect(screen.getByText('Summarize my unread feed')).toBeInTheDocument();
		expect(screen.getByText('Plan a weekend trip')).toBeInTheDocument();
	});

	it('populates input when suggestion chip is clicked', async () => {
		render(ChatPanel);
		const chip = screen.getByText('Plan a weekend trip');
		await fireEvent.click(chip);
		const textarea = screen.getByPlaceholderText('Send a message...') as HTMLTextAreaElement;
		expect(textarea.value).toBe('Plan a weekend trip');
	});

	it('has send button', () => {
		render(ChatPanel);
		expect(screen.getByLabelText('Send')).toBeInTheDocument();
	});
});
