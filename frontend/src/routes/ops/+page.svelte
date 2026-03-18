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
	import { ShieldCheck, ShieldX, X } from '@lucide/svelte';
	import SystemStatus from '$lib/components/shared/SystemStatus.svelte';

	let tab = $state<string>('approvals');
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
		} catch {
			// handled
		}
	}
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
			<div class="mb-4">
				<TaskCreateForm oncreate={handleCreateTask} />
			</div>
			{#if taskStore.loading}
				<div class="flex flex-col gap-2">
					{#each Array(3) as _, i}
						<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3 animate-in" style="animation-delay: {i * 50}ms">
							<Skeleton class="h-4 w-40 mb-1" />
							<Skeleton class="h-3 w-24" />
						</div>
					{/each}
				</div>
			{:else if taskStore.tasks.length === 0}
				<div class="py-16 text-center">
					<p class="text-sm text-[var(--text-tertiary)]">No tasks</p>
					<p class="mt-1 text-xs text-[var(--text-tertiary)]/60">Tasks will appear here when the agent is working</p>
				</div>
			{:else}
				<div class="flex flex-col gap-2">
					{#each taskStore.tasks as task, i (task.id)}
						<div class="animate-in" style="animation-delay: {i * 30}ms">
							<TaskCard {task} oncancel={handleCancel} />
						</div>
					{/each}
				</div>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
</div>
