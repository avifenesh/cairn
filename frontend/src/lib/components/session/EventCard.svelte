<script lang="ts" module>
	function formatDiff(diff: string): string {
		let oldLine = 0, newLine = 0;
		let inHunk = false;
		return diff.split('\n').map(line => {
			const escaped = line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
			// Parse hunk header for line numbers.
			if (line.startsWith('@@')) {
				inHunk = true;
				const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)/);
				if (match) {
					oldLine = parseInt(match[1], 10);
					newLine = parseInt(match[2], 10);
				}
				return `<span class="diff-hunk">${escaped}</span>`;
			}
			// Metadata lines before first hunk (diff --git, index, ---/+++).
			if (!inHunk || line.startsWith('+++') || line.startsWith('---')) {
				return `<span class="diff-meta">${escaped}</span>`;
			}
			// Build line number gutter.
			let gutter = '';
			if (line.startsWith('+')) {
				gutter = `<span class="diff-gutter diff-gutter-add">${String(newLine++).padStart(4)}</span>`;
				return `${gutter}<span class="diff-add">${escaped}</span>`;
			}
			if (line.startsWith('-')) {
				gutter = `<span class="diff-gutter diff-gutter-del">${String(oldLine++).padStart(4)}</span>`;
				return `${gutter}<span class="diff-del">${escaped}</span>`;
			}
			// Context line.
			gutter = `<span class="diff-gutter">${String(oldLine++).padStart(4)}</span>`;
			newLine++;
			return `${gutter}<span class="diff-ctx">${escaped}</span>`;
		}).join('\n');
	}
</script>

<script lang="ts">
	import type { SessionEvent } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { relativeTime, formatTime } from '$lib/utils/time';
	import { Wrench, Brain, AlertTriangle, MessageSquare, CheckCircle, XCircle, Loader2, Play, ChevronDown, ChevronUp, Code, PanelRight } from '@lucide/svelte';

	let { event, completedToolIds = new Set<string>(), isCompleted = false, onFileClick }: {
		event: SessionEvent;
		completedToolIds?: Set<string>;
		isCompleted?: boolean;
		onFileClick?: (path: string) => void;
	} = $props();

	const p = $derived(event.payload);
	const isError = $derived(p.isError === true || p.state === 'failed');
	const toolPending = $derived(
		event.eventType === 'tool_call' && !completedToolIds.has(String(p.toolId ?? ''))
	);

	let expandedInput = $state(false);
	let expandedOutput = $state(false);
	let expandedThinking = $state(false);
	let expandedDiff = $state(false);

	// Timestamp display: absolute for completed sessions, relative for active.
	const timeStr = $derived(
		isCompleted ? formatTime(event.timestamp) : relativeTime(event.timestamp)
	);

	// Live elapsed timer for pending tool calls.
	let now = $state(Date.now());
	$effect(() => {
		if (!toolPending) return;
		const interval = setInterval(() => { now = Date.now(); }, 1000);
		return () => clearInterval(interval);
	});
	const elapsed = $derived(
		toolPending && event.timestamp
			? Math.floor((now - new Date(event.timestamp).getTime()) / 1000)
			: 0
	);

	// Duration formatting with color coding.
	const durationMs = $derived(Number(p.durationMs ?? 0));
	const durationStr = $derived(
		durationMs > 1000 ? `${(durationMs / 1000).toFixed(1)}s` : durationMs > 0 ? `${durationMs}ms` : ''
	);
	const durationClass = $derived(
		durationMs > 5000 ? 'duration-slow' : durationMs > 1000 ? 'duration-normal' : 'duration-fast'
	);

	function toolInputSummary(toolName: unknown, input: unknown): string {
		if (!input || typeof input !== 'object') return '';
		const inp = input as Record<string, unknown>;
		const name = String(toolName ?? '');
		if (name === 'cairn.shell' || name.endsWith('.shell')) {
			const cmd = String(inp.command ?? '');
			return cmd.length > 120 ? cmd.slice(0, 120) + '...' : cmd;
		}
		if (['cairn.readFile', 'cairn.writeFile', 'cairn.editFile'].includes(name) ||
			name.endsWith('.readFile') || name.endsWith('.writeFile') || name.endsWith('.editFile'))
			return String(inp.path ?? '');
		if (name === 'cairn.gitRun' || name.endsWith('.gitRun')) {
			const args = inp.args ?? inp.command ?? '';
			return 'git ' + (Array.isArray(args) ? args.join(' ') : String(args));
		}
		if (name === 'cairn.webSearch' || name.endsWith('.webSearch')) return String(inp.query ?? '');
		if (name === 'cairn.readURL' || name.endsWith('.readURL')) return String(inp.url ?? '');
		for (const v of Object.values(inp)) {
			if (typeof v === 'string' && v.length > 0) return v.length > 100 ? v.slice(0, 100) + '...' : v;
		}
		return '';
	}

	function formatJSON(val: unknown): string {
		if (!val) return '';
		try { return JSON.stringify(val, null, 2); } catch { return String(val); }
	}

	const summary = $derived(toolInputSummary(p.toolName, p.input));
	const fullInput = $derived(formatJSON(p.input));
	const outputText = $derived(String(p.output ?? p.error ?? ''));
	const outputPreview = $derived(outputText.length > 200 ? outputText.slice(0, 200) + '...' : outputText);
