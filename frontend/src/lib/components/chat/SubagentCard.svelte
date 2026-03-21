<script lang="ts">
	import { Search, Code, Eye, Terminal, Loader2, Check, X, ChevronDown, ChevronUp, Square } from '@lucide/svelte';
	import type { SubagentInfo } from '$lib/types';

	let {
		subagent,
		onCancel,
	}: {
		subagent: SubagentInfo;
		onCancel?: () => void;
	} = $props();

	let expanded = $state(false);

	const isRunning = $derived(subagent.status === 'running');
	const isCompleted = $derived(subagent.status === 'completed');
	const isFailed = $derived(subagent.status === 'failed' || subagent.status === 'canceled');

	const typeIcons: Record<string, typeof Search> = {
		researcher: Search,
		coder: Code,
		reviewer: Eye,
		executor: Terminal,
	};
	const TypeIcon = $derived(typeIcons[subagent.type] ?? Search);

	const typeLabels: Record<string, string> = {
		researcher: 'Researcher',
		coder: 'Coder',
		reviewer: 'Reviewer',
		executor: 'Executor',
	};
	const typeLabel = $derived(typeLabels[subagent.type] ?? subagent.type);

	const borderColor = $derived(
		isRunning ? 'border-[var(--cairn-accent)]/40' :
		isCompleted ? 'border-[var(--color-success)]/40' :
		'border-[var(--color-error)]/40'
	);

	function formatDuration(ms?: number): string {
		if (!ms) return '';
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(1)}s`;
	}

	function truncate(text: string, max: number): string {
		if (text.length <= max) return text;
		return text.slice(0, max) + '...';
	}

	// Live elapsed timer for running subagents
	let elapsed = $state(0);
	let timerHandle: ReturnType<typeof setInterval> | null = null;

	$effect(() => {
		if (isRunning) {
			const start = new Date(subagent.createdAt).getTime();
			elapsed = Math.floor((Date.now() - start) / 1000);
			timerHandle = setInterval(() => {
				elapsed = Math.floor((Date.now() - start) / 1000);
			}, 1000);
		} else {
			if (timerHandle) clearInterval(timerHandle);
		}
		return () => {
			if (timerHandle) clearInterval(timerHandle);
		};
	});

	const elapsedStr = $derived(
		subagent.durationMs ? formatDuration(subagent.durationMs) : `${elapsed}s`
	);
</script>

<div class="w-full rounded-lg border {borderColor} bg-[var(--bg-1)] overflow-hidden transition-colors">
	<!-- Header row -->
	<div class="flex items-center gap-2 px-3 py-2 min-h-[44px]">
		<!-- Expand toggle (everything except cancel) -->
		<button
			class="flex-1 flex items-center gap-2 text-left hover:bg-[var(--bg-2)] rounded transition-colors min-w-0"
			onclick={() => { expanded = !expanded; }}
			type="button"
		>
			<!-- Status icon -->
			<div class="flex-shrink-0 flex h-6 w-6 items-center justify-center rounded-md
				{isRunning ? 'bg-[var(--cairn-accent)]/15 text-[var(--cairn-accent)]' :
				 isCompleted ? 'bg-[var(--color-success)]/15 text-[var(--color-success)]' :
				 'bg-[var(--color-error)]/15 text-[var(--color-error)]'}">
				{#if isRunning}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else if isCompleted}
					<Check class="h-3.5 w-3.5" />
				{:else}
					<X class="h-3.5 w-3.5" />
				{/if}
			</div>

			<!-- Type icon + progress -->
			<TypeIcon class="h-3 w-3 text-[var(--text-secondary)] flex-shrink-0" />
			<span class="text-xs font-medium text-[var(--text-primary)]">{typeLabel}</span>
			{#if subagent.round != null}
				<span class="text-[10px] text-[var(--text-tertiary)] font-mono">
					{subagent.round}{#if subagent.maxRounds}/{subagent.maxRounds}{/if}
				</span>
			{/if}
			{#if subagent.toolName && isRunning}
				<span class="text-[10px] text-[var(--text-tertiary)] font-mono truncate hidden sm:inline">
					{subagent.toolName}
				</span>
			{/if}

			<!-- Elapsed + chevron -->
			<span class="ml-auto text-[10px] text-[var(--text-tertiary)] font-mono flex-shrink-0">{elapsedStr}</span>
			{#if expanded}
				<ChevronUp class="h-3 w-3 text-[var(--text-tertiary)] flex-shrink-0" />
			{:else}
				<ChevronDown class="h-3 w-3 text-[var(--text-tertiary)] flex-shrink-0" />
			{/if}
		</button>

		<!-- Cancel (separate from toggle to avoid nested buttons) -->
		{#if isRunning && onCancel}
			<button
				type="button"
				class="flex h-7 w-7 items-center justify-center rounded hover:bg-[var(--color-error)]/15 text-[var(--text-tertiary)] hover:text-[var(--color-error)] flex-shrink-0"
				onclick={() => onCancel?.()}
				title="Cancel subagent"
				aria-label="Cancel subagent"
			>
				<Square class="h-3 w-3" />
			</button>
		{/if}
	</div>

	<!-- Body (collapsed on mobile by default, expanded on desktop if completed) -->
	{#if expanded}
		<div class="border-t border-[var(--border-subtle)] px-3 py-2 space-y-1.5">
			<!-- Instruction -->
			<p class="text-[11px] text-[var(--text-secondary)] leading-relaxed">
				{truncate(subagent.instruction, 300)}
			</p>

			<!-- Summary (completed) -->
			{#if subagent.summary}
				<div class="rounded-md bg-[var(--bg-2)] p-2">
					<span class="text-[9px] uppercase tracking-wider text-[var(--text-tertiary)]">Result</span>
					<p class="mt-0.5 text-[11px] text-[var(--text-primary)] whitespace-pre-wrap leading-relaxed">
						{truncate(subagent.summary, 1000)}
					</p>
				</div>
			{/if}

			<!-- Error -->
			{#if subagent.error}
				<div class="rounded-md bg-[var(--color-error)]/5 p-2">
					<span class="text-[9px] uppercase tracking-wider text-[var(--color-error)]">Error</span>
					<p class="mt-0.5 text-[11px] text-[var(--color-error)]/80 whitespace-pre-wrap">
						{subagent.error}
					</p>
				</div>
			{/if}

			<!-- Stats -->
			{#if subagent.toolCalls || subagent.round}
				<div class="flex gap-3 text-[10px] text-[var(--text-tertiary)]">
					{#if subagent.round}
						<span>{subagent.round} round{subagent.round !== 1 ? 's' : ''}</span>
					{/if}
					{#if subagent.toolCalls}
						<span>{subagent.toolCalls} tool call{subagent.toolCalls !== 1 ? 's' : ''}</span>
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>
