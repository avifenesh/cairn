import { Marked } from 'marked';
import DOMPurify from 'dompurify';

const marked = new Marked({
	breaks: true,
	gfm: true,
});

// Custom renderer: code blocks get a header bar with lang label + copy button
// Copy is handled via event delegation in StreamingText, not inline onclick
const renderer = {
	code({ text, lang }: { text: string; lang?: string | null }) {
		const langLabel = (lang || '').replace(/[^a-zA-Z0-9_-]/g, '');
		const escapedCode = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		return `<div class="cairn-code-block" data-lang="${langLabel}">` +
			`<div class="cairn-code-header">` +
				`<span class="cairn-code-lang">${langLabel}</span>` +
				`<button class="cairn-code-copy" data-copy="true">Copy</button>` +
			`</div>` +
			`<pre><code class="language-${langLabel}">${escapedCode}</code></pre>` +
		`</div>`;
	},
};

marked.use({ renderer });

export function renderMarkdown(content: string): string {
	const raw = marked.parse(content, { async: false }) as string;
	return DOMPurify.sanitize(raw, {
		ADD_ATTR: ['data-lang', 'data-copy'],
		ADD_TAGS: ['button'],
	});
}
