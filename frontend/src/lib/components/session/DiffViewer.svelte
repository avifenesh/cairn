<script lang="ts">
	import type { FileChange } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { FileText, FilePlus, FileX } from '@lucide/svelte';

	let { files = [], selectedFile = $bindable<string | null>(null) }: {
		files: FileChange[];
		selectedFile?: string | null;
	} = $props();

	let diffHtml = $state('');

	$effect(() => {
		if (selectedFile) {
			const file = files.find((f) => f.path === selectedFile);
			if (file?.diff) {
				renderDiff(file.diff);
			} else {
				diffHtml = '';
			}
		} else {
			diffHtml = '';
		}
	});

	function escapeHtml(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
	}

	async function renderDiff(diff: string) {
		try {
			const { html } = await import('diff2html');
			diffHtml = html(diff, {
				drawFileList: false,
				outputFormat: 'line-by-line',
				matching: 'lines',
			});
		} catch {
			// Escape HTML to prevent XSS in fallback rendering.
			diffHtml = `<pre class="text-xs">${escapeHtml(diff)}</pre>`;
		}
	}

	const OP_BADGE: Record<string, { variant: 'default' | 'secondary' | 'destructive'; label: string }> = {
		write: { variant: 'secondary', label: 'M' },
		delete: { variant: 'destructive', label: 'D' },
		rename: { variant: 'default', label: 'R' },
	};
</script>

<div class="diff-viewer">
	{#if files.length === 0}
		<div class="empty-state">
			<FileText size={24} class="text-muted-foreground" />
			<p class="text-sm text-muted-foreground mt-2">No file changes yet</p>
		</div>
	{:else}
		<!-- File tree -->
		<div class="file-tree">
			{#each files as file}
				<button
					class="file-entry"
					class:active={selectedFile === file.path}
					onclick={() => (selectedFile = file.path)}
				>
					{#if file.operation === 'delete'}
						<FileX size={14} class="text-destructive" />
					{:else if file.operation === 'write' && !file.diff}
						<FilePlus size={14} class="text-green-500" />
					{:else}
						<FileText size={14} />
					{/if}
					<span class="file-path">{file.path}</span>
					<Badge variant={OP_BADGE[file.operation]?.variant ?? 'secondary'} class="text-xs ml-auto">
						{OP_BADGE[file.operation]?.label ?? file.operation}
					</Badge>
				</button>
			{/each}
		</div>

		<!-- Diff pane -->
		{#if selectedFile && diffHtml}
			<div class="diff-pane">
				{@html diffHtml}
			</div>
		{:else if selectedFile}
			<div class="empty-state">
				<p class="text-sm text-muted-foreground">No diff available for this file</p>
			</div>
		{/if}
	{/if}
</div>

<style>
	.diff-viewer {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}
	.file-tree {
		border-bottom: 1px solid hsl(var(--border));
		max-height: 40%;
		overflow-y: auto;
		padding: 0.25rem;
	}
	.file-entry {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.25rem 0.5rem;
		border-radius: 0.25rem;
		width: 100%;
		background: none;
		border: none;
		cursor: pointer;
		font-size: 0.75rem;
		color: inherit;
		text-align: left;
	}
	.file-entry:hover {
		background: hsl(var(--muted) / 0.5);
	}
	.file-entry.active {
		background: hsl(var(--muted));
	}
	.file-path {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: var(--font-mono, monospace);
	}
	.diff-pane {
		flex: 1;
		overflow: auto;
		padding: 0.5rem;
		font-size: 0.75rem;
	}
	.diff-pane :global(.d2h-wrapper) {
		font-size: 0.75rem;
	}
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: 2rem;
	}
</style>
