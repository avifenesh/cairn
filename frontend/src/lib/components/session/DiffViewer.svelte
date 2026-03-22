<script lang="ts">
	import 'diff2html/bundles/css/diff2html.min.css';
	import type { FileChange } from '$lib/types';
	import { FileText, FilePlus, FileX, Columns2, AlignJustify } from '@lucide/svelte';

	let { files = [], selectedFile = $bindable<string | null>(null) }: {
		files: FileChange[];
		selectedFile?: string | null;
	} = $props();

	let diffEl: HTMLElement | undefined = $state();
	let viewMode = $state<'line-by-line' | 'side-by-side'>('line-by-line');

	// Parse all file diffs to extract line stats.
	interface FileStats {
		added: number;
		deleted: number;
	}
	const fileStats = $derived.by((): Map<string, FileStats> => {
		const map = new Map<string, FileStats>();
		for (const f of files) {
			if (!f.diff) {
				map.set(f.path, { added: 0, deleted: 0 });
				continue;
			}
			let added = 0, deleted = 0;
			for (const line of f.diff.split('\n')) {
				if (line.startsWith('+') && !line.startsWith('+++')) added++;
				else if (line.startsWith('-') && !line.startsWith('---')) deleted++;
			}
			map.set(f.path, { added, deleted });
		}
		return map;
	});

	const totalAdded = $derived(
		Array.from(fileStats.values()).reduce((sum, s) => sum + s.added, 0)
	);
	const totalDeleted = $derived(
		Array.from(fileStats.values()).reduce((sum, s) => sum + s.deleted, 0)
	);

	// Auto-select first file.
	$effect(() => {
		if (!selectedFile && files.length > 0) {
			selectedFile = files[0].path;
		}
	});

	// Render diff when selection or view mode changes.
	$effect(() => {
		const file = selectedFile ? files.find((f) => f.path === selectedFile) : null;
		const diff = file?.diff;
		const mode = viewMode;
		if (diff && diffEl) {
			renderDiff(diff, mode);
		} else if (diffEl) {
			// Clear previous diff by replacing children safely.
			while (diffEl.firstChild) diffEl.removeChild(diffEl.firstChild);
		}
	});

	async function renderDiff(diff: string, mode: 'line-by-line' | 'side-by-side') {
		if (!diffEl) return;
		try {
			const { Diff2HtmlUI } = await import('diff2html/lib/ui/js/diff2html-ui');
			const { ColorSchemeType } = await import('diff2html/lib/types');
			const isDark = !document.documentElement.getAttribute('data-theme')?.includes('light');
			const ui = new Diff2HtmlUI(diffEl, diff, {
				outputFormat: mode,
				drawFileList: false,
				matching: 'lines',
				diffStyle: 'word',
				colorScheme: isDark ? ColorSchemeType.DARK : ColorSchemeType.LIGHT,
				synchronisedScroll: true,
				highlight: true,
				fileContentToggle: false,
				stickyFileHeaders: true,
				renderNothingWhenEmpty: false,
				maxLineSizeInBlockForComparison: 200,
			});
			ui.draw();
			ui.highlightCode();
			if (mode === 'side-by-side') {
				ui.synchronisedScroll();
			}
		} catch {
			// Fallback: render escaped plain text diff.
			if (diffEl) {
				while (diffEl.firstChild) diffEl.removeChild(diffEl.firstChild);
				const pre = document.createElement('pre');
				pre.className = 'diff-fallback';
				pre.textContent = diff;
				diffEl.appendChild(pre);
			}
		}
	}

	const OP_ICON: Record<string, { component: typeof FileText; colorClass: string }> = {
		write: { component: FileText, colorClass: 'text-[#fbbf24]' },
		delete: { component: FileX, colorClass: 'text-[#f87171]' },
	};

	function getIcon(file: FileChange) {
		if (file.operation === 'delete') return OP_ICON.delete;
		if (file.operation === 'write' && !file.diff) return { component: FilePlus, colorClass: 'text-[#34d399]' };
		return OP_ICON.write;
	}

	function basename(path: string): string {
		const parts = path.split('/');
		return parts[parts.length - 1];
	}

	function dirname(path: string): string {
		const parts = path.split('/');
		if (parts.length <= 1) return '';
		return parts.slice(0, -1).join('/') + '/';
	}
</script>

