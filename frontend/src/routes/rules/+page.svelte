<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { getRules, createRule, deleteRule, updateRule, getRecentRuleExecutions } from '$lib/api/client';
	import { ruleStore } from '$lib/stores/rules.svelte';
	import { relativeTime } from '$lib/utils/time';
	import type { Rule, RuleExecution } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Zap, Plus, Trash2, ToggleLeft, ToggleRight, Clock, AlertCircle, CheckCircle2, XCircle } from '@lucide/svelte';

	let loading = $state(true);
	let showCreate = $state(false);
	let tab = $state<'rules' | 'history'>('rules');

	// Create form state
	let newName = $state('');
	let newDesc = $state('');
	let newEventType = $state('EventIngested');
	let newFilter = $state('');
	let newCondition = $state('');
	let newActionType = $state<'notify' | 'task'>('notify');
	let newActionMessage = $state('');
	let newThrottle = $state(0);
	let creating = $state(false);

	function onRuleExecuted(e: Event) {
		const detail = (e as CustomEvent).detail;
		if (detail) ruleStore.addExecution(detail);
	}

	onMount(async () => {
		window.addEventListener('cairn:rule-executed', onRuleExecuted);
		try {
			const [rulesRes, execsRes] = await Promise.all([
				getRules(),
				getRecentRuleExecutions().catch(() => ({ items: [] })),
			]);
			ruleStore.setRules(rulesRes.items ?? []);
			ruleStore.setExecutions(execsRes.items ?? []);
		} catch (e) {
			console.error('Failed to load rules:', e);
		} finally {
			loading = false;
		}
	});
	onDestroy(() => window.removeEventListener('cairn:rule-executed', onRuleExecuted));

	async function handleCreate() {
		if (!newName.trim() || !newActionMessage.trim()) return;
		creating = true;
		try {
			const filter: Record<string, string> = {};
			if (newFilter.trim()) {
				try { Object.assign(filter, JSON.parse(newFilter)); } catch { /* ignore */ }
			}
			const rule: Partial<Rule> = {
				name: newName.trim(),
				description: newDesc.trim(),
				enabled: true,
				trigger: { type: 'event', eventType: newEventType, filter },
				condition: newCondition.trim(),
				actions: [{ type: newActionType, params: newActionType === 'notify'
					? { message: newActionMessage, priority: '1' }
					: { description: newActionMessage, type: 'general' }
				}],
				throttleMs: newThrottle * 1000,
			};
			const res = await createRule(rule);
			ruleStore.addRule(res.rule);
			showCreate = false;
			newName = ''; newDesc = ''; newFilter = ''; newCondition = ''; newActionMessage = ''; newThrottle = 0;
		} catch (e) {
			console.error('Failed to create rule:', e);
		} finally {
			creating = false;
		}
	}

	async function handleToggle(rule: Rule) {
		try {
			await updateRule(rule.id, { enabled: !rule.enabled });
			ruleStore.updateRule(rule.id, { enabled: !rule.enabled });
		} catch (e) {
			console.error('Failed to toggle rule:', e);
		}
	}

	async function handleDelete(id: string, name: string) {
		if (!confirm(`Delete rule "${name}"? This cannot be undone.`)) return;
		try {
			await deleteRule(id);
			ruleStore.removeRule(id);
		} catch (e) {
			console.error('Failed to delete rule:', e);
		}
	}

	function triggerSummary(rule: Rule): string {
		if (rule.trigger.type === 'cron') return `Schedule: ${rule.trigger.schedule}`;
		let s = `On ${rule.trigger.eventType}`;
		if (rule.trigger.filter && Object.keys(rule.trigger.filter).length > 0) {
			s += ` (${Object.entries(rule.trigger.filter).map(([k, v]) => `${k}=${v}`).join(', ')})`;
		}
		return s;
	}

	function actionSummary(rule: Rule): string {
		return rule.actions.map(a => {
			if (a.type === 'notify') return `Notify: ${a.params.message?.slice(0, 50) ?? ''}`;
			if (a.type === 'task') return `Task: ${a.params.description?.slice(0, 50) ?? ''}`;
			return a.type;
		}).join(', ');
	}

	const statusIcon = (status: string) => {
		if (status === 'success') return CheckCircle2;
		if (status === 'error') return XCircle;
		return AlertCircle;
	};
	const statusColor = (status: string) => {
		if (status === 'success') return 'text-[var(--color-success)]';
		if (status === 'error') return 'text-[var(--color-error)]';
		return 'text-[var(--text-tertiary)]';
	};
