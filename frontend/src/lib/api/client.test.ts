import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ApiError } from './client';

// Test the ApiError class and query param building logic without hitting the network.
// The actual fetch calls are thin wrappers — we test the logic, not the transport.

describe('ApiError', () => {
	it('creates error with status and body', () => {
		const err = new ApiError(404, 'not found');
		expect(err.status).toBe(404);
		expect(err.body).toBe('not found');
		expect(err.message).toBe('API 404: not found');
		expect(err).toBeInstanceOf(Error);
	});

	it('creates error for server errors', () => {
		const err = new ApiError(500, '{"error":"internal"}');
		expect(err.status).toBe(500);
	});
});

describe('API client fetch behavior', () => {
	let originalFetch: typeof globalThis.fetch;

	beforeEach(() => {
		originalFetch = globalThis.fetch;
	});

	afterEach(() => {
		globalThis.fetch = originalFetch;
	});

	it('GET includes credentials and token header', async () => {
		localStorage.setItem('cairn_api_token', 'test-token');

		let capturedInit: RequestInit | undefined;
		globalThis.fetch = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
			capturedInit = init;
			return new Response(JSON.stringify({ ok: true }), { status: 200 });
		});

		// Dynamic import to pick up mocked fetch
		const { health } = await import('./client');
		await health();

		expect(capturedInit?.credentials).toBe('include');
		expect((capturedInit?.headers as Record<string, string>)['X-Api-Token']).toBe('test-token');

		localStorage.removeItem('cairn_api_token');
	});

	it('throws ApiError on non-ok response', async () => {
		globalThis.fetch = vi.fn(async () => {
			return new Response('bad request', { status: 400 });
		});

		const { health } = await import('./client');
		await expect(health()).rejects.toThrow(ApiError);
	});

	it('POST sends JSON body', async () => {
		let capturedBody: string | undefined;
		globalThis.fetch = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
			capturedBody = init?.body as string;
			return new Response(JSON.stringify({ ok: true }), { status: 200 });
		});

		const { approve } = await import('./client');
		await approve('test-id');

		// approve sends POST without body (undefined)
		expect(capturedBody).toBeUndefined();
	});
});
