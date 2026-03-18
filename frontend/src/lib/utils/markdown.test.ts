import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';

describe('renderMarkdown', () => {
	it('renders bold text', () => {
		const result = renderMarkdown('**hello**');
		expect(result).toContain('<strong>hello</strong>');
	});

	it('renders inline code', () => {
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

	it('does not allow onclick attributes', () => {
		const result = renderMarkdown('<button onclick="alert(1)">click</button>');
		expect(result).not.toContain('onclick');
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

	it('renders fenced code blocks with header bar', () => {
		const result = renderMarkdown('```javascript\nconst x = 1;\n```');
		expect(result).toContain('cairn-code-block');
		expect(result).toContain('cairn-code-header');
		expect(result).toContain('cairn-code-lang');
		expect(result).toContain('javascript');
		expect(result).toContain('const x = 1;');
	});

	it('renders code block copy button with data-copy attribute', () => {
		const result = renderMarkdown('```go\nfmt.Println("hi")\n```');
		expect(result).toContain('data-copy="true"');
		expect(result).toContain('Copy');
		// No inline onclick
		expect(result).not.toContain('onclick');
	});

	it('sanitizes lang label in code blocks', () => {
		const result = renderMarkdown('```"><script>alert(1)</script>\ncode\n```');
		// Script tags stripped, no attribute breakout
		expect(result).not.toContain('<script>');
		expect(result).not.toContain('onclick');
		expect(result).not.toContain('onerror');
		// Lang label should be alphanumeric only (special chars stripped)
		expect(result).toContain('data-lang="scriptalert1script"');
	});

	it('renders code block without lang label', () => {
		const result = renderMarkdown('```\nplain code\n```');
		expect(result).toContain('cairn-code-block');
		expect(result).toContain('plain code');
	});

	it('renders tables', () => {
		const result = renderMarkdown('| A | B |\n|---|---|\n| 1 | 2 |');
		expect(result).toContain('<table>');
		expect(result).toContain('<th>');
	});

	it('renders blockquotes', () => {
		const result = renderMarkdown('> quoted text');
		expect(result).toContain('<blockquote>');
		expect(result).toContain('quoted text');
	});
});