</script>

<div class="mx-auto max-w-4xl px-4 py-4 sm:p-6">
	<!-- Header -->
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Rules</h1>
			<p class="mt-1 text-xs text-[var(--text-tertiary)]">Automation rules — when X happens, do Y</p>
		</div>
		<div class="flex gap-2">
			<Button variant={tab === 'rules' ? 'default' : 'outline'} size="sm" class="h-8 text-xs" onclick={() => tab = 'rules'}>
				Rules ({ruleStore.rules.length})
			</Button>
			<Button variant={tab === 'history' ? 'default' : 'outline'} size="sm" class="h-8 text-xs" onclick={() => tab = 'history'}>
				History
			</Button>
			<Button size="sm" class="h-8 text-xs gap-1.5" onclick={() => showCreate = !showCreate}>
				<Plus class="h-3.5 w-3.5" /> New Rule
			</Button>
		</div>
	</div>

	<!-- Create Form -->
	{#if showCreate}
		<div class="mb-6 rounded-lg border border-[var(--cairn-accent)]/30 bg-[var(--bg-1)] p-4 space-y-3 animate-in">
			<p class="text-sm font-medium text-[var(--text-primary)]">Create Rule</p>
			<div class="grid grid-cols-2 gap-3">
				<input bind:value={newName} placeholder="Rule name" class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none" />
				<input bind:value={newDesc} placeholder="Description (optional)" class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none" />
			</div>
			<div class="grid grid-cols-3 gap-3">
				<select bind:value={newEventType} class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)]">
					<option value="EventIngested">Signal Event</option>
					<option value="TaskCreated">Task Created</option>
					<option value="TaskCompleted">Task Completed</option>
					<option value="TaskFailed">Task Failed</option>
					<option value="MemoryProposed">Memory Proposed</option>
				</select>
				<input bind:value={newFilter} placeholder={'Filter JSON: {"sourceType":"github"}'} class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)] font-mono text-xs focus:border-[var(--cairn-accent)] focus:outline-none" />
				<input bind:value={newCondition} placeholder='Condition: contains(title, "PR")' class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)] font-mono text-xs focus:border-[var(--cairn-accent)] focus:outline-none" />
			</div>
			<div class="grid grid-cols-3 gap-3">
				<select bind:value={newActionType} class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)]">
					<option value="notify">Notify</option>
					<option value="task">Submit Task</option>
				</select>
				<input bind:value={newActionMessage} placeholder={newActionType === 'notify' ? 'Message: New PR: {{.title}}' : 'Task description'} class="col-span-2 rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none" />
			</div>
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<label for="throttle-input" class="text-xs text-[var(--text-tertiary)]">Throttle (seconds):</label>
					<input id="throttle-input" type="number" bind:value={newThrottle} min="0" class="w-20 rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1 text-sm text-[var(--text-primary)]" />
				</div>
				<div class="flex gap-2">
					<Button variant="outline" size="sm" class="h-7 text-xs" onclick={() => showCreate = false}>Cancel</Button>
					<Button size="sm" class="h-7 text-xs gap-1" onclick={handleCreate} disabled={creating || !newName.trim() || !newActionMessage.trim()}>
						<Zap class="h-3 w-3" /> Create
					</Button>
				</div>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			<Skeleton class="h-16 w-full" />
			<Skeleton class="h-16 w-full" />
			<Skeleton class="h-16 w-full" />
		</div>
	{:else if tab === 'rules'}
		<!-- Rules List -->
		{#if ruleStore.rules.length === 0}
			<div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-1)] p-8 text-center">
				<Zap class="mx-auto h-8 w-8 text-[var(--text-tertiary)]/40 mb-2" />
				<p class="text-sm text-[var(--text-tertiary)]">No automation rules yet</p>
				<p class="text-xs text-[var(--text-tertiary)]/60 mt-1">Create a rule to react to events automatically</p>
			</div>
		{:else}
			<div class="space-y-2">
				{#each ruleStore.rules as rule (rule.id)}
					<div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-1)] p-4 flex items-start gap-3 hover:border-[var(--cairn-accent)]/30 transition-colors">
						<button onclick={() => handleToggle(rule)} class="mt-0.5 flex-shrink-0" aria-label={rule.enabled ? `Disable rule ${rule.name}` : `Enable rule ${rule.name}`}>
							{#if rule.enabled}
								<ToggleRight class="h-5 w-5 text-[var(--cairn-accent)]" />
							{:else}
								<ToggleLeft class="h-5 w-5 text-[var(--text-tertiary)]" />
							{/if}
						</button>
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium text-[var(--text-primary)] {rule.enabled ? '' : 'opacity-50'}">{rule.name}</span>
								{#if rule.condition}
									<Badge variant="outline" class="text-[10px]">expr</Badge>
								{/if}
								{#if rule.throttleMs > 0}
									<Badge variant="outline" class="text-[10px] gap-0.5"><Clock class="h-2.5 w-2.5" />{rule.throttleMs / 1000}s</Badge>
								{/if}
							</div>
							<p class="text-xs text-[var(--text-tertiary)] mt-0.5">{triggerSummary(rule)}</p>
							<p class="text-xs text-[var(--text-secondary)] mt-0.5">{actionSummary(rule)}</p>
							{#if rule.lastFiredAt}
								<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-1">Last fired {relativeTime(rule.lastFiredAt)}</p>
							{/if}
						</div>
						<button onclick={() => handleDelete(rule.id, rule.name)} class="flex-shrink-0 p-1 rounded hover:bg-[var(--color-error)]/10 transition-colors" aria-label={`Delete rule ${rule.name}`}>
							<Trash2 class="h-3.5 w-3.5 text-[var(--text-tertiary)] hover:text-[var(--color-error)]" />
						</button>
					</div>
				{/each}
			</div>
		{/if}
	{:else}
		<!-- Execution History -->
		{#if ruleStore.executions.length === 0}
			<div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-1)] p-8 text-center">
				<Clock class="mx-auto h-8 w-8 text-[var(--text-tertiary)]/40 mb-2" />
				<p class="text-sm text-[var(--text-tertiary)]">No rule executions yet</p>
			</div>
		{:else}
			<div class="space-y-1">
				{#each ruleStore.executions as exec (exec.id)}
					{@const Icon = statusIcon(exec.status)}
					<div class="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-[var(--bg-1)] transition-colors">
						<Icon class="h-3.5 w-3.5 flex-shrink-0 {statusColor(exec.status)}" />
						<code class="text-[11px] font-mono text-[var(--text-tertiary)]">{ruleStore.rules.find(r => r.id === exec.ruleId)?.name ?? exec.ruleId.slice(0, 12)}</code>
						<Badge variant="outline" class="text-[10px] {statusColor(exec.status)}">{exec.status}</Badge>
						{#if exec.error}
							<span class="text-xs text-[var(--color-error)] truncate flex-1">{exec.error}</span>
						{:else}
							<span class="flex-1"></span>
						{/if}
						<span class="text-[10px] text-[var(--text-tertiary)] tabular-nums">{exec.durationMs}ms</span>
						<time class="text-[10px] text-[var(--text-tertiary)] tabular-nums font-mono">{relativeTime(exec.createdAt)}</time>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
