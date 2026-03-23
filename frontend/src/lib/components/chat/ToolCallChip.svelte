<script lang="ts">
	import { Wrench, Check, AlertCircle, ChevronDown, ChevronUp } from '@lucide/svelte';

	let {
		toolName,
		phase,
		args,
		result,
		error,
		durationMs,
		isExternal = false,
	}: {
		toolName: string;
		phase: string;
		args?: Record<string, unknown>;
		result?: string;
		error?: string;
		durationMs?: number;
		isExternal?: boolean;
	} = $props();

	let expanded = $state(false);

	const hasDetails = $derived(!!(result || error || (args && Object.keys(args).length > 0)));
	const isError = $derived(!!error);
	const isDone = $derived(phase === 'result');

	function formatDuration(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function truncate(text: string, max: number): string {
		if (text.length <= max) return text;
		return text.slice(0, max) + '...';
	}

	// Split result into summary and diff if present.
	const resultParts = $derived.by((): { text: string; diff: string } => {
		if (!result) return { text: '', diff: '' };
		const idx = result.indexOf('\ndiff --git ');
		if (idx >= 0) return { text: result.slice(0, idx).trim(), diff: result.slice(idx + 1) };
		const idx2 = result.indexOf('\n--- ');
		if (idx2 >= 0 && result.indexOf('\n+++ ', idx2) > idx2) return { text: result.slice(0, idx2).trim(), diff: result.slice(idx2 + 1) };
		return { text: result, diff: '' };
	});

	let showDiff = $state(false);

	function formatDiffLine(line: string): string {
		const esc = line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		if (line.startsWith('+') && !line.startsWith('+++')) return `<span style="color: var(--color-success)">${esc}</span>`;
		if (line.startsWith('-') && !line.startsWith('---')) return `<span style="color: var(--color-error)">${esc}</span>`;
		if (line.startsWith('@@')) return `<span style="color: var(--src-github)">${esc}</span>`;
		return esc;
	}

	function formatDiff(diff: string): string {
		return diff.split('\n').map(formatDiffLine).join('\n');
	}
</script>

<div class="inline-flex flex-col">
	<button
		class="inline-flex items-center gap-1 h-5 px-1.5 text-[10px] font-mono rounded-md border transition-colors hover:enabled:bg-[var(--bg-2)] disabled:cursor-default
			{isError ? 'border-[var(--color-error)]/30 text-[var(--color-error)]' :
			 phase === 'start' ? 'border-[var(--cairn-accent)]/30 text-[var(--cairn-accent)]' :
			 'border-[var(--border-subtle)] text-[var(--text-tertiary)]'}"
		onclick={() => { expanded = !expanded; }}
		disabled={!hasDetails}
		type="button"
	>
		{#if isError}
			<AlertCircle class="h-2.5 w-2.5" />
		{:else if phase === 'start'}
			<Wrench class="h-2.5 w-2.5 animate-spin" />
		{:else}
			<Check class="h-2.5 w-2.5" />
		{/if}
		{toolName}
		{#if isExternal}
			<span class="text-[8px] text-[var(--text-tertiary)] bg-[var(--bg-3)] px-1 rounded">mcp</span>
		{/if}
		{#if durationMs !== undefined && isDone}
			<span class="text-[var(--text-tertiary)] ml-0.5">{formatDuration(durationMs)}</span>
		{/if}
		{#if hasDetails && isDone}
			{#if expanded}
				<ChevronUp class="h-2.5 w-2.5 ml-0.5" />
			{:else}
				<ChevronDown class="h-2.5 w-2.5 ml-0.5" />
			{/if}
		{/if}
	</button>

	{#if expanded && hasDetails}
		<div class="mt-1 rounded-md border border-[var(--border-subtle)] bg-[var(--bg-2)] p-2 text-[11px] font-mono max-w-md overflow-hidden">
			{#if args && Object.keys(args).length > 0}
				<div class="mb-1.5">
					<span class="text-[var(--text-tertiary)] text-[9px] uppercase tracking-wider">Args</span>
					<pre class="mt-0.5 whitespace-pre-wrap break-all text-[var(--text-secondary)] max-h-24 overflow-y-auto">{truncate(JSON.stringify(args, null, 2), 500)}</pre>
				</div>
			{/if}
			{#if error}
				<div>
					<span class="text-[var(--color-error)] text-[9px] uppercase tracking-wider">Error</span>
					<pre class="mt-0.5 whitespace-pre-wrap break-all text-[var(--color-error)]/80 max-h-32 overflow-y-auto">{truncate(error, 1000)}</pre>
				</div>
			{:else if result}
				<div>
					<span class="text-[var(--text-tertiary)] text-[9px] uppercase tracking-wider">Result</span>
					<pre class="mt-0.5 whitespace-pre-wrap break-all text-[var(--text-secondary)] max-h-32 overflow-y-auto">{truncate(resultParts.text, 1000)}</pre>
					{#if resultParts.diff}
						<button
							class="mt-1 text-[9px] font-sans text-[var(--cairn-accent)] hover:underline"
							onclick={() => showDiff = !showDiff}
							type="button"
						>
							{showDiff ? 'hide diff' : 'show diff'}
						</button>
						{#if showDiff}
							<pre class="mt-1 whitespace-pre-wrap text-[10px] max-h-48 overflow-y-auto leading-tight" style="color: var(--text-secondary)">{@html formatDiff(resultParts.diff)}</pre>
						{/if}
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>
