<script lang="ts">
	import { onMount } from 'svelte';
	import { getTasks, getApprovals, cancelTask, createTask, approve, deny } from '$lib/api/client';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import TaskCard from '$lib/components/tasks/TaskCard.svelte';
	import TaskCreateForm from '$lib/components/tasks/TaskCreateForm.svelte';
	import ApprovalCard from '$lib/components/tasks/ApprovalCard.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { ShieldCheck, ShieldX, X, Trash2 } from '@lucide/svelte';
	import SystemStatus from '$lib/components/shared/SystemStatus.svelte';

	let tab = $state<string>('approvals');
	let taskFilter = $state<'all' | 'active' | 'completed' | 'failed'>('all');
	let selectedIds = $state<Set<string>>(new Set());

	onMount(async () => {
		taskStore.setLoading(true);
		try {
			const [tasksRes, approvalsRes] = await Promise.all([
				getTasks(),
				getApprovals({ status: 'pending' }),
			]);
			taskStore.setTasks(tasksRes.items);
			taskStore.setApprovals(approvalsRes.items);
		} catch {
			// handled
		} finally {
			taskStore.setLoading(false);
		}
	});

	async function handleApprove(id: string) {
		taskStore.resolveApproval(id, 'approved');
		selectedIds.delete(id);
		selectedIds = new Set(selectedIds);
		await approve(id);
	}

	async function handleDeny(id: string) {
		taskStore.resolveApproval(id, 'denied');
		selectedIds.delete(id);
		selectedIds = new Set(selectedIds);
		await deny(id);
	}

	async function handleCancel(id: string) {
		await cancelTask(id);
	}

	function toggleSelect(id: string) {
		if (selectedIds.has(id)) {
			selectedIds.delete(id);
		} else {
			selectedIds.add(id);
		}
		selectedIds = new Set(selectedIds);
	}

	async function bulkApprove() {
		const ids = [...selectedIds];
		ids.forEach((id) => taskStore.resolveApproval(id, 'approved'));
		selectedIds = new Set();
		await Promise.all(ids.map((id) => approve(id)));
	}

	async function bulkDeny() {
		const ids = [...selectedIds];
		ids.forEach((id) => taskStore.resolveApproval(id, 'denied'));
		selectedIds = new Set();
		await Promise.all(ids.map((id) => deny(id)));
	}

	async function handleCreateTask(description: string, type: string, priority: number) {
		try {
			const task = await createTask(description, type, priority);
			taskStore.upsertTask(task);
		} catch (err) {
			console.error('[ops] Failed to create task:', err);
		}
	}

	function handleDeleteTask(id: string) {
		taskStore.removeTask(id);
	}

	function handleClearCompleted() {
		const completed = taskStore.tasks.filter((t) => t.status === 'completed' || t.status === 'cancelled');
		completed.forEach((t) => taskStore.removeTask(t.id));
	}

	const filteredTasks = $derived(() => {
		const tasks = taskStore.tasks;
		if (taskFilter === 'all') return tasks;
		if (taskFilter === 'active') return tasks.filter((t) => t.status === 'pending' || t.status === 'running');
		if (taskFilter === 'completed') return tasks.filter((t) => t.status === 'completed');
		if (taskFilter === 'failed') return tasks.filter((t) => t.status === 'failed' || t.status === 'cancelled');
		return tasks;
	});

	const completedCount = $derived(taskStore.tasks.filter((t) => t.status === 'completed' || t.status === 'cancelled').length);

	const taskFilters: Array<{ key: typeof taskFilter; label: string }> = [
		{ key: 'all', label: 'All' },
		{ key: 'active', label: 'Active' },
		{ key: 'completed', label: 'Completed' },
		{ key: 'failed', label: 'Failed' },
	];
</script>

