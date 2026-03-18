<script lang="ts">
	import type { Task } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { CheckCircle, XCircle, Clock, Ban, Loader2, MessageSquare, ChevronDown, ChevronUp, Trash2, ExternalLink } from '@lucide/svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';

	let {
		task,
		oncancel,
		ondelete,
	}: {
		task: Task;
		oncancel?: (id: string) => void;
		ondelete?: (id: string) => void;
	} = $props();

	let expanded = $state(false);

	const icons: Record<string, typeof CheckCircle> = {
		completed: CheckCircle,
		failed: XCircle,
		pending: Clock,
		running: Loader2,
		cancelled: Ban,
	};

	const colors: Record<string, string> = {
		completed: 'var(--color-success)',
		failed: 'var(--color-error)',
		pending: 'var(--color-warning)',
		running: 'var(--cairn-accent)',
		cancelled: 'var(--text-tertiary)',
	};

	const Icon = $derived(icons[task.status] ?? Clock);
	const isChat = $derived(task.type === 'chat' && !!task.sessionId);
	const isDone = $derived(task.status === 'completed' || task.status === 'failed' || task.status === 'cancelled');
	const hasResult = $derived(!!task.result || !!task.error);
</script>

<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] card-hover overflow-hidden">
	<!-- Main row -->
	<div class="flex items-center gap-3 p-3">
		<div class="flex items-center justify-center h-8 w-8 rounded-md bg-[var(--bg-2)] flex-shrink-0">
			<Icon
				class="h-4 w-4 {task.status === 'running' ? 'animate-spin' : ''}"
				style="color: {colors[task.status]}"
			/>
		</div>
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium text-[var(--text-primary)]">{task.title}</p>
			<div class="mt-0.5 flex items-center gap-1.5 flex-wrap">
				<Badge variant="outline" class="h-4 px-1 text-[10px]" style="color: {colors[task.status]}">
					{task.status}
				</Badge>
				{#if task.type === 'a2a'}
					<Badge variant="outline" class="h-4 px-1 text-[10px] text-[var(--cairn-accent)]">a2a</Badge>
				{/if}
				{#if isChat}
					<span class="flex items-center gap-0.5 text-[10px] text-[var(--text-tertiary)]">
						<MessageSquare class="h-2.5 w-2.5" />
						{task.mode ?? 'chat'}
					</span>
				{:else}
					<span class="text-[10px] text-[var(--text-tertiary)]">{task.type}</span>
				{/if}
				<span class="text-[10px] text-[var(--text-tertiary)]">&middot;</span>
				<time class="text-[10px] text-[var(--text-tertiary)]" datetime={task.createdAt}>{relativeTime(task.createdAt)}</time>
			</div>
		</div>
		<div class="flex items-center gap-1 flex-shrink-0">
			{#if isChat}
				<a
					href="/chat?session={task.sessionId}"
					class="h-7 px-2 rounded-md text-[10px] text-[var(--text-tertiary)] hover:text-[var(--cairn-accent)] hover:bg-[var(--bg-2)] flex items-center gap-1 transition-colors"
					title="Open chat"
				>
					<ExternalLink class="h-3 w-3" /> Chat
				</a>
			{/if}
			{#if hasResult || task.error}
				<button
					class="h-7 w-7 rounded-md text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-2)] flex items-center justify-center transition-colors"
					onclick={() => { expanded = !expanded; }}
					title={expanded ? 'Collapse' : 'Show details'}
					type="button"
				>
					{#if expanded}
						<ChevronUp class="h-3.5 w-3.5" />
					{:else}
						<ChevronDown class="h-3.5 w-3.5" />
					{/if}
				</button>
			{/if}
			{#if oncancel && (task.status === 'running' || task.status === 'pending')}
				<Button
					variant="ghost"
					size="sm"
					class="h-7 text-[10px] text-[var(--text-tertiary)] hover:text-[var(--color-error)]"
					onclick={() => oncancel?.(task.id)}
				>
					Cancel
				</Button>
			{/if}
			{#if ondelete && isDone}
				<button
					class="h-7 w-7 rounded-md text-[var(--text-tertiary)] hover:text-[var(--color-error)] hover:bg-[var(--bg-2)] flex items-center justify-center transition-colors"
					onclick={() => ondelete?.(task.id)}
					title="Delete task"
					type="button"
				>
					<Trash2 class="h-3 w-3" />
				</button>
			{/if}
		</div>
	</div>

	<!-- Expandable details -->
	{#if expanded}
		<div class="border-t border-border-subtle px-4 py-2.5 text-xs space-y-2">
			{#if task.error}
				<div>
					<span class="text-[9px] text-[var(--color-error)] uppercase tracking-wider">Error</span>
					<pre class="mt-0.5 whitespace-pre-wrap break-all text-[var(--color-error)]/80 font-mono max-h-24 overflow-y-auto">{task.error}</pre>
				</div>
			{/if}
			{#if task.result}
				<div>
					<span class="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider">Result</span>
					<pre class="mt-0.5 whitespace-pre-wrap break-all text-[var(--text-secondary)] font-mono max-h-32 overflow-y-auto">{task.result}</pre>
				</div>
			{/if}
			{#if task.input?.message}
				<div>
					<span class="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider">Input</span>
					<p class="mt-0.5 text-[var(--text-secondary)]">{task.input.message}</p>
				</div>
			{/if}
		</div>
	{/if}
</div>
