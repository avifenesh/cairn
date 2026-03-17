<script lang="ts">
	import { onMount } from 'svelte';
	import { getTasks, getApprovals, cancelTask, approve, deny } from '$lib/api/client';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import TaskCard from '$lib/components/tasks/TaskCard.svelte';
	import ApprovalCard from '$lib/components/tasks/ApprovalCard.svelte';

	let tab = $state<'approvals' | 'tasks'>('approvals');
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
</script>

<div class="mx-auto max-w-4xl p-6">
	<h1 class="mb-6 text-2xl font-semibold text-[var(--text-primary)]">Ops Inbox</h1>

	<!-- Tab bar -->
	<div class="mb-6 flex gap-1 rounded-lg bg-[var(--bg-2)] p-1">
		<button
			class="flex-1 rounded-md px-3 py-1.5 text-sm transition-colors duration-[var(--dur-fast)]
				{tab === 'approvals' ? 'bg-[var(--bg-1)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-secondary)]'}"
			onclick={() => (tab = 'approvals')}
		>
			Approvals
			{#if taskStore.pendingApprovals.length > 0}
				<span class="ml-1.5 inline-flex min-w-[18px] items-center justify-center rounded-full bg-[var(--pub-accent)] px-1 text-[10px] font-medium text-[var(--primary-foreground)]">
					{taskStore.pendingApprovals.length}
				</span>
			{/if}
		</button>
		<button
			class="flex-1 rounded-md px-3 py-1.5 text-sm transition-colors duration-[var(--dur-fast)]
				{tab === 'tasks' ? 'bg-[var(--bg-1)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-secondary)]'}"
			onclick={() => (tab = 'tasks')}
		>
			Tasks
		</button>
	</div>

	<!-- Bulk actions bar -->
	{#if tab === 'approvals' && selectedIds.size > 0}
		<div class="mb-4 flex items-center gap-3 rounded-lg bg-[var(--bg-2)] px-4 py-2">
			<span class="text-xs text-[var(--text-secondary)]">{selectedIds.size} selected</span>
			<button
				class="rounded-md bg-[var(--color-success)]/10 px-3 py-1 text-xs font-medium text-[var(--color-success)] hover:bg-[var(--color-success)]/20 transition-colors"
				onclick={bulkApprove}
			>
				Approve all
			</button>
			<button
				class="rounded-md bg-[var(--color-error)]/10 px-3 py-1 text-xs font-medium text-[var(--color-error)] hover:bg-[var(--color-error)]/20 transition-colors"
				onclick={bulkDeny}
			>
				Deny all
			</button>
			<button
				class="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
				onclick={() => (selectedIds = new Set())}
			>
				Clear
			</button>
		</div>
	{/if}

	{#if taskStore.loading}
		<div class="flex flex-col gap-3">
			{#each Array(3) as _}
				<div class="h-20 animate-pulse rounded-lg bg-[var(--bg-2)]"></div>
			{/each}
		</div>
	{:else if tab === 'approvals'}
		{#if taskStore.pendingApprovals.length === 0}
			<p class="py-12 text-center text-sm text-[var(--text-tertiary)]">No pending approvals</p>
		{:else}
			<div class="flex flex-col gap-3">
				{#each taskStore.pendingApprovals as approval (approval.id)}
					<ApprovalCard
						{approval}
						onapprove={handleApprove}
						ondeny={handleDeny}
						selected={selectedIds.has(approval.id)}
						onselect={toggleSelect}
					/>
				{/each}
			</div>
		{/if}
	{:else}
		{#if taskStore.tasks.length === 0}
			<p class="py-12 text-center text-sm text-[var(--text-tertiary)]">No tasks</p>
		{:else}
			<div class="flex flex-col gap-2">
				{#each taskStore.tasks as task (task.id)}
					<TaskCard {task} oncancel={handleCancel} />
				{/each}
			</div>
		{/if}
	{/if}
</div>