<div class="mx-auto max-w-5xl p-6">
	<h1 class="mb-6 text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Ops</h1>

	<div class="mb-6">
		<SystemStatus />
	</div>

	<Tabs.Root bind:value={tab}>
		<Tabs.List class="mb-6">
			<Tabs.Trigger value="approvals" class="gap-1.5">
				Approvals
				{#if taskStore.pendingApprovals.length > 0}
					<Badge variant="default" class="h-4 min-w-4 px-1 text-[10px]">
						{taskStore.pendingApprovals.length}
					</Badge>
				{/if}
			</Tabs.Trigger>
			<Tabs.Trigger value="tasks">Tasks</Tabs.Trigger>
		</Tabs.List>

		<!-- Bulk actions bar -->
		{#if tab === 'approvals' && selectedIds.size > 0}
			<div class="mb-4 flex items-center gap-2 rounded-lg border border-border-subtle bg-[var(--bg-1)] px-4 py-2">
				<span class="text-xs text-[var(--text-secondary)] font-medium">{selectedIds.size} selected</span>
				<Button
					variant="outline"
					size="sm"
					class="h-7 text-xs gap-1 border-[var(--color-success)]/30 text-[var(--color-success)] hover:bg-[var(--color-success)]/10"
					onclick={bulkApprove}
				>
					<ShieldCheck class="h-3 w-3" /> Approve
				</Button>
				<Button
					variant="outline"
					size="sm"
					class="h-7 text-xs gap-1 border-[var(--color-error)]/30 text-[var(--color-error)] hover:bg-[var(--color-error)]/10"
					onclick={bulkDeny}
				>
					<ShieldX class="h-3 w-3" /> Deny
				</Button>
				<Button
					variant="ghost"
					size="sm"
					class="h-7 text-xs ml-auto"
					onclick={() => (selectedIds = new Set())}
				>
					<X class="h-3 w-3" /> Clear
				</Button>
			</div>
		{/if}

		<Tabs.Content value="approvals">
			{#if taskStore.loading}
				<div class="flex flex-col gap-3">
					{#each Array(3) as _, i}
						<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 animate-in" style="animation-delay: {i * 50}ms">
							<Skeleton class="h-4 w-48 mb-2" />
							<Skeleton class="h-3 w-32" />
						</div>
					{/each}
				</div>
			{:else if taskStore.pendingApprovals.length === 0}
				<div class="py-16 text-center">
					<p class="text-sm text-[var(--text-tertiary)]">No pending approvals</p>
					<p class="mt-1 text-xs text-[var(--text-tertiary)]/60">All clear — nothing needs your attention</p>
				</div>
			{:else}
				<div class="flex flex-col gap-3">
					{#each taskStore.pendingApprovals as approval, i (approval.id)}
						<div class="animate-in" style="animation-delay: {i * 40}ms">
							<ApprovalCard
								{approval}
								onapprove={handleApprove}
								ondeny={handleDeny}
								selected={selectedIds.has(approval.id)}
								onselect={toggleSelect}
							/>
						</div>
					{/each}
				</div>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="tasks">
			<div class="mb-4 flex items-center gap-2 flex-wrap">
				<TaskCreateForm oncreate={handleCreateTask} />
				<div class="flex items-center gap-1 ml-auto">
					{#each taskFilters as f}
						<Button
							variant={taskFilter === f.key ? 'secondary' : 'ghost'}
							size="sm"
							class="h-6 text-[10px] px-2 {taskFilter === f.key ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-tertiary)]'}"
							onclick={() => (taskFilter = f.key)}
						>
							{f.label}
						</Button>
					{/each}
				</div>
			</div>
			{#if completedCount > 0}
				<div class="mb-3 flex items-center gap-2">
					<Button
						variant="ghost"
						size="sm"
						class="h-6 text-[10px] gap-1 text-[var(--text-tertiary)] hover:text-[var(--color-error)]"
						onclick={handleClearCompleted}
					>
						<Trash2 class="h-3 w-3" /> Clear {completedCount} finished
					</Button>
				</div>
			{/if}
			{#if taskStore.loading}
				<div class="flex flex-col gap-2">
					{#each Array(3) as _, i}
						<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 animate-in" style="animation-delay: {i * 50}ms">
							<Skeleton class="h-4 w-40 mb-1" />
							<Skeleton class="h-3 w-24" />
						</div>
					{/each}
				</div>
			{:else if filteredTasks().length === 0}
				<div class="py-16 text-center">
					<p class="text-sm text-[var(--text-tertiary)]">
						{taskFilter === 'all' ? 'No tasks' : `No ${taskFilter} tasks`}
					</p>
					<p class="mt-1 text-xs text-[var(--text-tertiary)]/60">Tasks will appear here when the agent is working</p>
				</div>
			{:else}
				<div class="flex flex-col gap-2">
					{#each filteredTasks() as task, i (task.id)}
						<div class="animate-in" style="animation-delay: {Math.min(i * 20, 200)}ms">
							<TaskCard {task} oncancel={handleCancel} ondelete={handleDeleteTask} />
						</div>
					{/each}
				</div>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
