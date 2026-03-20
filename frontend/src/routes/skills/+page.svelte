<script lang="ts">
	import { onMount } from 'svelte';
	import { getSkills, getSkillDetail, createSkillApi, updateSkillApi, deleteSkillApi } from '$lib/api/client';
	import { renderMarkdown } from '$lib/utils/markdown';
	import type { Skill } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Sparkles, Search, X, ChevronDown, ChevronUp, FileText, Loader2, Plus, Pencil, Trash2, Save } from '@lucide/svelte';

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

	const inclusionColors: Record<string, string> = {
		always: 'text-[var(--color-success)]',
		auto: 'text-[var(--cairn-accent)]',
		manual: 'text-[var(--color-warning)]',
	};
</script>

<div class="mx-auto max-w-5xl p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Skills</h1>
		<div class="flex items-center gap-3">
			{#if skills.length > 0}
				<span class="text-[11px] text-[var(--text-tertiary)] font-mono tabular-nums">
					{activeSkills.length} active / {skills.length} loaded
				</span>
			{/if}
			<Button size="sm" class="h-7 text-xs gap-1" onclick={() => showCreate = !showCreate}>
				<Plus class="h-3 w-3" /> New Skill
			</Button>
		</div>
	</div>

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
					<button
						class="flex w-full items-center gap-3 p-3 text-left"
						onclick={() => toggleExpanded(skill.name)}
						type="button"
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
								<Badge variant="outline" class="h-4 px-1 text-[10px] {inclusionColors[skill.inclusion] ?? 'text-[var(--text-tertiary)]'}">
									{skill.inclusion}
								</Badge>
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
					</button>

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
</div>

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
