<script lang="ts">
	import { onMount } from 'svelte';
	import { getSoul, updateSoul, getSoulHistory } from '$lib/api/client';
	import type { SoulHistoryEntry } from '$lib/types';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { relativeTime } from '$lib/utils/time';
	import { Save, History, Eye, Edit3 } from '@lucide/svelte';

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
		<h1 class="text-2xl font-semibold text-[var(--text-primary)]">Soul</h1>
		<div class="flex gap-2">
			<button
				class="flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors"
				onclick={loadHistory}
			>
				<History class="h-3.5 w-3.5" />
				History
			</button>
			<button
				class="flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-3)] transition-colors"
				onclick={() => (editing = !editing)}
			>
				{#if editing}
					<Eye class="h-3.5 w-3.5" /> Preview
				{:else}
					<Edit3 class="h-3.5 w-3.5" /> Edit
				{/if}
			</button>
			{#if editing}
				<button
					class="flex items-center gap-1.5 rounded-md bg-[var(--pub-accent)] px-3 py-1.5 text-xs font-medium text-[var(--primary-foreground)] hover:opacity-90 transition-opacity disabled:opacity-50"
					onclick={handleSave}
					disabled={saving}
				>
					<Save class="h-3.5 w-3.5" />
					{saving ? 'Saving...' : 'Save'}
				</button>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="h-96 animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
	{:else if editing}
		<textarea
			bind:value={content}
			class="h-[calc(100vh-200px)] w-full resize-none rounded-lg border border-border-subtle bg-[var(--bg-2)] p-4 font-mono text-sm text-[var(--text-primary)] focus:border-[var(--pub-accent)] focus:outline-none"
		></textarea>
	{:else}
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-6">
			<div class="prose prose-sm prose-invert max-w-none text-[var(--text-primary)]">
				{@html renderMarkdown(content)}
			</div>
		</div>
	{/if}

	{#if showHistory && history.length > 0}
		<div class="mt-6">
			<h2 class="mb-3 text-lg font-medium text-[var(--text-primary)]">History</h2>
			<div class="flex flex-col gap-2">
				{#each history as entry (entry.sha)}
					<div class="flex items-center gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
						<code class="text-xs font-mono text-[var(--pub-accent)]">{entry.sha.slice(0, 7)}</code>
						<p class="flex-1 text-sm text-[var(--text-primary)]">{entry.message}</p>
						<span class="text-xs text-[var(--text-tertiary)]">{relativeTime(entry.date)}</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