<div class="diff-viewer">
	{#if files.length === 0}
		<div class="empty-state">
			<FileText size={24} class="text-muted-foreground" />
			<p class="text-sm text-muted-foreground mt-2">No file changes yet</p>
		</div>
	{:else}
		<!-- Summary bar -->
		<div class="diff-summary">
			<span class="summary-files">{files.length} file{files.length !== 1 ? 's' : ''} changed</span>
			{#if totalAdded > 0}<span class="summary-added">+{totalAdded}</span>{/if}
			{#if totalDeleted > 0}<span class="summary-deleted">-{totalDeleted}</span>{/if}
			<div class="summary-controls">
				<button
					class="view-btn" class:active={viewMode === 'line-by-line'}
					onclick={() => (viewMode = 'line-by-line')}
					aria-label="Unified view"
				><AlignJustify size={12} /></button>
				<button
					class="view-btn" class:active={viewMode === 'side-by-side'}
					onclick={() => (viewMode = 'side-by-side')}
					aria-label="Split view"
				><Columns2 size={12} /></button>
			</div>
		</div>

		<!-- File tree -->
		<div class="file-tree">
			{#each files as file (file.path)}
				{@const stats = fileStats.get(file.path)}
				{@const icon = getIcon(file)}
				{@const IconComponent = icon.component}
				<button
					class="file-entry"
					class:active={selectedFile === file.path}
					onclick={() => (selectedFile = file.path)}
				>
					<IconComponent size={13} class={icon.colorClass} />
					<span class="file-dir">{dirname(file.path)}</span><span class="file-name">{basename(file.path)}</span>
					<span class="file-stats">
						{#if stats && stats.added > 0}<span class="stat-add">+{stats.added}</span>{/if}
						{#if stats && stats.deleted > 0}<span class="stat-del">-{stats.deleted}</span>{/if}
					</span>
				</button>
			{/each}
		</div>

		<!-- Diff pane -->
		{#if selectedFile}
			{@const file = files.find((f) => f.path === selectedFile)}
			{#if file?.diff}
				<div class="diff-pane" bind:this={diffEl}></div>
			{:else}
				<div class="empty-state">
					<p class="text-sm text-muted-foreground">
						{file?.operation === 'delete' ? 'File deleted' : 'No diff available'}
					</p>
				</div>
			{/if}
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

	/* Summary bar */
	.diff-summary {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0.625rem;
		border-bottom: 1px solid var(--border-subtle);
		font-size: 0.6875rem;
		color: var(--text-secondary);
		flex-shrink: 0;
	}
	.summary-files { font-weight: 500; }
	.summary-added { color: #34d399; font-weight: 500; font-variant-numeric: tabular-nums; }
	.summary-deleted { color: #f87171; font-weight: 500; font-variant-numeric: tabular-nums; }
	.summary-controls {
		display: flex; gap: 0.125rem; margin-left: auto;
		border: 1px solid var(--border-subtle);
		border-radius: 0.25rem;
		overflow: hidden;
	}
	.view-btn {
		display: flex; align-items: center; justify-content: center;
		padding: 0.25rem 0.375rem;
		background: none; border: none; cursor: pointer;
		color: var(--text-tertiary);
		transition: all 0.15s;
	}
	.view-btn:hover { color: var(--text-primary); background: var(--bg-2); }
	.view-btn.active {
		color: var(--cairn-accent);
		background: var(--accent-dim);
	}

	/* File tree */
	.file-tree {
		border-bottom: 1px solid var(--border-subtle);
		max-height: 35%;
		overflow-y: auto;
		padding: 0.25rem;
		flex-shrink: 0;
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
		font-size: 0.6875rem;
		color: inherit;
		text-align: left;
		transition: background 0.1s;
	}
	.file-entry:hover {
		background: var(--bg-2);
	}
	.file-entry.active {
		background: var(--accent-dim);
		border-left: 2px solid var(--cairn-accent);
	}
	.file-dir {
		color: var(--text-tertiary);
		font-family: var(--font-mono, monospace);
		flex-shrink: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.file-name {
		font-family: var(--font-mono, monospace);
		font-weight: 500;
		color: var(--text-primary);
		white-space: nowrap;
	}
	.file-stats {
		margin-left: auto;
		display: flex;
		gap: 0.375rem;
		font-family: var(--font-mono, monospace);
		font-variant-numeric: tabular-nums;
		flex-shrink: 0;
	}
	.stat-add { color: #34d399; }
	.stat-del { color: #f87171; }

	/* Diff pane */
	.diff-pane {
		flex: 1;
		overflow: auto;
		padding: 0.25rem;
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: 2rem;
	}

	/* Fallback for failed diff2html load */
	.diff-pane :global(.diff-fallback) {
		font-size: 0.75rem;
		font-family: var(--font-mono, monospace);
		white-space: pre-wrap;
		word-break: break-all;
		line-height: 1.4;
		color: var(--text-secondary);
		padding: 0.5rem;
	}
</style>
