<script lang="ts">
	import { onMount } from 'svelte';
	import { getCrons, createCron, updateCron, deleteCron, getCronDetail } from '$lib/api/client';
	import type { CronJob, CronExecution } from '$lib/types';
	import { relativeTime } from '$lib/utils/time';
	import { cronToHuman } from '$lib/utils/cron';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import { Clock, Plus, Trash2, ChevronDown, ChevronUp, Play, Pause, Loader2, Calendar } from '@lucide/svelte';

	let jobs = $state<CronJob[]>([]);
	let loading = $state(true);
	let expandedId = $state<string | null>(null);
	let executions = $state<CronExecution[]>([]);
	let loadingExecs = $state(false);

	// Create form state
	let newName = $state('');
	let newSchedule = $state('');
	let newInstruction = $state('');
	let newPriority = $state(3);
	let newTimezone = $state('UTC');
	let creating = $state(false);
	let createError = $state('');

	const PRIORITIES = [
		{ value: 0, label: 'Critical' },
		{ value: 1, label: 'High' },
		{ value: 2, label: 'Normal' },
		{ value: 3, label: 'Low' },
		{ value: 4, label: 'Idle' },
	];

	const statusColors: Record<string, string> = {
		fired: 'var(--cairn-accent)',
		completed: 'var(--color-success)',
		failed: 'var(--color-error)',
		skipped_cooldown: 'var(--color-warning)',
	};

	onMount(async () => {
		try {
			const res = await getCrons();
			jobs = res.items ?? [];
		} catch (e) {
			console.error('Failed to load cron jobs:', e);
		} finally {
			loading = false;
		}
	});

	async function handleCreate() {
		if (!newName.trim() || !newSchedule.trim() || !newInstruction.trim()) {
			createError = 'Name, schedule, and instruction are required';
			return;
		}
		creating = true;
		createError = '';
		try {
			const job = await createCron({
				name: newName.trim(),
				schedule: newSchedule.trim(),
				instruction: newInstruction.trim(),
				priority: newPriority,
				timezone: newTimezone.trim() || 'UTC',
			});
			jobs = [job, ...jobs];
			newName = '';
			newSchedule = '';
			newInstruction = '';
			newPriority = 3;
		} catch (e) {
			createError = e instanceof Error ? e.message : 'Failed to create';
		} finally {
			creating = false;
		}
	}

	async function toggleEnabled(job: CronJob) {
		try {
			const res = await updateCron(job.id, { enabled: !job.enabled });
			jobs = jobs.map((j) => (j.id === job.id ? res.job : j));
		} catch (e) {
			console.error('Failed to toggle cron job:', e);
		}
	}

	async function handleDelete(id: string) {
		try {
			await deleteCron(id);
			jobs = jobs.filter((j) => j.id !== id);
			if (expandedId === id) expandedId = null;
		} catch (e) {
			console.error('Failed to delete cron job:', e);
		}
	}

	async function toggleExpand(id: string) {
		if (expandedId === id) {
			expandedId = null;
			return;
		}
		expandedId = id;
		loadingExecs = true;
		try {
			const detail = await getCronDetail(id);
			executions = detail.executions ?? [];
		} catch (e) {
			console.error('Failed to load cron executions:', e);
			executions = [];
		} finally {
			loadingExecs = false;
		}
	}

	const schedulePreview = $derived(
		newSchedule.trim() ? cronToHuman(newSchedule.trim()) : ''
	);
</script>