</script>

{#if event.eventType === 'tool_call'}
	<div class="event-card event-card--tool">
		<div class="event-icon"><Wrench size={14} /></div>
		<div class="event-body">
			<div class="event-header">
				<Badge variant="outline" class="text-xs">{p.toolName}</Badge>
				{#if toolPending}
					<Loader2 size={12} class="animate-spin text-muted-foreground" />
					{#if elapsed > 0}<span class="event-meta">{elapsed}s</span>{/if}
				{/if}
				{#if timeStr}<span class="event-time">{timeStr}</span>{/if}
			</div>
			{#if summary}
				<p class="tool-summary">{summary}</p>
			{/if}
			{#if fullInput}
				<button class="expand-btn" onclick={() => (expandedInput = !expandedInput)}>
					{#if expandedInput}<ChevronUp size={10} />{:else}<ChevronDown size={10} />{/if}
					<span>{expandedInput ? 'hide' : 'input'}</span>
				</button>
				{#if expandedInput}
					<pre class="code-block">{fullInput}</pre>
				{/if}
			{/if}
		</div>
	</div>

{:else if event.eventType === 'tool_result'}
	<div class="event-card event-card--tool-result" class:event-card--error={isError}>
		<div class="event-icon">
			{#if isError}<XCircle size={14} class="text-destructive" />
			{:else}<CheckCircle size={14} class="text-green-500" />{/if}
		</div>
		<div class="event-body">
			<div class="event-header">
				<Badge variant={isError ? 'destructive' : 'secondary'} class="text-xs">{p.toolName}</Badge>
				{#if durationStr}
					<span class="event-meta {durationClass}">{durationStr}</span>
				{/if}
				{#if timeStr}<span class="event-time">{timeStr}</span>{/if}
			</div>
			{#if outputText}
				{#if outputText.length <= 200}
					<pre class="output-preview" class:output-error={isError}>{outputText}</pre>
				{:else}
					<pre class="output-preview" class:output-error={isError}>{expandedOutput ? outputText : outputPreview}</pre>
					<button class="expand-btn" onclick={() => (expandedOutput = !expandedOutput)}>
						{#if expandedOutput}<ChevronUp size={10} />{:else}<ChevronDown size={10} />{/if}
						<span>{expandedOutput ? 'less' : 'more'}</span>
					</button>
				{/if}
			{/if}
		</div>
	</div>

{:else if event.eventType === 'thinking'}
	<div class="event-card event-card--thinking">
		<div class="event-icon"><Brain size={14} class="text-muted-foreground" /></div>
		<div class="event-body">
			<button class="expand-btn thinking-toggle" onclick={() => (expandedThinking = !expandedThinking)}>
				{#if expandedThinking}<ChevronUp size={10} />{:else}<ChevronDown size={10} />{/if}
				<span class="italic">Thinking...</span>
			</button>
			{#if expandedThinking}
				<p class="thinking-text">{p.text}</p>
			{/if}
		</div>
	</div>

{:else if event.eventType === 'text_delta'}
	{@const isUser = (p.author as string) === 'user'}
	<div class="event-card" class:event-card--user={isUser}>
		<div class="event-icon"><MessageSquare size={14} class={isUser ? 'text-blue-400' : 'text-[var(--text-tertiary)]'} /></div>
		<div class="event-body">
			<span class="text-xs font-medium" class:text-blue-400={isUser}>{isUser ? 'You' : 'Agent'}</span>
			{#if isUser}
				<p class="message-text">{p.text}</p>
			{:else}
				<div class="cairn-prose message-text">{@html renderMarkdown(String(p.text))}</div>
			{/if}
		</div>
	</div>

{:else if event.eventType === 'file_change'}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="event-card event-card--file" onclick={() => onFileClick?.(String(p.path))}
		role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && onFileClick?.(String(p.path))}>
		<div class="event-icon"><Code size={14} class="text-green-400" /></div>
		<div class="event-body">
			<div class="event-header">
				<span class="text-xs font-mono font-medium">{p.path}</span>
				<Badge variant="outline" class="text-xs">{p.operation}</Badge>
				{#if onFileClick}
					<!-- stopPropagation prevents the parent file_change card's onclick from also firing. -->
					<button class="open-panel-btn" onclick={(e) => { e.stopPropagation(); onFileClick?.(String(p.path)); }}
						aria-label="Open in diff panel">
						<PanelRight size={10} /><span>panel</span>
					</button>
				{/if}
			</div>
			{#if p.diff}
				<button class="expand-btn" onclick={(e) => { e.stopPropagation(); expandedDiff = !expandedDiff; }}>
					{#if expandedDiff}<ChevronUp size={10} />{:else}<ChevronDown size={10} />{/if}
					<span>{expandedDiff ? 'hide diff' : 'show diff'}</span>
				</button>
				{#if expandedDiff}
					<pre class="diff-block">{@html formatDiff(String(p.diff))}</pre>
				{/if}
			{/if}
		</div>
	</div>

{:else if event.eventType === 'state_change'}
	<div class="event-card event-card--state">
		<div class="event-icon"><Play size={14} /></div>
		<div class="event-body">
			<div class="event-header">
				<Badge variant={p.state === 'completed' ? 'default' : p.state === 'failed' ? 'destructive' : 'secondary'} class="text-xs">
					{p.state}
				</Badge>
				{#if p.reason}<span class="event-meta">{p.reason}</span>{/if}
				{#if timeStr}<span class="event-time">{timeStr}</span>{/if}
			</div>
		</div>
	</div>

{:else if event.eventType === 'user_steer'}
	<div class="event-card event-card--steer">
		<div class="event-icon"><MessageSquare size={14} class="text-blue-400" /></div>
		<div class="event-body">
			<p class="text-sm text-blue-400 italic">{p.content}</p>
		</div>
	</div>

{:else if event.eventType === 'round_complete'}
	<!-- round_complete events are replaced by RoundSeparator in ActivityStream -->

{:else if event.eventType === 'approval_request'}
	<div class="event-card event-card--approval">
		<div class="event-icon"><AlertTriangle size={14} class="text-amber-500" /></div>
		<div class="event-body">
			<Badge variant="outline" class="border-amber-500 text-amber-500 text-xs">Approval Required</Badge>
			<p class="text-sm mt-1">{p.description ?? p.operation}</p>
		</div>
	</div>
{/if}

<style>
	.event-card {
		display: flex;
		gap: 0.5rem;
		padding: 0.375rem 0.5rem;
		border-radius: 0.375rem;
		align-items: flex-start;
	}
	.event-card:hover { background: var(--color-surface-hover, hsl(var(--muted) / 0.5)); }
	.event-icon { flex-shrink: 0; margin-top: 0.125rem; }
	.event-body { flex: 1; min-width: 0; }
	.event-header { display: flex; align-items: center; gap: 0.375rem; flex-wrap: wrap; }
	.event-meta { font-size: 0.6875rem; color: var(--text-tertiary, hsl(var(--muted-foreground))); }
	.event-time { font-size: 0.625rem; color: var(--text-tertiary, hsl(var(--muted-foreground))); margin-left: auto; }

	.tool-summary {
		font-size: 0.75rem; font-family: var(--font-mono, monospace);
		color: var(--text-secondary, hsl(var(--muted-foreground)));
		margin-top: 0.25rem; word-break: break-all; white-space: pre-wrap; line-height: 1.4;
	}
	.expand-btn {
		display: inline-flex; align-items: center; gap: 0.25rem;
		font-size: 0.625rem; color: var(--text-tertiary, hsl(var(--muted-foreground)));
		background: none; border: none; cursor: pointer; padding: 0.125rem 0; margin-top: 0.125rem;
	}
	.expand-btn:hover { color: var(--cairn-accent, #60a5fa); }
	.thinking-toggle { font-size: 0.6875rem; }

	.code-block, .diff-block {
		font-size: 0.6875rem; font-family: var(--font-mono, monospace);
		background: hsl(var(--muted) / 0.5); border-radius: 0.25rem;
		padding: 0.375rem 0.5rem; margin-top: 0.25rem;
		overflow-x: auto; max-height: 16rem; overflow-y: auto;
		white-space: pre-wrap; word-break: break-all; line-height: 1.3;
	}
	.output-preview {
		font-size: 0.6875rem; font-family: var(--font-mono, monospace);
		color: var(--text-secondary, hsl(var(--muted-foreground)));
		margin-top: 0.25rem; white-space: pre-wrap; word-break: break-all;
		line-height: 1.3; max-height: 16rem; overflow-y: auto;
	}
	.output-error { color: hsl(var(--destructive)); }
	.thinking-text {
		font-size: 0.75rem; color: var(--text-tertiary, hsl(var(--muted-foreground)));
		font-style: italic; white-space: pre-wrap; word-break: break-word;
		line-height: 1.4; margin-top: 0.25rem; max-height: 20rem; overflow-y: auto;
	}
	.message-text {
		font-size: 0.8125rem; line-height: 1.5;
		white-space: pre-wrap; word-break: break-word; margin-top: 0.125rem;
	}

	/* Duration color coding */
	.duration-fast { color: #22c55e; }
	.duration-normal { color: var(--text-tertiary, hsl(var(--muted-foreground))); }
	.duration-slow { color: #f59e0b; }

	/* Card variants */
	.event-card--user { border-left: 2px solid var(--cairn-accent, #60a5fa); background: hsl(var(--muted) / 0.2); }
	.event-card--approval { border-left: 2px solid var(--color-warning, #f59e0b); background: hsl(var(--muted) / 0.3); }
	.event-card--steer { border-left: 2px solid var(--cairn-accent, #60a5fa); }
	.event-card--error { border-left: 2px solid var(--color-error, hsl(var(--destructive))); }
	.event-card--file { border-left: 2px solid #22c55e; cursor: pointer; }

	.open-panel-btn {
		display: inline-flex; align-items: center; gap: 0.25rem;
		font-size: 0.5625rem; color: var(--text-tertiary, hsl(var(--muted-foreground)));
		background: none; border: none; cursor: pointer; padding: 0.0625rem 0.25rem;
		margin-left: auto; border-radius: 0.125rem;
	}
	.open-panel-btn:hover { color: var(--cairn-accent, #10b981); background: var(--accent-dim, rgba(16,185,129,0.1)); }

	/* Inline diff styling */
	.diff-block :global(.diff-gutter) {
		display: inline-block; width: 4ch; text-align: right;
		color: var(--text-tertiary, hsl(var(--muted-foreground)));
		margin-right: 0.5rem; user-select: none; opacity: 0.6;
	}
	.diff-block :global(.diff-gutter-add) { color: #34d399; }
	.diff-block :global(.diff-gutter-del) { color: #f87171; }
	.diff-block :global(.diff-add) { color: #34d399; }
	.diff-block :global(.diff-del) { color: #f87171; }
	.diff-block :global(.diff-ctx) { color: var(--text-secondary, hsl(var(--muted-foreground))); }
	.diff-block :global(.diff-hunk) { color: #818cf8; font-style: italic; }
	.diff-block :global(.diff-meta) { color: var(--text-tertiary, hsl(var(--muted-foreground))); font-weight: 500; }

	/* Markdown prose for agent messages */
	.cairn-prose :global(p) { margin: 0.25rem 0; }
	.cairn-prose :global(code) {
		font-size: 0.75rem; background: hsl(var(--muted) / 0.5);
		padding: 0.125rem 0.25rem; border-radius: 0.25rem;
	}
	.cairn-prose :global(pre) {
		background: hsl(var(--muted) / 0.5); border-radius: 0.375rem;
		padding: 0.5rem; margin: 0.375rem 0; overflow-x: auto;
		font-size: 0.75rem; line-height: 1.4;
	}
	.cairn-prose :global(pre code) { background: none; padding: 0; }
	.cairn-prose :global(ul), .cairn-prose :global(ol) { padding-left: 1.25rem; margin: 0.25rem 0; }
	.cairn-prose :global(li) { margin: 0.125rem 0; }
	.cairn-prose :global(a) { color: var(--cairn-accent, #60a5fa); text-decoration: underline; }
	.cairn-prose :global(blockquote) {
		border-left: 2px solid var(--cairn-accent, #60a5fa);
		padding-left: 0.75rem; margin: 0.25rem 0;
		color: var(--text-secondary, hsl(var(--muted-foreground)));
	}
	.cairn-prose :global(h1), .cairn-prose :global(h2), .cairn-prose :global(h3) {
		font-weight: 600; margin: 0.5rem 0 0.25rem;
	}
	.cairn-prose :global(h1) { font-size: 1rem; }
	.cairn-prose :global(h2) { font-size: 0.9375rem; }
	.cairn-prose :global(h3) { font-size: 0.875rem; }
	.cairn-prose :global(table) { border-collapse: collapse; margin: 0.375rem 0; font-size: 0.75rem; }
	.cairn-prose :global(th), .cairn-prose :global(td) {
		border: 1px solid hsl(var(--border)); padding: 0.25rem 0.5rem;
	}
	.cairn-prose :global(th) { background: hsl(var(--muted) / 0.3); font-weight: 600; }
	.cairn-prose :global(strong) { font-weight: 600; }
</style>
