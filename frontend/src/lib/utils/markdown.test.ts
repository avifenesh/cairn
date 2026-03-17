import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';

describe('renderMarkdown', () => {
	it('renders bold text', () => {
		const result = renderMarkdown('**hello**');
		expect(result).toContain('<strong>hello</strong>');
	});

	it('renders code blocks', () => {
		const result = renderMarkdown('`inline code`');
		expect(result).toContain('<code>inline code</code>');
	});

	it('renders links', () => {
		const result = renderMarkdown('[text](https://example.com)');
		expect(result).toContain('href="https://example.com"');
		expect(result).toContain('text');
	});

	it('sanitizes script tags (XSS prevention)', () => {
		const result = renderMarkdown('<script>alert("xss")</script>');
		expect(result).not.toContain('<script>');
	});

	it('sanitizes onerror attributes', () => {
		const result = renderMarkdown('<img src=x onerror=alert(1)>');
		expect(result).not.toContain('onerror');
	});

	it('renders GFM line breaks', () => {
		const result = renderMarkdown('line1\nline2');
		expect(result).toContain('<br');
	});

	it('handles empty string', () => {
		expect(renderMarkdown('')).toBe('');
	});

	it('renders lists', () => {
		const result = renderMarkdown('- item1\n- item2');
		expect(result).toContain('<li>');
		expect(result).toContain('item1');
		expect(result).toContain('item2');
	});
});