<div class="space-y-3">
	<!-- Job list -->
	{#if loading}
		<div class="flex items-center gap-2 py-8 justify-center text-[var(--text-tertiary)]">
			<Loader2 class="h-4 w-4 animate-spin" />
			<span class="text-xs">Loading jobs...</span>
		</div>
	{:else if jobs.length === 0}
		<div class="flex flex-col items-center py-8 text-[var(--text-tertiary)]">
			<Clock class="h-8 w-8 mb-2 opacity-40" />
			<p class="text-xs">No scheduled jobs</p>
			<p class="text-[10px] opacity-60 mt-0.5">Create one below to automate recurring tasks</p>
		</div>
	{:else}
		{#each jobs as job (job.id)}
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-0)]">
				<!-- Job row -->
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-[var(--bg-1)] transition-colors rounded-lg cursor-pointer"
					onclick={() => toggleExpand(job.id)}
					role="button"
					tabindex="0"
					onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleExpand(job.id); }}
				>
					<!-- Enable/disable toggle -->
					<button
						class="flex-shrink-0 rounded-md p-1 transition-colors
							{job.enabled ? 'text-[var(--color-success)] hover:text-[var(--color-success)]/80' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
						title={job.enabled ? 'Disable' : 'Enable'}
						onclick={(e) => { e.stopPropagation(); toggleEnabled(job); }}
					>
						{#if job.enabled}
							<Play class="h-3.5 w-3.5" />
						{:else}
							<Pause class="h-3.5 w-3.5" />
						{/if}
					</button>

					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2">
							<span class="text-sm font-medium text-[var(--text-primary)] truncate">{job.name}</span>
							<Badge variant="outline" class="h-4 px-1 text-[10px] font-mono border-border-subtle">
								{cronToHuman(job.schedule)}
							</Badge>
						</div>
						<div class="flex items-center gap-2 mt-0.5 text-[10px] text-[var(--text-tertiary)]">
							{#if job.nextRunAt}
								<span>Next: {relativeTime(job.nextRunAt)}</span>
							{/if}
							{#if job.lastRunAt}
								<span>&middot; Last: {relativeTime(job.lastRunAt)}</span>
							{/if}
							{#if !job.enabled}
								<Badge variant="outline" class="h-3.5 px-1 text-[9px] text-[var(--color-warning)] border-[var(--color-warning)]/30">
									paused
								</Badge>
							{/if}
						</div>
					</div>

					<button
						class="flex-shrink-0 rounded-md p-1 text-[var(--text-tertiary)] hover:text-[var(--color-error)] hover:bg-[var(--bg-2)] transition-colors"
						title="Delete"
						onclick={(e) => { e.stopPropagation(); handleDelete(job.id); }}
					>
						<Trash2 class="h-3.5 w-3.5" />
					</button>

					{#if expandedId === job.id}
						<ChevronUp class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
					{:else}
						<ChevronDown class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
					{/if}
				</div>

				<!-- Expanded details -->
				{#if expandedId === job.id}
					<div class="px-4 pb-4 pt-1 border-t border-border-subtle">
						<!-- Instruction -->
						<div class="mb-3">
							<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Instruction</p>
							<p class="text-xs text-[var(--text-secondary)] bg-[var(--bg-1)] rounded-md px-3 py-2 font-mono whitespace-pre-wrap">{job.instruction}</p>
						</div>

						<!-- Details grid -->
						<div class="grid grid-cols-4 gap-3 mb-3">
							<div>
								<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Schedule</p>
								<p class="text-xs text-[var(--text-primary)] font-mono">{job.schedule}</p>
							</div>
							<div>
								<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Priority</p>
								<p class="text-xs text-[var(--text-primary)]">{PRIORITIES.find(p => p.value === job.priority)?.label ?? job.priority}</p>
							</div>
							<div>
								<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Timezone</p>
								<p class="text-xs text-[var(--text-primary)] font-mono">{job.timezone}</p>
							</div>
							<div>
								<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Cooldown</p>
								<p class="text-xs text-[var(--text-primary)]">{job.cooldownMs >= 60000 ? Math.round(job.cooldownMs / 60000) + 'min' : Math.round(job.cooldownMs / 1000) + 's'}</p>
							</div>
						</div>

						<!-- Execution history -->
						<div>
							<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Recent Executions</p>
							{#if loadingExecs}
								<div class="flex items-center gap-1 text-[10px] text-[var(--text-tertiary)] py-2">
									<Loader2 class="h-3 w-3 animate-spin" /> Loading...
								</div>
							{:else if executions.length === 0}
								<p class="text-[10px] text-[var(--text-tertiary)] py-2">No executions yet</p>
							{:else}
								<div class="space-y-1">
									{#each executions as exec}
										<div class="flex items-center gap-2 text-[10px]">
											<span
												class="h-1.5 w-1.5 rounded-full flex-shrink-0"
												style="background: {statusColors[exec.status] ?? 'var(--text-tertiary)'}"
											></span>
											<span class="text-[var(--text-secondary)]">{exec.status}</span>
											<span class="text-[var(--text-tertiary)]">{relativeTime(exec.createdAt)}</span>
											{#if exec.error}
												<span class="text-[var(--color-error)] truncate">{exec.error}</span>
											{/if}
										</div>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/each}
	{/if}

	<Separator />

	<!-- Create form -->
	<div class="rounded-lg border border-border-subtle bg-[var(--bg-0)] p-4 space-y-3">
		<div class="flex items-center gap-2 mb-1">
			<Plus class="h-4 w-4 text-[var(--cairn-accent)]" />
			<p class="text-sm font-medium text-[var(--text-primary)]">New Scheduled Job</p>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Name</p>
				<Input type="text" bind:value={newName} placeholder="Morning digest" class="h-7 text-xs" />
			</div>
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Schedule (cron)</p>
				<Input type="text" bind:value={newSchedule} placeholder="0 9 * * 1-5" class="h-7 text-xs font-mono" />
				{#if schedulePreview && schedulePreview !== newSchedule.trim()}
					<p class="text-[10px] text-[var(--cairn-accent)] mt-0.5 flex items-center gap-1">
						<Calendar class="h-3 w-3" /> {schedulePreview}
					</p>
				{/if}
			</div>
		</div>

		<div>
			<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Instruction</p>
			<textarea
				bind:value={newInstruction}
				placeholder="What should cairn do? (natural language)"
				class="w-full rounded-md border border-border-subtle bg-[var(--bg-1)] px-2.5 py-1.5 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/40 focus:border-[var(--cairn-accent)] focus:outline-none resize-none h-16"
			></textarea>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Priority</p>
				<select
					bind:value={newPriority}
					class="w-full h-7 rounded-md border border-border-subtle bg-[var(--bg-1)] px-2 text-xs text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none"
				>
					{#each PRIORITIES as p}
						<option value={p.value}>{p.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Timezone</p>
				<Input type="text" bind:value={newTimezone} placeholder="UTC" class="h-7 text-xs font-mono" />
			</div>
		</div>

		{#if createError}
			<p class="text-[10px] text-[var(--color-error)]">{createError}</p>
		{/if}

		<div class="flex justify-end">
			<Button
				size="sm"
				class="h-7 text-xs gap-1 px-3"
				onclick={handleCreate}
				disabled={creating}
			>
				{#if creating}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Plus class="h-3 w-3" />{/if}
				Create Job
			</Button>
		</div>
	</div>
</div>
