<script lang="ts">
	import { onMount } from 'svelte';
	import { getSoul, updateSoul, getSoulHistory } from '$lib/api/client';
	import type { SoulHistoryEntry } from '$lib/types';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { relativeTime } from '$lib/utils/time';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Save, History, Eye, Edit3, GitCommit } from '@lucide/svelte';

	let content = $state('');
	let sha = $state<string | null>(null);
	let editing = $state(false);
	let history = $state<SoulHistoryEntry[]>([]);
	let showHistory = $state(false);
	let saving = $state(false);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await getSoul();
			content = res.content;
			sha = res.sha ?? null;
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
		} catch {
			// handled
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
			} catch {
				// handled
			}
		}
	}
</script>

<div class="mx-auto max-w-4xl p-6">
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Soul</h1>
			<p class="mt-1 text-xs text-[var(--text-tertiary)]">The agent's identity, personality, and behavioral guidelines</p>
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
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-6">
			<div class="prose prose-sm prose-invert max-w-none text-[var(--text-primary)] [&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-medium [&_code]:text-[var(--cairn-accent)] [&_code]:bg-[var(--bg-2)] [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_code]:text-xs">
				{@html renderMarkdown(content)}
			</div>
		</div>
	{/if}

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
