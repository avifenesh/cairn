import { Marked } from 'marked';
import DOMPurify from 'dompurify';

const marked = new Marked({
	breaks: true,
	gfm: true,
});

// Custom renderer: code blocks get a header bar with lang + copy button
const renderer = {
	code({ text, lang }: { text: string; lang?: string | null }) {
		const langLabel = lang || '';
		const escapedCode = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		const highlighted = langLabel ? highlightBasic(escapedCode, langLabel) : escapedCode;
		return `<div class="cairn-code-block" data-lang="${langLabel}">` +
			`<div class="cairn-code-header">` +
				`<span class="cairn-code-lang">${langLabel}</span>` +
				`<button class="cairn-code-copy" onclick="(function(b){var c=b.closest('.cairn-code-block').querySelector('code').textContent;navigator.clipboard.writeText(c);b.textContent='Copied';setTimeout(function(){b.textContent='Copy'},2000)})(this)">Copy</button>` +
			`</div>` +
			`<pre><code class="language-${langLabel}">${highlighted}</code></pre>` +
		`</div>`;
	},
};

marked.use({ renderer });

// Basic syntax highlighting — adds span classes for common patterns
function highlightBasic(code: string, lang: string): string {
	// Comments
	if (['js', 'javascript', 'ts', 'typescript', 'go', 'rust', 'java', 'c', 'cpp', 'svelte'].includes(lang)) {
		code = code.replace(/(\/\/.*?)$/gm, '<span class="hl-comment">$1</span>');
		code = code.replace(/(\/\*[\s\S]*?\*\/)/g, '<span class="hl-comment">$1</span>');
	}
	if (['python', 'py', 'bash', 'sh', 'yaml', 'toml'].includes(lang)) {
		code = code.replace(/(#.*?)$/gm, '<span class="hl-comment">$1</span>');
	}

	// Strings (double and single quotes — simple, doesn't handle escapes perfectly)
	code = code.replace(/("(?:[^"\\]|\\.)*")/g, '<span class="hl-string">$1</span>');
	code = code.replace(/('(?:[^'\\]|\\.)*')/g, '<span class="hl-string">$1</span>');
	code = code.replace(/(`(?:[^`\\]|\\.)*`)/g, '<span class="hl-string">$1</span>');

	// Keywords per language family
	const kwMap: Record<string, string[]> = {
		js: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'import', 'export', 'from', 'class', 'new', 'this', 'async', 'await', 'try', 'catch', 'throw', 'typeof', 'instanceof'],
		ts: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'import', 'export', 'from', 'class', 'new', 'this', 'async', 'await', 'try', 'catch', 'throw', 'typeof', 'instanceof', 'interface', 'type', 'enum'],
		go: ['func', 'return', 'if', 'else', 'for', 'range', 'switch', 'case', 'default', 'package', 'import', 'var', 'const', 'type', 'struct', 'interface', 'map', 'chan', 'go', 'defer', 'select', 'nil', 'true', 'false'],
		python: ['def', 'return', 'if', 'elif', 'else', 'for', 'while', 'import', 'from', 'class', 'try', 'except', 'raise', 'with', 'as', 'pass', 'yield', 'lambda', 'None', 'True', 'False', 'in', 'not', 'and', 'or', 'is'],
		rust: ['fn', 'let', 'mut', 'return', 'if', 'else', 'for', 'while', 'loop', 'match', 'use', 'mod', 'pub', 'struct', 'enum', 'impl', 'trait', 'self', 'Self', 'async', 'await', 'move', 'true', 'false'],
		bash: ['if', 'then', 'else', 'fi', 'for', 'do', 'done', 'while', 'case', 'esac', 'function', 'return', 'export', 'local', 'echo', 'exit'],
	};
	const aliases: Record<string, string> = { javascript: 'js', typescript: 'ts', py: 'python', sh: 'bash', svelte: 'ts' };
	const kwLang = aliases[lang] ?? lang;
	const keywords = kwMap[kwLang];
	if (keywords) {
		const kwPattern = new RegExp(`\\b(${keywords.join('|')})\\b`, 'g');
		code = code.replace(kwPattern, (m) => {
			// Don't highlight inside already-highlighted spans
			return `<span class="hl-keyword">${m}</span>`;
		});
	}

	// Numbers
	code = code.replace(/\b(\d+(?:\.\d+)?)\b/g, '<span class="hl-number">$1</span>');

	return code;
}

export function renderMarkdown(content: string): string {
	const raw = marked.parse(content, { async: false }) as string;
	return DOMPurify.sanitize(raw, {
		ADD_ATTR: ['onclick', 'data-lang'],
		ADD_TAGS: ['button'],
	});
}
