<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		getSoul, updateSoul, getSoulHistory, getSoulPatch, approveSoulPatch, denySoulPatch,
		getUserProfile, updateUserProfile,
		getAgentsConfig, updateAgentsConfig, getAgentsPatch, approveAgentsPatch, denyAgentsPatch,
		getMemoryFile, updateMemoryFile, getMemoryPatch, approveMemoryPatch, denyMemoryPatch,
	} from '$lib/api/client';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { relativeTime } from '$lib/utils/time';
	import type { SoulHistoryEntry, SoulPatch } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Save, History, Eye, Edit3, GitCommit, Check, X, AlertTriangle, Loader2, Heart, User, Shield, Brain } from '@lucide/svelte';
	import { page } from '$app/state';

	// --- Tab state ---
	let tab = $state(page.url.searchParams.get('tab') || 'soul');

	$effect(() => {
		const url = new URL(window.location.href);
		if (tab !== 'soul') {
			url.searchParams.set('tab', tab);
		} else {
			url.searchParams.delete('tab');
		}
		history.replaceState({}, '', url);
	});

	const TAB_INFO: Record<string, { description: string; icon: typeof Heart }> = {
		soul: { description: 'Who the agent IS. Identity, voice, values.', icon: Heart },
		user: { description: 'Who you ARE. Preferences, communication style, work patterns.', icon: User },
		agents: { description: 'How agents OPERATE. Rules, permissions, retry limits, PR discipline.', icon: Shield },
		memory: { description: 'Pinned essentials the agent must always know. Canonical identities, critical conventions.', icon: Brain },
	};

	// --- Per-tab state ---
	let soulContent = $state('');
	let soulSha = $state<string | null>(null);
	let soulEditing = $state(false);
	let soulSaving = $state(false);
	let soulLoading = $state(true);
	let soulPatch = $state<SoulPatch | null>(null);
	let soulHistory = $state<SoulHistoryEntry[]>([]);
	let showSoulHistory = $state(false);

	let userContent = $state('');
	let userEditing = $state(false);
	let userSaving = $state(false);
	let userLoading = $state(true);

	let agentsContent = $state('');
	let agentsEditing = $state(false);
	let agentsSaving = $state(false);
	let agentsLoading = $state(true);
	let agentsPatch = $state<SoulPatch | null>(null);

	let memoryContent = $state('');
	let memoryEditing = $state(false);
	let memorySaving = $state(false);
	let memoryLoading = $state(true);
	let memoryPatch = $state<SoulPatch | null>(null);

	// Patch review state (shared)
	let denyMode = $state(false);
	let denyReason = $state('');
	let patchActing = $state(false);

	// --- Load all on mount ---
	onMount(async () => {
		const [soulRes, soulPatchRes, userRes, agentsRes, agentsPatchRes, memRes, memPatchRes] = await Promise.allSettled([
			getSoul(),
			getSoulPatch().catch(() => ({ patch: null })),
			getUserProfile(),
			getAgentsConfig(),
			getAgentsPatch().catch(() => ({ patch: null })),
			getMemoryFile(),
			getMemoryPatch().catch(() => ({ patch: null })),
		]);
		if (soulRes.status === 'fulfilled') { soulContent = soulRes.value.content; soulSha = soulRes.value.sha ?? null; }
		if (soulPatchRes.status === 'fulfilled') soulPatch = soulPatchRes.value.patch ?? null;
		if (userRes.status === 'fulfilled') userContent = userRes.value.content;
		if (agentsRes.status === 'fulfilled') agentsContent = agentsRes.value.content;
		if (agentsPatchRes.status === 'fulfilled') agentsPatch = agentsPatchRes.value.patch ?? null;
		if (memRes.status === 'fulfilled') memoryContent = memRes.value.content;
		if (memPatchRes.status === 'fulfilled') memoryPatch = memPatchRes.value.patch ?? null;
		soulLoading = false; userLoading = false; agentsLoading = false; memoryLoading = false;
	});

	// --- Save handlers ---
	async function saveSoul() {
		soulSaving = true;
		try { const res = await updateSoul(soulContent); soulSha = res.sha; soulEditing = false; }
		catch (e) { console.error('Save soul:', e); }
		finally { soulSaving = false; }
	}
	async function saveUser() {
		userSaving = true;
		try { await updateUserProfile(userContent); userEditing = false; }
		catch (e) { console.error('Save user:', e); }
		finally { userSaving = false; }
	}
	async function saveAgents() {
		agentsSaving = true;
		try { await updateAgentsConfig(agentsContent); agentsEditing = false; }
		catch (e) { console.error('Save agents:', e); }
		finally { agentsSaving = false; }
	}
	async function saveMemory() {
		memorySaving = true;
		try { await updateMemoryFile(memoryContent); memoryEditing = false; }
		catch (e) { console.error('Save memory:', e); }
		finally { memorySaving = false; }
	}

	// --- Patch handlers ---
	async function handleApprovePatch(target: 'soul' | 'agents' | 'memory') {
		const p = target === 'soul' ? soulPatch : target === 'agents' ? agentsPatch : memoryPatch;
		if (!p) return;
		patchActing = true;
		try {
			if (target === 'soul') { await approveSoulPatch(p.id); const res = await getSoul(); soulContent = res.content; soulSha = res.sha ?? null; soulPatch = null; }
			else if (target === 'agents') { await approveAgentsPatch(p.id); const res = await getAgentsConfig(); agentsContent = res.content; agentsPatch = null; }
			else { await approveMemoryPatch(p.id); const res = await getMemoryFile(); memoryContent = res.content; memoryPatch = null; }
			denyMode = false;
		} catch (e) { console.error('Approve patch:', e); }
		finally { patchActing = false; }
	}

	async function handleDenyPatch(target: 'soul' | 'agents' | 'memory') {
		const p = target === 'soul' ? soulPatch : target === 'agents' ? agentsPatch : memoryPatch;
		if (!p || !denyReason.trim()) return;
		patchActing = true;
		try {
			if (target === 'soul') { await denySoulPatch(p.id, denyReason.trim()); soulPatch = null; }
			else if (target === 'agents') { await denyAgentsPatch(p.id, denyReason.trim()); agentsPatch = null; }
			else { await denyMemoryPatch(p.id, denyReason.trim()); memoryPatch = null; }
			denyMode = false; denyReason = '';
		} catch (e) { console.error('Deny patch:', e); }
		finally { patchActing = false; }
	}

	// --- Soul history ---
	async function loadSoulHistory() {
		showSoulHistory = !showSoulHistory;
		if (showSoulHistory && soulHistory.length === 0) {
			try { const res = await getSoulHistory(); soulHistory = res.items; }
			catch (e) { console.error('History:', e); }
		}
	}

	// --- SSE listeners ---
	function onSoulPatchSSE() {
		if (soulLoading) return;
		Promise.all([getSoul().catch(() => null), getSoulPatch().catch(() => ({ patch: null }))]).then(([s, p]) => {
			if (s) { soulContent = s.content; soulSha = s.sha ?? null; }
			soulPatch = p?.patch ?? null;
		});
	}
	onMount(() => window.addEventListener('cairn:soul-patch', onSoulPatchSSE));
	onDestroy(() => window.removeEventListener('cairn:soul-patch', onSoulPatchSSE));

	// --- Diff computation ---
	function computeDiff(current: string, preview: string) {
		const oldLines = current.split('\n');
		const newLines = preview.split('\n');
		const result: { type: 'same' | 'add' | 'remove'; text: string; lineNo: number }[] = [];
		let i = 0;
		while (i < oldLines.length && i < newLines.length && oldLines[i] === newLines[i]) i++;
		const contextStart = Math.max(0, i - 3);
		for (let j = contextStart; j < i; j++) result.push({ type: 'same', text: oldLines[j], lineNo: j + 1 });
		for (let j = i; j < oldLines.length; j++) {
			if (j < newLines.length && oldLines[j] !== newLines[j]) result.push({ type: 'remove', text: oldLines[j], lineNo: j + 1 });
		}
		for (let j = i; j < newLines.length; j++) result.push({ type: 'add', text: newLines[j], lineNo: j + 1 });
		return result;
	}

	// TOC for current tab content
	function extractToc(content: string) {
		const headings: { text: string; id: string }[] = [];
		for (const line of content.split('\n')) {
			const match = line.match(/^##\s+(.+)/);
			if (match) {
				const text = match[1].trim();
				headings.push({ text, id: text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '') });
			}
		}
		return headings;
	}
