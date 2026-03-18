import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import MessageBubble from './MessageBubble.svelte';
import type { ChatMessage } from '$lib/types';

function makeMessage(overrides: Partial<ChatMessage> = {}): ChatMessage {
	return {
		id: 'msg-1',
		role: 'assistant',
		content: 'Hello world',
		createdAt: '2026-03-17T12:00:00Z',
		...overrides,
	};
}

describe('MessageBubble', () => {
	it('renders message content', () => {
		render(MessageBubble, { message: makeMessage({ content: 'Test reply' }) });
		expect(screen.getByText('Test reply')).toBeInTheDocument();
	});

	it('renders user messages with user icon area', () => {
		const { container } = render(MessageBubble, {
			message: makeMessage({ role: 'user', content: 'Hi' }),
		});
		// User messages use flex-row-reverse
		const wrapper = container.querySelector('.flex-row-reverse');
		expect(wrapper).toBeInTheDocument();
	});

	it('renders assistant messages without flex-row-reverse', () => {
		const { container } = render(MessageBubble, {
			message: makeMessage({ role: 'assistant', content: 'Hi' }),
		});
		const wrapper = container.querySelector('.flex-row-reverse');
		expect(wrapper).toBeNull();
	});

	it('renders tool call chips when present', () => {
		render(MessageBubble, {
			message: makeMessage({
				toolCalls: [
					{ toolName: 'readFile', phase: 'result' },
					{ toolName: 'shell', phase: 'start' },
				],
			}),
		});
		expect(screen.getByText('readFile')).toBeInTheDocument();
		expect(screen.getByText('shell')).toBeInTheDocument();
	});

	it('does not render tool call section when no tool calls', () => {
		const { container } = render(MessageBubble, {
			message: makeMessage({ toolCalls: [] }),
		});
		// No tool call chips rendered
		expect(screen.queryByText('readFile')).toBeNull();
	});

	it('renders reasoning block when reasoning steps present', () => {
		render(MessageBubble, {
			message: makeMessage({
				reasoning: [{ round: 1, thought: 'Let me think about this...' }],
			}),
		});
		// Collapsed by default — header shows step count
		expect(screen.getByText(/Thought.*1 step/)).toBeInTheDocument();
	});

	it('renders timestamp', () => {
		const { container } = render(MessageBubble, { message: makeMessage() });
		const time = container.querySelector('time');
		expect(time).toBeInTheDocument();
	});
});
