<script lang="ts">
	import { onMount } from 'svelte';
	import { getTasks, getApprovals, cancelTask, approve, deny } from '$lib/api/client';
	import { taskStore } from '$lib/stores/tasks.svelte';
	import { relativeTime } from '$lib/utils/time';
	import { CheckCircle, XCircle, Clock, Ban, Loader2 } from '@lucide/svelte';

	let tab = $state<'approvals' | 'tasks'>('approvals');

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
			// handled by error state
		} finally {
			taskStore.setLoading(false);
		}
	});

	async function handleApprove(id: string) {
		taskStore.resolveApproval(id, 'approved');
		await approve(id);
	}

	async function handleDeny(id: string) {
		taskStore.resolveApproval(id, 'denied');
		await deny(id);
	}

	async function handleCancel(id: string) {
		await cancelTask(id);
	}

	const statusIcon: Record<string, typeof CheckCircle> = {
		completed: CheckCircle,
		failed: XCircle,
		pending: Clock,
		running: Loader2,
		cancelled: Ban,
	};

	const statusColor: Record<string, string> = {
		completed: 'var(--color-success)',
		failed: 'var(--color-error)',
		pending: 'var(--color-warning)',
		running: 'var(--pub-accent)',
		cancelled: 'var(--text-tertiary)',
	};
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
					<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
						<div class="mb-2 flex items-start justify-between">
							<div>
								<p class="text-sm font-medium text-[var(--text-primary)]">{approval.title}</p>
								{#if approval.description}
									<p class="mt-1 text-xs text-[var(--text-secondary)]">{approval.description}</p>
								{/if}
							</div>
							<span class="text-xs text-[var(--text-tertiary)]">
								{relativeTime(approval.createdAt)}
							</span>
						</div>
						<div class="flex gap-2">
							<button
								class="rounded-md bg-[var(--color-success)]/10 px-3 py-1.5 text-xs font-medium text-[var(--color-success)] hover:bg-[var(--color-success)]/20 transition-colors"
								onclick={() => handleApprove(approval.id)}
							>
								Approve
							</button>
							<button
								class="rounded-md bg-[var(--color-error)]/10 px-3 py-1.5 text-xs font-medium text-[var(--color-error)] hover:bg-[var(--color-error)]/20 transition-colors"
								onclick={() => handleDeny(approval.id)}
							>
								Deny
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{:else}
		{#if taskStore.tasks.length === 0}
			<p class="py-12 text-center text-sm text-[var(--text-tertiary)]">No tasks</p>
		{:else}
			<div class="flex flex-col gap-2">
				{#each taskStore.tasks as task (task.id)}
					{@const Icon = statusIcon[task.status] ?? Clock}
					<div class="flex items-center gap-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-3">
						<Icon
							class="h-4 w-4 flex-shrink-0"
							style="color: {statusColor[task.status]}"
						/>
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm text-[var(--text-primary)]">{task.title}</p>
							<p class="text-xs text-[var(--text-tertiary)]">
								{task.type} &middot; {task.status} &middot; {relativeTime(task.createdAt)}
							</p>
						</div>
						{#if task.status === 'running' || task.status === 'pending'}
							<button
								class="text-xs text-[var(--text-tertiary)] hover:text-[var(--color-error)]"
								onclick={() => handleCancel(task.id)}
							>
								Cancel
							</button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