</script>

<div class="mx-auto max-w-4xl px-4 py-4 sm:p-6">
	<div class="mb-4">
		<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Identity</h1>
		<p class="mt-1 text-xs text-[var(--text-tertiary)]">Agent identity, user profile, operating rules, and pinned knowledge</p>
	</div>

	<Tabs.Root bind:value={tab}>
		<Tabs.List class="mb-5 w-full">
			<Tabs.Trigger value="soul" class="gap-1.5 text-xs">
				<Heart class="h-3.5 w-3.5" /> Soul
				{#if soulPatch}<span class="ml-1 h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]"></span>{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="user" class="gap-1.5 text-xs">
				<User class="h-3.5 w-3.5" /> User
			</Tabs.Trigger>
			<Tabs.Trigger value="agents" class="gap-1.5 text-xs">
				<Shield class="h-3.5 w-3.5" /> Agents
				{#if agentsPatch}<span class="ml-1 h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]"></span>{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="memory" class="gap-1.5 text-xs">
				<Brain class="h-3.5 w-3.5" /> Memory
				{#if memoryPatch}<span class="ml-1 h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]"></span>{/if}
			</Tabs.Trigger>
		</Tabs.List>

		<!-- SOUL TAB -->
		<Tabs.Content value="soul">
			{@render tabHeader('soul', soulEditing, soulSaving, () => soulEditing = !soulEditing, saveSoul, loadSoulHistory)}
			{@render patchBanner(soulPatch, soulContent, 'soul')}
			{@render editorOrPreview(soulContent, soulLoading, soulEditing, (v: string) => soulContent = v)}
			{#if showSoulHistory && soulHistory.length > 0}
				<Separator class="my-6" />
				<h2 class="mb-3 text-sm font-medium text-[var(--text-primary)]">Revision History</h2>
				<div class="flex flex-col gap-1">
					{#each soulHistory as entry, i (entry.sha)}
						<div class="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-[var(--bg-1)] transition-colors" style="animation-delay: {i * 30}ms">
							<GitCommit class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
							<code class="text-[11px] font-mono text-[var(--cairn-accent)]">{entry.sha.slice(0, 7)}</code>
							<p class="flex-1 text-sm text-[var(--text-primary)] truncate">{entry.message}</p>
							<time class="text-[11px] text-[var(--text-tertiary)] tabular-nums font-mono" datetime={entry.date}>{relativeTime(entry.date)}</time>
						</div>
					{/each}
				</div>
			{/if}
		</Tabs.Content>

		<!-- USER TAB -->
		<Tabs.Content value="user">
			{@render tabHeader('user', userEditing, userSaving, () => userEditing = !userEditing, saveUser)}
			{@render editorOrPreview(userContent, userLoading, userEditing, (v: string) => userContent = v)}
		</Tabs.Content>

		<!-- AGENTS TAB -->
		<Tabs.Content value="agents">
			{@render tabHeader('agents', agentsEditing, agentsSaving, () => agentsEditing = !agentsEditing, saveAgents)}
			{@render patchBanner(agentsPatch, agentsContent, 'agents')}
			{@render editorOrPreview(agentsContent, agentsLoading, agentsEditing, (v: string) => agentsContent = v)}
		</Tabs.Content>

		<!-- MEMORY TAB -->
		<Tabs.Content value="memory">
			{@render tabHeader('memory', memoryEditing, memorySaving, () => memoryEditing = !memoryEditing, saveMemory)}
			{@render patchBanner(memoryPatch, memoryContent, 'memory')}
			{@render editorOrPreview(memoryContent, memoryLoading, memoryEditing, (v: string) => memoryContent = v)}
		</Tabs.Content>
	</Tabs.Root>
</div>

<!-- SNIPPETS -->

{#snippet tabHeader(target: string, editing: boolean, saving: boolean, toggleEdit: () => void, save: () => void, historyFn?: () => void)}
	<div class="mb-4 flex items-center justify-between">
		<p class="text-xs text-[var(--text-tertiary)]">{TAB_INFO[target]?.description ?? ''}</p>
		<div class="flex gap-2">
			{#if historyFn}
				<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={historyFn}>
					<History class="h-3.5 w-3.5" /> History
				</Button>
			{/if}
			<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={toggleEdit}>
				{#if editing}<Eye class="h-3.5 w-3.5" /> Preview{:else}<Edit3 class="h-3.5 w-3.5" /> Edit{/if}
			</Button>
			{#if editing}
				<Button size="sm" class="h-8 text-xs gap-1.5" onclick={save} disabled={saving}>
					<Save class="h-3.5 w-3.5" /> {saving ? 'Saving...' : 'Save'}
				</Button>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet patchBanner(patch: SoulPatch | null, content: string, target: 'soul' | 'agents' | 'memory')}
	{#if patch}
		<div class="mb-6 rounded-lg border-2 border-[var(--color-warning)]/40 bg-[var(--color-warning)]/5 overflow-hidden">
			<div class="px-4 py-3 flex items-center gap-3 border-b border-[var(--color-warning)]/20">
				<AlertTriangle class="h-4 w-4 text-[var(--color-warning)] flex-shrink-0" />
				<div class="flex-1 min-w-0">
					<p class="text-sm font-medium text-[var(--text-primary)]">Patch proposed</p>
					<p class="text-[11px] text-[var(--text-tertiary)]">From {patch.source} - {relativeTime(patch.createdAt)}</p>
				</div>
				<Badge variant="outline" class="text-[10px] text-[var(--color-warning)]">pending review</Badge>
			</div>
			<div class="px-4 py-3">
				<p class="text-[10px] uppercase tracking-wider text-[var(--text-tertiary)] mb-2">Proposed changes</p>
				<div class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] overflow-hidden font-mono text-xs leading-5">
					{#each computeDiff(content, patch.preview) as line}
						<div class="flex {line.type === 'add' ? 'bg-[var(--color-success)]/10' : line.type === 'remove' ? 'bg-[var(--color-error)]/10' : ''}">
							<span class="w-10 flex-shrink-0 text-right pr-2 text-[var(--text-tertiary)]/50 select-none border-r border-[var(--border-subtle)]/50 py-px">{line.lineNo}</span>
							<span class="w-5 flex-shrink-0 text-center select-none py-px {line.type === 'add' ? 'text-[var(--color-success)]' : line.type === 'remove' ? 'text-[var(--color-error)]' : 'text-[var(--text-tertiary)]/30'}">
								{line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}
							</span>
							<span class="flex-1 px-2 py-px whitespace-pre-wrap break-all {line.type === 'add' ? 'text-[var(--color-success)]' : line.type === 'remove' ? 'text-[var(--color-error)]' : 'text-[var(--text-primary)]'}">{line.text}</span>
						</div>
					{/each}
				</div>
			</div>
			<div class="px-4 py-3 border-t border-[var(--color-warning)]/20 bg-[var(--bg-1)]/50">
				{#if denyMode}
					<div class="space-y-2">
						<p class="text-xs text-[var(--text-secondary)]">Why are you denying this patch?</p>
						<textarea bind:value={denyReason} placeholder="Your reason will be saved to memory..." class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/50 focus:border-[var(--cairn-accent)] focus:outline-none resize-none h-20"></textarea>
						<div class="flex gap-2 justify-end">
							<Button variant="outline" size="sm" class="h-7 text-xs" onclick={() => { denyMode = false; denyReason = ''; }}>Cancel</Button>
							<Button size="sm" class="h-7 text-xs gap-1 bg-[var(--color-error)] hover:bg-[var(--color-error)]/90" onclick={() => handleDenyPatch(target)} disabled={patchActing || !denyReason.trim()}>
								{#if patchActing}<Loader2 class="h-3 w-3 animate-spin" />{:else}<X class="h-3 w-3" />{/if} Deny
							</Button>
						</div>
					</div>
				{:else}
					<div class="flex gap-2 justify-end">
						<Button variant="outline" size="sm" class="h-7 text-xs gap-1 text-[var(--color-error)]" onclick={() => denyMode = true}><X class="h-3 w-3" /> Deny</Button>
						<Button size="sm" class="h-7 text-xs gap-1" onclick={() => handleApprovePatch(target)} disabled={patchActing}>
							{#if patchActing}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Check class="h-3 w-3" />{/if} Approve & apply
						</Button>
					</div>
				{/if}
			</div>
		</div>
	{/if}
{/snippet}

{#snippet editorOrPreview(content: string, loading: boolean, editing: boolean, onchange: (v: string) => void)}
	{#if loading}
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-6 space-y-3">
			<Skeleton class="h-6 w-48" /><Skeleton class="h-4 w-full" /><Skeleton class="h-4 w-3/4" /><Skeleton class="h-4 w-5/6" />
		</div>
	{:else if editing}
		<textarea
			value={content}
			oninput={(e) => onchange(e.currentTarget.value)}
			class="h-[calc(100vh-320px)] w-full resize-none rounded-lg border border-border-subtle bg-[var(--bg-1)] p-5 font-mono text-sm text-[var(--text-primary)] leading-relaxed focus:border-[var(--cairn-accent)] focus:ring-1 focus:ring-[var(--cairn-accent)]/30 focus:outline-none transition-colors"
			spellcheck="false"
		></textarea>
	{:else if content}
		<div class="flex gap-6">
			{#if extractToc(content).length > 2}
				<nav class="hidden lg:block w-48 flex-shrink-0 sticky top-6 self-start">
					<p class="text-[10px] uppercase tracking-wider text-[var(--text-tertiary)] mb-2 font-medium">Contents</p>
					<ul class="space-y-1 border-l border-border-subtle pl-3">
						{#each extractToc(content) as heading}
							<li><a href="#{heading.id}" class="text-xs text-[var(--text-tertiary)] hover:text-[var(--cairn-accent)] transition-colors block py-0.5 truncate">{heading.text}</a></li>
						{/each}
					</ul>
				</nav>
			{/if}
			<div class="flex-1 min-w-0 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-6">
				{#key content.length}
					<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">{@html renderMarkdown(content)}</div>
				{/key}
			</div>
		</div>
	{:else}
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-8 text-center">
			<p class="text-sm text-[var(--text-tertiary)]">This file is empty. Click Edit to add content.</p>
		</div>
	{/if}
{/snippet}

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
	.cairn-prose :global(h1) { font-size: 1.5em; font-weight: 700; margin: 1em 0 0.5em; color: var(--text-primary); border-bottom: 1px solid var(--border-subtle); padding-bottom: 0.3em; }
	.cairn-prose :global(h2) { font-size: 1.2em; font-weight: 600; margin: 1.2em 0 0.4em; color: var(--text-primary); scroll-margin-top: 2rem; }
	.cairn-prose :global(h3) { font-size: 1.05em; font-weight: 600; margin: 0.8em 0 0.3em; color: var(--text-primary); }
	.cairn-prose :global(blockquote) { border-left: 3px solid var(--cairn-accent); padding-left: 1em; margin: 0.75em 0; color: var(--text-secondary); font-style: italic; }
	.cairn-prose :global(hr) { border: none; border-top: 1px solid var(--border-subtle); margin: 1.5em 0; }
	.cairn-prose :global(table) { width: 100%; border-collapse: collapse; margin: 0.75em 0; font-size: 0.9em; }
	.cairn-prose :global(th), .cairn-prose :global(td) { border: 1px solid var(--border-subtle); padding: 0.4em 0.6em; text-align: left; }
	.cairn-prose :global(th) { background: var(--bg-2); font-weight: 600; color: var(--text-primary); }
	.cairn-prose :global(a) { color: var(--cairn-accent); text-decoration: none; }
	.cairn-prose :global(a:hover) { text-decoration: underline; }
</style>
