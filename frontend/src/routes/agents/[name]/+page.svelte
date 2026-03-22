<script lang="ts">
	import { onMount } from 'svelte';
	import { getAgentType, deleteAgentType, updateAgentType, runAgentType, getSkills, type AgentTypeDetail } from '$lib/api/client';
	import type { Skill } from '$lib/types';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { renderMarkdown } from '$lib/utils/markdown';
	import { ArrowLeft, Trash2, Pencil, Save, X, Play, Loader2, CheckCircle } from '@lucide/svelte';

	let { data } = $props<{ data: { name: string } }>();
	let agentType = $state<AgentTypeDetail | null>(null);
	let loading = $state(true);
	let error = $state('');
	let deleting = $state(false);
	let editing = $state(false);
	let saving = $state(false);

	// Run form state
	let showRunForm = $state(false);
	let runInstruction = $state('');
	let runExecMode = $state<'background'>('background');
	let running = $state(false);
	let runResult = $state<{ taskId: string } | null>(null);

	// Edit form state
	let editDescription = $state('');
	let editMode = $state('work');
	let editMaxRounds = $state(80);
	let editWorktree = $state(false);
	let editDeniedTools = $state('');
	let editSkills = $state<Set<string>>(new Set());
	let editContent = $state('');

	// Available skills for the selector
	let availableSkills = $state<Skill[]>([]);

	onMount(async () => {
		try {
			const [at, skillsRes] = await Promise.all([
				getAgentType(data.name),
				getSkills().catch(() => ({ items: [] as Skill[], summary: {}, currentlyActive: [] as string[] })),
			]);
			agentType = at;
			availableSkills = skillsRes.items ?? [];
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load agent type';
		} finally {
			loading = false;
		}
	});

	const modeColors: Record<string, string> = {
		talk: 'var(--color-info)',
		work: 'var(--color-warning)',
		coding: 'var(--color-success)',
	};

	function startEdit() {
		if (!agentType) return;
		editDescription = agentType.description;
		editMode = agentType.mode;
		editMaxRounds = agentType.maxRounds;
		editWorktree = agentType.worktree;
		editDeniedTools = agentType.deniedTools?.join(', ') ?? '';
		editSkills = new Set(agentType.skills ?? []);
		editContent = agentType.content;
		editing = true;
	}

	async function handleSave() {
		if (!agentType) return;
		saving = true;
		error = '';
		try {
			const denied = editDeniedTools.split(',').map(s => s.trim()).filter(Boolean);
			const skills = [...editSkills];
			await updateAgentType(agentType.name, {
				description: editDescription,
				mode: editMode,
				maxRounds: editMaxRounds,
				worktree: editWorktree,
				deniedTools: denied,
				skills: skills,
				content: editContent,
			});
			agentType = await getAgentType(data.name);
			editing = false;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function handleRun() {
		if (!agentType || !runInstruction.trim()) return;
		running = true;
		error = '';
		runResult = null;
		try {
			const result = await runAgentType(agentType.name, {
				instruction: runInstruction.trim(),
				execMode: runExecMode,
			});
			runResult = { taskId: result.taskId };
			runInstruction = '';
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Run failed';
		} finally {
			running = false;
		}
	}

	async function handleDelete() {
		if (!agentType || !confirm(`Delete agent type "${agentType.name}"? This cannot be undone.`)) return;
		deleting = true;
		try {
			await deleteAgentType(agentType.name);
			window.location.href = '/agents';
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			deleting = false;
		}
	}
</script>

<div class="mx-auto max-w-4xl px-4 py-4 sm:p-6">
	<a href="/agents" class="mb-4 inline-flex items-center gap-1.5 text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] transition-colors">
		<ArrowLeft class="h-3 w-3" />
		Back to Agents
	</a>

	{#if loading}
		<div class="mt-4">
			<Skeleton class="h-8 w-48 mb-2" />
			<Skeleton class="h-4 w-96 mb-6" />
			<Skeleton class="h-64 w-full rounded-lg" />
		</div>
	{:else if error && !agentType}
		<div class="mt-4 rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-sm text-red-400">
			{error}
		</div>
	{:else if agentType}
		<div class="mt-2">
			{#if error}
				<div class="mb-4 rounded-lg border border-red-500/20 bg-red-500/5 p-3 text-xs text-red-400">
					{error}
				</div>
			{/if}

			<!-- Header -->
			<div class="flex items-start justify-between mb-6">
				<div>
					<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">{agentType.name}</h1>
					{#if !editing}
						<p class="mt-1 text-sm text-[var(--text-secondary)]">{agentType.description}</p>
						<div class="mt-2 flex items-center gap-2">
							<Badge variant="outline" class="text-[10px]" style="border-color: {modeColors[agentType.mode] ?? 'var(--border-subtle)'}; color: {modeColors[agentType.mode] ?? 'var(--text-tertiary)'}">
								{agentType.mode} mode
							</Badge>
							<Badge variant="outline" class="text-[10px]">{agentType.maxRounds} rounds</Badge>
							{#if agentType.worktree}
								<Badge variant="outline" class="text-[10px]">worktree</Badge>
							{/if}
							{#if agentType.model && agentType.model !== 'default'}
								<Badge variant="outline" class="text-[10px]">model: {agentType.model}</Badge>
							{/if}
						</div>
					{/if}
				</div>
				<div class="flex items-center gap-1">
					{#if !editing}
						<button
							class="rounded-md p-2 text-[var(--text-tertiary)] hover:text-[var(--color-success)] hover:bg-[var(--color-success)]/10 transition-colors"
							onclick={() => { showRunForm = !showRunForm; runResult = null; }}
							title="Run agent type"
						>
							<Play class="h-4 w-4" />
						</button>
						<button
							class="rounded-md p-2 text-[var(--text-tertiary)] hover:text-[var(--cairn-accent)] hover:bg-[var(--bg-2)] transition-colors"
							onclick={startEdit}
							title="Edit agent type"
						>
							<Pencil class="h-4 w-4" />
						</button>
					{/if}
					<button
						class="rounded-md p-2 text-[var(--text-tertiary)] hover:text-red-400 hover:bg-red-500/10 transition-colors"
						onclick={handleDelete}
						disabled={deleting}
						title="Delete agent type"
					>
						<Trash2 class="h-4 w-4" />
					</button>
				</div>
			</div>

			{#if editing}
				<!-- Edit Mode -->
				<div class="space-y-4">
					<div>
						<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Description</p>
						<textarea
							bind:value={editDescription}
							class="w-full rounded-md border border-border-subtle bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none resize-y h-16"
						></textarea>
					</div>

					<div class="grid grid-cols-3 gap-4">
						<div>
							<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Mode</p>
							<select
								bind:value={editMode}
								class="w-full h-8 rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none"
							>
								<option value="talk">talk</option>
								<option value="work">work</option>
								<option value="coding">coding</option>
							</select>
						</div>
						<div>
							<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Max Rounds</p>
							<input
								type="number"
								bind:value={editMaxRounds}
								min="1"
								max="400"
								class="w-full h-8 rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none"
							/>
						</div>
						<div class="flex items-end">
							<label for="edit-worktree" class="flex items-center gap-2 text-sm text-[var(--text-primary)] cursor-pointer">
								<input id="edit-worktree" type="checkbox" bind:checked={editWorktree} class="rounded border-border-subtle" />
								Worktree isolation
							</label>
						</div>
					</div>

					<div>
						<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Denied Tools (comma-separated)</p>
						<input
							type="text"
							bind:value={editDeniedTools}
							placeholder="cairn.shell, cairn.writeFile"
							class="w-full h-8 rounded-md border border-border-subtle bg-[var(--bg-1)] px-3 text-sm text-[var(--text-primary)] font-mono focus:border-[var(--cairn-accent)] focus:outline-none"
						/>
					</div>

				{#if availableSkills.length > 0}
					<div>
						<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1.5">Pre-loaded Skills</p>
						<div class="flex flex-wrap gap-2">
							{#each availableSkills as sk (sk.name)}
								<button
									type="button"
									class="rounded-md px-2.5 py-1 text-[11px] border transition-colors {editSkills.has(sk.name) ? 'border-[var(--cairn-accent)] text-[var(--cairn-accent)] bg-[var(--cairn-accent)]/10' : 'border-border-subtle text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
									onclick={() => { const next = new Set(editSkills); if (next.has(sk.name)) next.delete(sk.name); else next.add(sk.name); editSkills = next; }}
									title={sk.description}
								>
									{sk.name}
								</button>
							{/each}
						</div>
					</div>
				{/if}

					<div>
						<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1">System Prompt</p>
						<textarea
							bind:value={editContent}
							class="w-full rounded-md border border-border-subtle bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono focus:border-[var(--cairn-accent)] focus:outline-none resize-y"
							style="min-height: 200px"
						></textarea>
					</div>

					<div class="flex gap-2 justify-end">
						<button
							class="inline-flex items-center gap-1.5 rounded-md border border-border-subtle bg-[var(--bg-1)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-2)] transition-colors"
							onclick={() => { editing = false; error = ''; }}
						>
							<X class="h-3 w-3" /> Cancel
						</button>
						<button
							class="inline-flex items-center gap-1.5 rounded-md bg-[var(--cairn-accent)] px-3 py-1.5 text-xs text-white hover:opacity-90 transition-opacity disabled:opacity-50"
							onclick={handleSave}
							disabled={saving}
						>
							<Save class="h-3 w-3" /> {saving ? 'Saving...' : 'Save'}
						</button>
					</div>
				</div>
			{:else}
				<!-- Run Form -->
				{#if showRunForm}
					<div class="mb-4 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
						<p class="text-sm font-medium text-[var(--text-primary)]">Run {agentType.name}</p>

						<div>
							<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Instruction</p>
							<textarea
								bind:value={runInstruction}
								placeholder="What should this agent do?"
								class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none resize-y h-20"
							></textarea>
						</div>

						<div class="flex items-center gap-4">
							<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Exec Mode</p>
							<div class="flex gap-2">
								<button
									class="rounded-md px-2.5 py-1 text-[10px] border transition-colors border-[var(--cairn-accent)] text-[var(--cairn-accent)] bg-[var(--cairn-accent)]/10"
									onclick={() => runExecMode = 'background'}
								>
									Background
								</button>
							</div>
						</div>

						{#if runResult}
							<div class="rounded-md border border-[var(--color-success)]/20 bg-[var(--color-success)]/5 p-3 text-xs">
								<div class="flex items-center gap-2 text-[var(--color-success)]">
									<CheckCircle class="h-3.5 w-3.5" />
									Agent spawned successfully
								</div>
								<a
									href="/ops"
									class="mt-1 inline-block text-[var(--cairn-accent)] hover:underline"
								>
									View task {runResult.taskId}
								</a>
							</div>
						{/if}

						<div class="flex justify-end">
							<button
								class="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-success)] px-3 py-1.5 text-xs text-white hover:opacity-90 transition-opacity disabled:opacity-50"
								onclick={handleRun}
								disabled={running || !runInstruction.trim()}
							>
								{#if running}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Play class="h-3 w-3" />{/if}
								Run
							</button>
						</div>
					</div>
				{/if}

				<!-- View Mode -->
				<!-- Tool access -->
				{#if agentType.deniedTools && agentType.deniedTools.length > 0}
					<div class="mb-4 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
						<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1.5">Denied Tools</p>
						<div class="flex flex-wrap gap-1">
							{#each agentType.deniedTools as t}
								<span class="rounded bg-red-500/10 px-1.5 py-0.5 text-[10px] font-mono text-red-400">{t}</span>
							{/each}
						</div>
					</div>
				{/if}

			<!-- Pre-loaded skills -->
			{#if agentType.skills && agentType.skills.length > 0}
				<div class="mb-4 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
					<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-1.5">Pre-loaded Skills</p>
					<div class="flex flex-wrap gap-1">
						{#each agentType.skills as s}
							<a href="/skills/{s}" class="rounded bg-[var(--cairn-accent)]/10 px-1.5 py-0.5 text-[10px] font-mono text-[var(--cairn-accent)] hover:bg-[var(--cairn-accent)]/20 transition-colors">{s}</a>
						{/each}
					</div>
				</div>
			{/if}

				<!-- Content / System Prompt -->
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
					<p class="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider mb-3">System Prompt</p>
					<div class="prose prose-sm prose-invert max-w-none text-[var(--text-secondary)]">
						{@html renderMarkdown(agentType.content)}
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>
