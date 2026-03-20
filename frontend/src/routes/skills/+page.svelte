<script lang="ts">
	import { onMount } from 'svelte';
	import { getSkills, getSkillDetail, createSkillApi, updateSkillApi, deleteSkillApi, searchMarketplace, browseMarketplace, getMarketplaceDetail, getMarketplacePreview, installMarketplaceSkill, reviewMarketplaceSkill } from '$lib/api/client';
	import { renderMarkdown } from '$lib/utils/markdown';
	import type { Skill, MarketplaceSearchResult, MarketplaceSkill } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Sparkles, Search, X, ChevronDown, ChevronUp, FileText, Loader2, Plus, Pencil, Trash2, Save, Download, Star, Store, Check, ShieldAlert, ShieldCheck } from '@lucide/svelte';

	let skills = $state<Skill[]>([]);
	let activeSkills = $state<string[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let expandedSkill = $state<string | null>(null);
	let detailContent = $state<string | null>(null);
	let detailLoading = $state(false);
	let dialogOpen = $state(false);
	let dialogSkill = $state<{ name: string; content: string } | null>(null);

	// Create form state
	let showCreate = $state(false);
	let newName = $state('');
	let newDescription = $state('');
	let newContent = $state('');
	let newInclusion = $state('on-demand');
	let newAllowedTools = $state('');
	let creating = $state(false);
	let createError = $state('');

	// Edit state
	let editingSkill = $state<string | null>(null);
	let editDesc = $state('');
	let editContent = $state('');
	let editInclusion = $state('on-demand');
	let editAllowedTools = $state('');
	let savingEdit = $state(false);
	let editError = $state('');

	// Tab state
	let activeTab = $state<'installed' | 'marketplace'>('installed');

	// Marketplace state
	let mpQuery = $state('');
	let mpResults = $state<MarketplaceSearchResult[]>([]);
	let mpBrowse = $state<MarketplaceSkill[]>([]);
	let mpLoading = $state(false);
	let mpSort = $state<'trending' | 'downloads' | 'stars' | 'updated'>('trending');
	let mpInstalled = $state<Record<string, boolean>>({});
	let installing = $state<string | null>(null);
	let mpPreviewSlug = $state<string | null>(null);
	let mpPreviewContent = $state<string | null>(null);
	let mpPreviewLoading = $state(false);
	let mpDebounceTimer: ReturnType<typeof setTimeout> | null = null;

	// Stats enrichment: slug -> {downloads, stars}
	let mpStats = $state<Record<string, { downloads: number; stars: number }>>({});
	let mpStatsFetching = $state<Set<string>>(new Set());

	// Security review state
	let reviewSlug = $state<string | null>(null);
	let reviewLoading = $state(false);
	let reviewResult = $state<{ safe: boolean; risk: string; issues: string[]; summary: string } | null>(null);

	onMount(async () => {
		try {
			const res = await getSkills();
			skills = res.items;
			activeSkills = res.currentlyActive ?? [];
		} catch {
			// handled
		} finally {
			loading = false;
		}
	});

	const filtered = $derived(() => {
		if (!searchQuery.trim()) return skills;
		const q = searchQuery.toLowerCase();
		return skills.filter(
			(s) =>
				s.name.toLowerCase().includes(q) ||
				s.description.toLowerCase().includes(q) ||
				s.scope.toLowerCase().includes(q) ||
				s.inclusion.toLowerCase().includes(q),
		);
	});

	async function toggleExpanded(name: string) {
		if (expandedSkill === name) {
			expandedSkill = null;
			detailContent = null;
			return;
		}
		expandedSkill = name;
		detailContent = null;
		detailLoading = true;
		try {
			const detail = await getSkillDetail(name);
			detailContent = detail.content ?? null;
		} catch {
			detailContent = null;
		} finally {
			detailLoading = false;
		}
	}

	function openDialog(name: string, content: string) {
		dialogSkill = { name, content };
		dialogOpen = true;
	}

	async function handleCreate() {
		if (!newName.trim() || !newDescription.trim() || !newContent.trim()) {
			createError = 'Name, description, and content are required';
			return;
		}
		creating = true;
		createError = '';
		try {
			const tools = newAllowedTools.trim() ? newAllowedTools.split(',').map((t) => t.trim()).filter(Boolean) : undefined;
			await createSkillApi({ name: newName.trim(), description: newDescription.trim(), content: newContent.trim(), inclusion: newInclusion, allowedTools: tools });
			const res = await getSkills();
			skills = res.items;
			showCreate = false;
			newName = '';
			newDescription = '';
			newContent = '';
			newAllowedTools = '';
		} catch (e) {
			createError = e instanceof Error ? e.message : 'Failed to create';
		} finally {
			creating = false;
		}
	}

	async function startEdit(skill: Skill) {
		editingSkill = skill.name;
		editDesc = skill.description;
		editInclusion = skill.inclusion;
		editAllowedTools = skill.allowedTools?.join(', ') ?? '';
		editError = '';
		// Fetch fresh content for this specific skill.
		try {
			const detail = await getSkillDetail(skill.name);
			editContent = detail.content ?? '';
		} catch {
			editContent = '';
		}
	}

	async function saveEdit(name: string) {
		savingEdit = true;
		editError = '';
		try {
			const tools = editAllowedTools.trim() ? editAllowedTools.split(',').map((t) => t.trim()).filter(Boolean) : undefined;
			await updateSkillApi(name, { description: editDesc.trim() || undefined, content: editContent.trim() || undefined, inclusion: editInclusion || undefined, allowedTools: tools });
			const res = await getSkills();
			skills = res.items;
			editingSkill = null;
		} catch (e) {
			editError = e instanceof Error ? e.message : 'Failed to update';
		} finally {
			savingEdit = false;
		}
	}

	async function handleDelete(name: string) {
		try {
			await deleteSkillApi(name);
			skills = skills.filter((s) => s.name !== name);
			if (expandedSkill === name) expandedSkill = null;
		} catch (e) {
			console.error('Failed to delete skill:', e);
		}
	}

	async function toggleInclusion(skill: Skill) {
		const newInc = skill.inclusion === 'always' ? 'on-demand' : 'always';
		try {
			await updateSkillApi(skill.name, { inclusion: newInc });
			skills = skills.map((s) => s.name === skill.name ? { ...s, inclusion: newInc } : s);
		} catch (e) {
			console.error('Failed to toggle inclusion:', e);
		}
	}

	async function mpSearch(query: string) {
		if (!query.trim()) { mpResults = []; return; }
		mpLoading = true;
		try {
			const res = await searchMarketplace(query, 15);
			mpResults = res.results ?? [];
			mpInstalled = res.installed ?? {};
		} catch (e) { console.error('Marketplace search failed:', e); mpResults = []; }
		finally { mpLoading = false; }
	}

	function mpSearchDebounced(query: string) {
		if (mpDebounceTimer) clearTimeout(mpDebounceTimer);
		mpDebounceTimer = setTimeout(() => mpSearch(query), 300);
	}

	const FEATURED_QUERIES = ['git', 'docker', 'python', 'react', 'typescript', 'rust', 'go', 'svelte'];

	async function mpBrowseLoad() {
		mpLoading = true;
		try {
			const res = await browseMarketplace(mpSort, 30);
			const items = res.skills ?? [];
			mpInstalled = res.installed ?? {};
			if (items.length > 0) {
				mpBrowse = items;
				return;
			}
			// Browse returned empty — seed with featured search results instead.
			const picks = FEATURED_QUERIES.sort(() => Math.random() - 0.5).slice(0, 4);
			const batched = await Promise.allSettled(picks.map(q => searchMarketplace(q, 5)));
			const seeded: MarketplaceSkill[] = [];
			const seen = new Set<string>();
			for (const r of batched) {
				if (r.status !== 'fulfilled') continue;
				for (const sr of r.value.results ?? []) {
					if (seen.has(sr.slug)) continue;
					seen.add(sr.slug);
					seeded.push({ slug: sr.slug, displayName: sr.displayName, summary: sr.summary, stats: { downloads: 0, stars: 0, versions: 0, installsAllTime: 0 }, owner: { handle: '', displayName: '', image: '' }, latestVersion: { version: sr.version, changelog: '' } });
					mpInstalled = { ...mpInstalled, ...(r.value.installed ?? {}) };
				}
			}
			mpBrowse = seeded;
		} catch (e) { console.error('Marketplace browse failed:', e); mpBrowse = []; }
		finally { mpLoading = false; }
	}

	async function enrichStats(slugs: string[]) {
		const toFetch = slugs.filter(s => !mpStats[s] && !mpStatsFetching.has(s));
		if (toFetch.length === 0) return;
		toFetch.forEach(s => mpStatsFetching.add(s));
		mpStatsFetching = new Set(mpStatsFetching);
		const results = await Promise.allSettled(toFetch.map(s => getMarketplaceDetail(s)));
		const newStats = { ...mpStats };
		results.forEach((r, i) => {
			if (r.status === 'fulfilled' && r.value.skill?.stats) {
				newStats[toFetch[i]] = { downloads: r.value.skill.stats.downloads ?? 0, stars: r.value.skill.stats.stars ?? 0 };
			}
		});
		mpStats = newStats;
		toFetch.forEach(s => mpStatsFetching.delete(s));
		mpStatsFetching = new Set(mpStatsFetching);
	}

	// Trigger stats enrichment when display items change
	$effect(() => {
		const items = mpDisplayItems();
		if (items.length > 0 && activeTab === 'marketplace') {
			const slugs = items.slice(0, 10).map(i => i.slug).filter(s => !mpStats[s]);
			if (slugs.length > 0) enrichStats(slugs);
		}
	});

	async function handleInstallClick(slug: string) {
		// Step 1: Security review
		reviewSlug = slug;
		reviewLoading = true;
		reviewResult = null;
		try {
			const res = await reviewMarketplaceSkill(slug);
			reviewResult = res;
			reviewLoading = false;
			// If safe, don't auto-install - let user confirm
		} catch (e) {
			console.error('Security review failed:', e);
			reviewResult = { safe: false, risk: 'unknown', issues: ['Review request failed'], summary: 'Could not complete security review' };
			reviewLoading = false;
		}
	}

	async function confirmInstall(slug: string) {
		reviewSlug = null;
		reviewResult = null;
		installing = slug;
		try {
			await installMarketplaceSkill(slug);
			mpInstalled = { ...mpInstalled, [slug]: true };
			const res = await getSkills();
			skills = res.items;
		} catch (e) {
			console.error('Failed to install skill:', e);
		} finally {
			installing = null;
		}
	}

	async function showMpPreview(slug: string) {
		mpPreviewSlug = slug;
		mpPreviewContent = null;
		mpPreviewLoading = true;
		try {
			const res = await getMarketplacePreview(slug);
			mpPreviewContent = res.content ?? null;
		} catch (e) { console.error('Marketplace preview failed:', e); mpPreviewContent = null; }
		finally { mpPreviewLoading = false; }
	}

	$effect(() => {
		if (activeTab === 'marketplace' && mpBrowse.length === 0 && !mpQuery.trim()) {
			mpBrowseLoad();
		}
	});

	$effect(() => {
		if (activeTab === 'marketplace' && mpQuery) mpSearchDebounced(mpQuery);
		// Clean up debounce timer when leaving marketplace tab.
		if (activeTab !== 'marketplace' && mpDebounceTimer) {
			clearTimeout(mpDebounceTimer);
			mpDebounceTimer = null;
		}
	});

	interface MpDisplayItem {
		slug: string;
		name: string;
		summary: string;
		version: string;
		downloads: number;
		stars: number;
		installed: boolean;
	}

	function normalizeSearchResult(r: MarketplaceSearchResult): MpDisplayItem {
		const stats = mpStats[r.slug];
		return { slug: r.slug, name: r.displayName || r.slug, summary: r.summary, version: r.version || '', downloads: stats?.downloads ?? 0, stars: stats?.stars ?? 0, installed: mpInstalled[r.slug] ?? false };
	}

	function normalizeBrowseItem(s: MarketplaceSkill): MpDisplayItem {
		return { slug: s.slug, name: s.displayName || s.slug, summary: s.summary, version: s.latestVersion?.version || '', downloads: s.stats?.downloads || 0, stars: s.stats?.stars || 0, installed: mpInstalled[s.slug] ?? false };
	}

	const mpDisplayItems = $derived(() => {
		// Read mpStats here so Svelte tracks it as a reactive dependency
		const stats = mpStats;
		if (mpQuery.trim()) return mpResults.map(r => {
			const s = stats[r.slug];
			return { slug: r.slug, name: r.displayName || r.slug, summary: r.summary, version: r.version || '', downloads: s?.downloads ?? 0, stars: s?.stars ?? 0, installed: mpInstalled[r.slug] ?? false };
		});
		return mpBrowse.map(s => {
			const enriched = stats[s.slug];
			return { slug: s.slug, name: s.displayName || s.slug, summary: s.summary, version: s.latestVersion?.version || '', downloads: enriched?.downloads ?? s.stats?.downloads ?? 0, stars: enriched?.stars ?? s.stats?.stars ?? 0, installed: mpInstalled[s.slug] ?? false };
		});
	});

	const inclusionColors: Record<string, string> = {
		always: 'text-[var(--color-success)]',
		auto: 'text-[var(--cairn-accent)]',
		manual: 'text-[var(--color-warning)]',
	};
</script>

<div class="mx-auto max-w-5xl p-6">
	<div class="mb-4 flex items-center justify-between">
		<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Skills</h1>
		<div class="flex items-center gap-3">
			{#if skills.length > 0 && activeTab === 'installed'}
				<span class="text-[11px] text-[var(--text-tertiary)] font-mono tabular-nums">
					{activeSkills.length} active / {skills.length} loaded
				</span>
			{/if}
			{#if activeTab === 'installed'}
				<Button size="sm" class="h-7 text-xs gap-1" onclick={() => showCreate = !showCreate}>
					<Plus class="h-3 w-3" /> New Skill
				</Button>
			{/if}
		</div>
	</div>

	<!-- Tabs -->
	<div class="mb-4 flex gap-1 rounded-lg bg-[var(--bg-1)] p-0.5 border border-border-subtle w-fit">
		<button
			class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors {activeTab === 'installed' ? 'bg-[var(--bg-0)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
			onclick={() => activeTab = 'installed'}
		>
			Installed ({skills.length})
		</button>
		<button
			class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors flex items-center gap-1.5 {activeTab === 'marketplace' ? 'bg-[var(--bg-0)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
			onclick={() => activeTab = 'marketplace'}
		>
			<Store class="h-3 w-3" /> Marketplace
		</button>
	</div>

	{#if activeTab === 'installed'}
	<!-- Create form -->
	{#if showCreate}
		<div class="mb-6 rounded-lg border border-[var(--cairn-accent)]/30 bg-[var(--bg-1)] p-4 space-y-3">
			<p class="text-sm font-medium text-[var(--text-primary)]">Create New Skill</p>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Name</p>
					<Input type="text" bind:value={newName} placeholder="my-skill" class="h-7 text-xs font-mono" />
				</div>
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Inclusion</p>
					<select bind:value={newInclusion} class="w-full h-7 rounded-md border border-border-subtle bg-[var(--bg-0)] px-2 text-xs text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none">
						<option value="on-demand">On Demand</option>
						<option value="always">Always</option>
					</select>
				</div>
			</div>
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Description (trigger keywords)</p>
				<Input type="text" bind:value={newDescription} placeholder="Use when user asks to..." class="h-7 text-xs" />
			</div>
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Allowed Tools (comma-separated, empty = all)</p>
				<Input type="text" bind:value={newAllowedTools} placeholder="cairn.shell, cairn.readFile" class="h-7 text-xs font-mono" />
			</div>
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Content (Markdown)</p>
				<textarea
					bind:value={newContent}
					placeholder="# My Skill\n\nInstructions for cairn..."
					class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs font-mono text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/40 focus:border-[var(--cairn-accent)] focus:outline-none resize-y h-32"
				></textarea>
			</div>
			{#if createError}
				<p class="text-[10px] text-[var(--color-error)]">{createError}</p>
			{/if}
			<div class="flex justify-end gap-2">
				<Button variant="outline" size="sm" class="h-7 text-xs" onclick={() => showCreate = false}>Cancel</Button>
				<Button size="sm" class="h-7 text-xs gap-1" onclick={handleCreate} disabled={creating}>
					{#if creating}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Plus class="h-3 w-3" />{/if}
					Create
				</Button>
			</div>
		</div>
	{/if}

	<!-- Search -->
	{#if skills.length > 0}
		<div class="mb-4 relative">
			<Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-[var(--text-tertiary)]" />
			<input
				type="text"
				bind:value={searchQuery}
				placeholder="Search skills..."
				aria-label="Search skills"
				class="w-full rounded-lg border border-border-subtle bg-[var(--bg-1)] pl-9 pr-8 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none focus:ring-1 focus:ring-[var(--cairn-accent)]/30"
			/>
			{#if searchQuery}
				<button
					class="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
					onclick={() => { searchQuery = ''; }}
					type="button"
					aria-label="Clear search"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			{/if}
		</div>
	{/if}

	{#if loading}
		<div class="flex flex-col gap-2">
			{#each Array(6) as _, i}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 animate-in" style="animation-delay: {i * 40}ms">
					<Skeleton class="h-4 w-32 mb-1" />
					<Skeleton class="h-3 w-48" />
				</div>
			{/each}
		</div>
	{:else if skills.length === 0}
		<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
			<Sparkles class="mb-3 h-10 w-10 opacity-30" />
			<p class="text-sm">No skills loaded</p>
			<p class="mt-1 text-xs opacity-60">Add SKILL.md files to your skill directories</p>
		</div>
	{:else if filtered().length === 0}
		<div class="py-12 text-center">
			<p class="text-sm text-[var(--text-tertiary)]">No skills match "{searchQuery}"</p>
		</div>
	{:else}
		<div class="flex flex-col gap-2">
			{#each filtered() as skill, i (skill.name)}
				{@const isActive = activeSkills.includes(skill.name)}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] card-hover animate-in" style="animation-delay: {i * 25}ms">
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="flex w-full items-center gap-3 p-3 text-left cursor-pointer"
						onclick={() => toggleExpanded(skill.name)}
						role="button"
						tabindex="0"
						onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleExpanded(skill.name); }}
						aria-expanded={expandedSkill === skill.name}
					>
						{#if isActive}
							<span class="h-2 w-2 rounded-full bg-[var(--color-success)]"></span>
						{:else}
							<span class="h-2 w-2 rounded-full bg-[var(--bg-3)]"></span>
						{/if}
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<p class="text-sm font-medium text-[var(--text-primary)]">{skill.name}</p>
								{#if skill.userInvocable}
									<Badge variant="outline" class="h-4 px-1 text-[10px]">invocable</Badge>
								{/if}
								<button
									class="h-4 px-1.5 text-[10px] font-medium rounded-full border transition-colors
										{skill.inclusion === 'always'
										? 'bg-[var(--color-success)]/10 text-[var(--color-success)] border-[var(--color-success)]/30 hover:bg-[var(--color-success)]/20'
										: 'bg-[var(--bg-2)] text-[var(--text-tertiary)] border-border-subtle hover:bg-[var(--bg-3)]'}"
									title="Click to toggle: {skill.inclusion === 'always' ? 'switch to on-demand' : 'switch to always'}"
									onclick={(e) => { e.stopPropagation(); toggleInclusion(skill); }}
								>
									{skill.inclusion}
								</button>
							</div>
							<p class="truncate text-xs text-[var(--text-secondary)]">{expandedSkill !== skill.name ? skill.description : ''}</p>
						</div>
						<Badge variant="secondary" class="h-5 text-[10px] flex-shrink-0">
							{skill.scope}
						</Badge>
						{#if expandedSkill === skill.name}
							<ChevronUp class="h-4 w-4 flex-shrink-0 text-[var(--text-tertiary)]" />
						{:else}
							<ChevronDown class="h-4 w-4 flex-shrink-0 text-[var(--text-tertiary)]" />
						{/if}
					</div>

					{#if expandedSkill === skill.name}
						<div class="border-t border-border-subtle px-4 py-3">
							<div class="flex flex-wrap gap-2 mb-3">
								{#if skill.disableModelInvocation}
									<Badge variant="outline" class="h-4 px-1 text-[10px] text-[var(--color-warning)]">manual-only</Badge>
								{/if}
								{#if skill.allowedTools && skill.allowedTools.length > 0}
									{#each skill.allowedTools as tool}
										<Badge variant="outline" class="h-4 px-1 text-[10px] font-mono">{tool}</Badge>
									{/each}
								{/if}
							</div>
							<p class="text-xs text-[var(--text-secondary)] mb-3">{skill.description}</p>

							<!-- Edit/Delete buttons -->
							<div class="flex gap-2 mb-3">
								<Button variant="outline" size="sm" class="h-6 text-[10px] gap-1 px-2" onclick={() => startEdit(skill)}>
									<Pencil class="h-3 w-3" /> Edit
								</Button>
								<Button variant="outline" size="sm" class="h-6 text-[10px] gap-1 px-2 text-[var(--color-error)]" onclick={() => handleDelete(skill.name)}>
									<Trash2 class="h-3 w-3" /> Delete
								</Button>
							</div>

							<!-- Edit form (inline) -->
							{#if editingSkill === skill.name}
								<div class="rounded-md border border-[var(--cairn-accent)]/30 bg-[var(--bg-0)] p-3 mb-3 space-y-2">
									<div>
										<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Description</p>
										<Input type="text" bind:value={editDesc} class="h-7 text-xs" />
									</div>
									<div class="grid grid-cols-2 gap-2">
										<div>
											<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Inclusion</p>
											<select bind:value={editInclusion} class="w-full h-7 rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 text-xs text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none">
												<option value="on-demand">On Demand</option>
												<option value="always">Always</option>
											</select>
										</div>
										<div>
											<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Allowed Tools</p>
											<Input type="text" bind:value={editAllowedTools} class="h-7 text-xs font-mono" />
										</div>
									</div>
									<div>
										<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Content</p>
										<textarea
											bind:value={editContent}
											class="w-full rounded-md border border-border-subtle bg-[var(--bg-1)] px-2.5 py-1.5 text-xs font-mono text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none resize-y h-32"
										></textarea>
									</div>
									{#if editError}
										<p class="text-[10px] text-[var(--color-error)]">{editError}</p>
									{/if}
									<div class="flex gap-2 justify-end">
										<Button variant="outline" size="sm" class="h-6 text-[10px]" onclick={() => editingSkill = null}>Cancel</Button>
										<Button size="sm" class="h-6 text-[10px] gap-1" onclick={() => saveEdit(skill.name)} disabled={savingEdit}>
											{#if savingEdit}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
											Save
										</Button>
									</div>
								</div>
							{/if}

							<!-- Skill content preview -->
							{#if detailLoading}
								<div class="flex items-center gap-2 text-xs text-[var(--text-tertiary)]">
									<Loader2 class="h-3 w-3 animate-spin" /> Loading skill content...
								</div>
							{:else if detailContent}
								<div class="rounded-md border border-border-subtle bg-[var(--bg-0)] p-3 max-h-48 overflow-y-auto">
									<div class="cairn-prose text-xs text-[var(--text-primary)] leading-relaxed">
										{@html renderMarkdown(detailContent.length > 1000 ? detailContent.slice(0, 1000) + '\n\n...' : detailContent)}
									</div>
								</div>
								<button
									class="mt-2 text-xs text-[var(--cairn-accent)] hover:underline flex items-center gap-1"
									onclick={() => openDialog(skill.name, detailContent ?? '')}
									type="button"
								>
									<FileText class="h-3 w-3" /> View full SKILL.md
								</button>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	{:else}
	<!-- Marketplace tab -->
	<div class="mb-4 relative">
		<Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-[var(--text-tertiary)]" />
		<input
			type="text"
			bind:value={mpQuery}
			placeholder="Search ClawHub marketplace..."
			aria-label="Search marketplace"
			class="w-full rounded-lg border border-border-subtle bg-[var(--bg-1)] pl-9 pr-8 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none focus:ring-1 focus:ring-[var(--cairn-accent)]/30"
		/>
		{#if mpQuery}
			<button
				class="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
				onclick={() => { mpQuery = ''; mpResults = []; }}
				type="button"
				aria-label="Clear search"
			>
				<X class="h-3.5 w-3.5" />
			</button>
		{/if}
	</div>

	<!-- Sort bar (only when not searching) -->
	{#if !mpQuery.trim()}
		<div class="mb-4 flex items-center gap-2">
			<span class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Sort:</span>
			{#each ['trending', 'downloads', 'stars', 'updated'] as sortOpt}
				<button
					class="px-2 py-0.5 text-[10px] rounded-full border transition-colors {mpSort === sortOpt ? 'bg-[var(--cairn-accent)]/10 text-[var(--cairn-accent)] border-[var(--cairn-accent)]/30' : 'text-[var(--text-tertiary)] border-border-subtle hover:text-[var(--text-secondary)]'}"
					onclick={() => { mpSort = sortOpt as typeof mpSort; mpBrowseLoad(); }}
				>
					{sortOpt}
				</button>
			{/each}
		</div>
	{/if}

	{#if mpLoading}
		<div class="flex flex-col gap-2">
			{#each Array(6) as _, i}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 animate-in" style="animation-delay: {i * 40}ms">
					<Skeleton class="h-4 w-40 mb-1" />
					<Skeleton class="h-3 w-64" />
				</div>
			{/each}
		</div>
	{:else}
		{#if mpDisplayItems().length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-[var(--text-tertiary)]">
				<Store class="mb-3 h-10 w-10 opacity-30" />
				<p class="text-sm">{mpQuery.trim() ? `No skills match "${mpQuery}"` : 'Browse ClawHub skills'}</p>
				<p class="mt-1 text-xs opacity-60">Search by keyword or browse by category</p>
			</div>
		{:else}
			<div class="flex flex-col gap-2">
				{#each mpDisplayItems() as item, i (item.slug)}
					<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 card-hover animate-in" style="animation-delay: {i * 25}ms">
						<div class="flex items-start gap-3">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2 mb-1">
									<p class="text-sm font-medium text-[var(--text-primary)]">{item.name}</p>
									<span class="text-[10px] text-[var(--text-tertiary)] font-mono">{item.slug}</span>
									{#if item.version}
										<Badge variant="outline" class="h-4 px-1 text-[10px]">v{item.version}</Badge>
									{/if}
									{#if item.installed}
										<Badge variant="outline" class="h-4 px-1 text-[10px] text-[var(--color-success)]">
											<Check class="h-2.5 w-2.5 mr-0.5" /> installed
										</Badge>
									{/if}
								</div>
								<p class="text-xs text-[var(--text-secondary)] line-clamp-2">{item.summary}</p>
								<div class="flex items-center gap-3 mt-1.5">
									{#if item.downloads}
										<span class="flex items-center gap-1 text-[10px] text-[var(--text-tertiary)]">
											<Download class="h-3 w-3" /> {item.downloads.toLocaleString()}
										</span>
									{/if}
									{#if item.stars}
										<span class="flex items-center gap-1 text-[10px] text-[var(--text-tertiary)]">
											<Star class="h-3 w-3" /> {item.stars}
										</span>
									{/if}
								</div>
							</div>
							<div class="flex gap-1.5 flex-shrink-0">
								<Button variant="outline" size="sm" class="h-6 text-[10px] px-2" onclick={() => showMpPreview(item.slug)}>
									<FileText class="h-3 w-3" />
								</Button>
								{#if item.installed}
									<Button variant="outline" size="sm" class="h-6 text-[10px] px-2" disabled>
										<Check class="h-3 w-3" />
									</Button>
								{:else}
									<Button size="sm" class="h-6 text-[10px] px-2 gap-1" onclick={() => handleInstallClick(item.slug)} disabled={installing === item.slug || reviewSlug === item.slug}>
										{#if installing === item.slug}
											<Loader2 class="h-3 w-3 animate-spin" />
										{:else}
											<Download class="h-3 w-3" />
										{/if}
										Install
									</Button>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/if}

	{/if}
</div>

<!-- Marketplace preview dialog -->
{#if mpPreviewSlug}
	<Dialog.Root open={!!mpPreviewSlug} onOpenChange={(open) => { if (!open) mpPreviewSlug = null; }}>
		<Dialog.Content class="sm:max-w-2xl max-h-[80vh] overflow-y-auto bg-[var(--bg-0)] border-border-subtle">
			<Dialog.Header>
				<Dialog.Title class="text-[var(--text-primary)]">{mpPreviewSlug}</Dialog.Title>
				<Dialog.Description class="text-[var(--text-tertiary)] text-xs">SKILL.md from ClawHub</Dialog.Description>
			</Dialog.Header>
			{#if mpPreviewLoading}
				<div class="flex items-center gap-2 text-xs text-[var(--text-tertiary)] py-8 justify-center">
					<Loader2 class="h-4 w-4 animate-spin" /> Loading preview...
				</div>
			{:else if mpPreviewContent}
				<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">
					{@html renderMarkdown(mpPreviewContent)}
				</div>
			{:else}
				<p class="text-xs text-[var(--text-tertiary)] py-4">No preview available.</p>
			{/if}
		</Dialog.Content>
	</Dialog.Root>
{/if}

<!-- Security review dialog -->
{#if reviewSlug}
	<Dialog.Root open={!!reviewSlug} onOpenChange={(open) => { if (!open) { reviewSlug = null; reviewResult = null; } }}>
		<Dialog.Content class="sm:max-w-lg bg-[var(--bg-0)] border-border-subtle">
			<Dialog.Header>
				<Dialog.Title class="text-[var(--text-primary)] flex items-center gap-2">
					<ShieldAlert class="h-5 w-5 text-[var(--color-warning)]" /> Security Review
				</Dialog.Title>
				<Dialog.Description class="text-[var(--text-tertiary)] text-xs">Reviewing {reviewSlug} before install</Dialog.Description>
			</Dialog.Header>
			{#if reviewLoading}
				<div class="flex items-center gap-3 py-8 justify-center">
					<Loader2 class="h-5 w-5 animate-spin text-[var(--cairn-accent)]" />
					<span class="text-sm text-[var(--text-secondary)]">Analyzing skill for security risks...</span>
				</div>
			{:else if reviewResult}
				<div class="space-y-4 py-2">
					<!-- Verdict -->
					<div class="flex items-center gap-3 p-3 rounded-lg {reviewResult.safe ? 'bg-[var(--color-success)]/10 border border-[var(--color-success)]/20' : 'bg-[var(--color-error)]/10 border border-[var(--color-error)]/20'}">
						{#if reviewResult.safe}
							<ShieldCheck class="h-6 w-6 text-[var(--color-success)]" />
							<div>
								<p class="text-sm font-medium text-[var(--color-success)]">Safe to install</p>
								<p class="text-xs text-[var(--text-secondary)]">{reviewResult.summary}</p>
							</div>
						{:else}
							<ShieldAlert class="h-6 w-6 text-[var(--color-error)]" />
							<div>
								<p class="text-sm font-medium text-[var(--color-error)]">Security concerns found</p>
								<p class="text-xs text-[var(--text-secondary)]">{reviewResult.summary}</p>
							</div>
						{/if}
					</div>

					<!-- Risk level -->
					<div class="flex items-center gap-2">
						<span class="text-[10px] uppercase tracking-wider text-[var(--text-tertiary)]">Risk:</span>
						<Badge variant="outline" class="text-[10px] {
							reviewResult.risk === 'none' || reviewResult.risk === 'low' ? 'text-[var(--color-success)]' :
							reviewResult.risk === 'medium' ? 'text-[var(--color-warning)]' :
							'text-[var(--color-error)]'
						}">{reviewResult.risk}</Badge>
					</div>

					<!-- Issues -->
					{#if reviewResult.issues && reviewResult.issues.length > 0}
						<div>
							<p class="text-[10px] uppercase tracking-wider text-[var(--text-tertiary)] mb-1">Issues</p>
							<ul class="space-y-1">
								{#each reviewResult.issues as issue}
									<li class="text-xs text-[var(--text-secondary)] flex items-start gap-1.5">
										<span class="text-[var(--color-warning)] mt-0.5">-</span>
										{issue}
									</li>
								{/each}
							</ul>
						</div>
					{/if}

					<!-- Actions -->
					<div class="flex justify-end gap-2 pt-2">
						<Button variant="outline" size="sm" class="text-xs" onclick={() => { reviewSlug = null; reviewResult = null; }}>
							Cancel
						</Button>
						<Button size="sm" class="text-xs gap-1 {reviewResult.safe ? '' : 'bg-[var(--color-error)] hover:bg-[var(--color-error)]/90'}" onclick={() => confirmInstall(reviewSlug!)}>
							<Download class="h-3 w-3" />
							{reviewResult.safe ? 'Install' : 'Install anyway'}
						</Button>
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Root>
{/if}

<!-- Skill detail dialog -->
<Dialog.Root bind:open={dialogOpen}>
	<Dialog.Content class="sm:max-w-2xl max-h-[80vh] overflow-y-auto bg-[var(--bg-0)] border-border-subtle">
		<Dialog.Header>
			<Dialog.Title class="text-[var(--text-primary)]">{dialogSkill?.name}</Dialog.Title>
			<Dialog.Description class="text-[var(--text-tertiary)] text-xs">Full SKILL.md content</Dialog.Description>
		</Dialog.Header>
		{#if dialogSkill?.content}
			<div class="cairn-prose text-sm text-[var(--text-primary)] leading-relaxed">
				{@html renderMarkdown(dialogSkill.content)}
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>

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
	.cairn-prose :global(h1), .cairn-prose :global(h2), .cairn-prose :global(h3) {
		font-weight: 600; margin: 0.75em 0 0.25em; color: var(--text-primary);
	}
</style>
