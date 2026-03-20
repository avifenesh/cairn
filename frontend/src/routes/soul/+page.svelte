<script lang="ts">
	import { onMount } from 'svelte';
	import { getSoul, updateSoul, getSoulHistory, getSoulPatch, approveSoulPatch, denySoulPatch } from '$lib/api/client';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { relativeTime } from '$lib/utils/time';
	import type { SoulHistoryEntry, SoulPatch } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Save, History, Eye, Edit3, GitCommit, Check, X, AlertTriangle, Plus, Loader2 } from '@lucide/svelte';

	let content = $state('');
	let sha = $state<string | null>(null);
	let editing = $state(false);
	let history = $state<SoulHistoryEntry[]>([]);
	let showHistory = $state(false);
	let saving = $state(false);
	let loading = $state(true);

	// Patch review state
	let patch = $state<SoulPatch | null>(null);
	let denyMode = $state(false);
	let denyReason = $state('');
	let patchActing = $state(false);

	onMount(async () => {
		try {
			const [soulRes, patchRes] = await Promise.all([
				getSoul(),
				getSoulPatch().catch(() => ({ patch: null })),
			]);
			content = soulRes.content;
			sha = soulRes.sha ?? null;
			patch = patchRes.patch ?? null;
		} catch {
			// handled
		} finally {
			loading = false;
		}
	});

	async function handleSave() {
		saving = true;
		try {
			const res = await updateSoul(content);
			sha = res.sha;
			editing = false;
		} catch (e) {
			console.error('Failed to save soul:', e);
		} finally {
			saving = false;
		}
	}

	async function loadHistory() {
		showHistory = !showHistory;
		if (showHistory && history.length === 0) {
			try {
				const res = await getSoulHistory();
				history = res.items;
			} catch (e) {
				console.error('Failed to load history:', e);
			}
		}
	}

	async function handleApprove() {
		if (!patch) return;
		patchActing = true;
		try {
			await approveSoulPatch(patch.id);
			// Reload soul content
			const res = await getSoul();
			content = res.content;
			sha = res.sha ?? null;
			patch = null;
			denyMode = false;
		} catch (e) {
			console.error('Failed to approve patch:', e);
		} finally {
			patchActing = false;
		}
	}

	async function handleDeny() {
		if (!patch || !denyReason.trim()) return;
		patchActing = true;
		try {
			await denySoulPatch(patch.id, denyReason.trim());
			patch = null;
			denyMode = false;
			denyReason = '';
		} catch (e) {
			console.error('Failed to deny patch:', e);
		} finally {
			patchActing = false;
		}
	}

	// Extract h2 headings for table of contents
	const toc = $derived(() => {
		const headings: { text: string; id: string }[] = [];
		const lines = content.split('\n');
		for (const line of lines) {
			const match = line.match(/^##\s+(.+)/);
			if (match) {
				const text = match[1].trim();
				const id = text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
				headings.push({ text, id });
			}
		}
		return headings;
	});
</script>

<div class="mx-auto max-w-4xl p-6">
	<!-- Header -->
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Soul</h1>
			<p class="mt-1 text-xs text-[var(--text-tertiary)]">Identity, personality, and behavioral guidelines</p>
		</div>
		<div class="flex gap-2">
			<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={loadHistory}>
				<History class="h-3.5 w-3.5" />
				History
			</Button>
			<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={() => (editing = !editing)}>
				{#if editing}
					<Eye class="h-3.5 w-3.5" /> Preview
				{:else}
					<Edit3 class="h-3.5 w-3.5" /> Edit
				{/if}
			</Button>
			{#if editing}
				<Button size="sm" class="h-8 text-xs gap-1.5" onclick={handleSave} disabled={saving}>
					<Save class="h-3.5 w-3.5" />
					{saving ? 'Saving...' : 'Save'}
				</Button>
			{/if}
		</div>
	</div>

	<!-- Pending Patch Banner -->
	{#if patch}
		<div class="mb-6 rounded-lg border-2 border-[var(--color-warning)]/40 bg-[var(--color-warning)]/5 overflow-hidden animate-in">
			<div class="px-4 py-3 flex items-center gap-3 border-b border-[var(--color-warning)]/20">
				<AlertTriangle class="h-4 w-4 text-[var(--color-warning)] flex-shrink-0" />
				<div class="flex-1 min-w-0">
					<p class="text-sm font-medium text-[var(--text-primary)]">Soul patch proposed</p>
					<p class="text-[11px] text-[var(--text-tertiary)]">
						From {patch.source} - {relativeTime(patch.createdAt)}
					</p>
				</div>
				<Badge variant="outline" class="text-[10px] text-[var(--color-warning)]">pending review</Badge>
			</div>

			<!-- Diff: proposed addition -->
			<div class="px-4 py-3">
				<p class="text-[10px] uppercase tracking-wider text-[var(--text-tertiary)] mb-2 flex items-center gap-1">
					<Plus class="h-3 w-3" /> Proposed addition
				</p>
				<div class="rounded-md border border-[var(--color-success)]/30 bg-[var(--color-success)]/5 p-3">
					<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">
						{@html renderMarkdown(patch.content)}
					</div>
				</div>
			</div>

			<!-- Actions -->
			<div class="px-4 py-3 border-t border-[var(--color-warning)]/20 bg-[var(--bg-1)]/50">
				{#if denyMode}
					<div class="space-y-2">
						<p class="text-xs text-[var(--text-secondary)]">Why are you denying this patch?</p>
						<textarea
							bind:value={denyReason}
							placeholder="Your reason will be saved to cairn's memory..."
							class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/50 focus:border-[var(--cairn-accent)] focus:outline-none resize-none h-20"
						></textarea>
						<div class="flex gap-2 justify-end">
							<Button variant="outline" size="sm" class="h-7 text-xs" onclick={() => { denyMode = false; denyReason = ''; }}>
								Cancel
							</Button>
							<Button size="sm" class="h-7 text-xs gap-1 bg-[var(--color-error)] hover:bg-[var(--color-error)]/90" onclick={handleDeny} disabled={patchActing || !denyReason.trim()}>
								{#if patchActing}<Loader2 class="h-3 w-3 animate-spin" />{:else}<X class="h-3 w-3" />{/if}
								Deny with reason
							</Button>
						</div>
					</div>
				{:else}
					<div class="flex gap-2 justify-end">
						<Button variant="outline" size="sm" class="h-7 text-xs gap-1 text-[var(--color-error)]" onclick={() => denyMode = true}>
							<X class="h-3 w-3" /> Deny
						</Button>
						<Button size="sm" class="h-7 text-xs gap-1" onclick={handleApprove} disabled={patchActing}>
							{#if patchActing}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Check class="h-3 w-3" />{/if}
							Approve & apply
						</Button>
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-6 space-y-3">
			<Skeleton class="h-6 w-48" />
			<Skeleton class="h-4 w-full" />
			<Skeleton class="h-4 w-3/4" />
			<Skeleton class="h-4 w-5/6" />
		</div>
	{:else if editing}
		<textarea
			bind:value={content}
			class="h-[calc(100vh-220px)] w-full resize-none rounded-lg border border-border-subtle bg-[var(--bg-1)] p-5 font-mono text-sm text-[var(--text-primary)] leading-relaxed focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none transition-colors"
			spellcheck="false"
		></textarea>
	{:else}
		<div class="flex gap-6">
			<!-- Table of Contents (desktop sidebar) -->
			{#if toc().length > 2}
				<nav class="hidden lg:block w-48 flex-shrink-0 sticky top-6 self-start">
					<p class="text-[10px] uppercase tracking-wider text-[var(--text-tertiary)] mb-2 font-medium">Contents</p>
					<ul class="space-y-1 border-l border-border-subtle pl-3">
						{#each toc() as heading}
							<li>
								<a
									href="#{heading.id}"
									class="text-xs text-[var(--text-tertiary)] hover:text-[var(--cairn-accent)] transition-colors block py-0.5 truncate"
								>
									{heading.text}
								</a>
							</li>
						{/each}
					</ul>
				</nav>
			{/if}

			<!-- Soul content -->
			<div class="flex-1 min-w-0 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-6">
				<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">
					{@html renderMarkdown(content)}
				</div>
			</div>
		</div>
	{/if}

	<!-- History -->
	{#if showHistory && history.length > 0}
		<Separator class="my-6" />
		<h2 class="mb-3 text-sm font-medium text-[var(--text-primary)]">Revision History</h2>
		<div class="flex flex-col gap-1">
			{#each history as entry, i (entry.sha)}
				<div class="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-[var(--bg-1)] transition-colors animate-in" style="animation-delay: {i * 30}ms">
					<GitCommit class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
					<code class="text-[11px] font-mono text-[var(--cairn-accent)]">{entry.sha.slice(0, 7)}</code>
					<p class="flex-1 text-sm text-[var(--text-primary)] truncate">{entry.message}</p>
					<time class="text-[11px] text-[var(--text-tertiary)] tabular-nums font-mono" datetime={entry.date}>{relativeTime(entry.date)}</time>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.cairn-prose :global(p) { margin: 0.5em 0; }
	.cairn-prose :global(p:first-child) { margin-top: 0; }
	.cairn-prose :global(strong) { color: var(--text-primary); font-weight: 600; }
	.cairn-prose :global(ul), .cairn-prose :global(ol) { padding-left: 1.25em; margin: 0.5em 0; }
	.cairn-prose :global(li) { margin: 0.2em 0; }
	.cairn-prose :global(code) {
		background: var(--bg-2); color: var(--cairn-accent);
		padding: 0.15em 0.4em; border-radius: 4px; font-size: 0.85em;
		font-family: 'Geist Mono', monospace;
	}
	.cairn-prose :global(pre) {
		background: var(--bg-2); border: 1px solid var(--border-subtle);
		border-radius: 8px; padding: 0.75em 1em; overflow-x: auto; margin: 0.75em 0;
	}
	.cairn-prose :global(pre code) { background: none; color: var(--text-primary); padding: 0; }
	.cairn-prose :global(h1) {
		font-size: 1.5em; font-weight: 700; margin: 1em 0 0.5em; color: var(--text-primary);
		border-bottom: 1px solid var(--border-subtle); padding-bottom: 0.3em;
	}
	.cairn-prose :global(h2) {
		font-size: 1.2em; font-weight: 600; margin: 1.2em 0 0.4em; color: var(--text-primary);
		scroll-margin-top: 2rem;
	}
	.cairn-prose :global(h3) {
		font-size: 1.05em; font-weight: 600; margin: 0.8em 0 0.3em; color: var(--text-primary);
	}
	.cairn-prose :global(blockquote) {
		border-left: 3px solid var(--cairn-accent); padding-left: 1em; margin: 0.75em 0;
		color: var(--text-secondary); font-style: italic;
	}
	.cairn-prose :global(hr) {
		border: none; border-top: 1px solid var(--border-subtle); margin: 1.5em 0;
	}
	.cairn-prose :global(table) {
		width: 100%; border-collapse: collapse; margin: 0.75em 0; font-size: 0.9em;
	}
	.cairn-prose :global(th), .cairn-prose :global(td) {
		border: 1px solid var(--border-subtle); padding: 0.4em 0.6em; text-align: left;
	}
	.cairn-prose :global(th) {
		background: var(--bg-2); font-weight: 600; color: var(--text-primary);
	}
	.cairn-prose :global(a) {
		color: var(--cairn-accent); text-decoration: none;
	}
	.cairn-prose :global(a:hover) { text-decoration: underline; }
</style>
