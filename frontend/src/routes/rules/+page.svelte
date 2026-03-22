<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { getRules, deleteRule, updateRule, getRecentRuleExecutions, getSources, getRuleTemplates } from '$lib/api/client';
	import { ruleStore } from '$lib/stores/rules.svelte';
	import { relativeTime } from '$lib/utils/time';
	import type { Rule, RuleTemplate } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import TemplateGallery from '$lib/components/rules/TemplateGallery.svelte';
	import RuleBuilder from '$lib/components/rules/RuleBuilder.svelte';
	import { Zap, Plus, Trash2, ToggleLeft, ToggleRight, Clock, AlertCircle, CheckCircle2, XCircle, Wrench, BookOpen, History } from '@lucide/svelte';

	let loading = $state(true);
	let tab = $state<'templates' | 'rules' | 'history'>('templates');
	let showBuilder = $state(false);
	let builderPrefill = $state<RuleTemplate | undefined>(undefined);

	function onRuleExecuted(e: Event) {
		const detail = (e as CustomEvent).detail;
		if (detail) ruleStore.addExecution(detail);
	}

	onMount(async () => {
		window.addEventListener('cairn:rule-executed', onRuleExecuted);
		try {
			const [rulesRes, execsRes, sourcesRes, templatesRes] = await Promise.all([
				getRules(),
				getRecentRuleExecutions().catch(() => ({ items: [] })),
				getSources().catch(() => ({ items: [] })),
				getRuleTemplates().catch(() => ({ items: [] })),
			]);
			ruleStore.setRules(rulesRes.items ?? []);
			ruleStore.setExecutions(execsRes.items ?? []);
			ruleStore.setSources(sourcesRes.items ?? []);
			ruleStore.setTemplates(templatesRes.items ?? []);
			// If user already has rules, default to rules tab.
			if ((rulesRes.items ?? []).length > 0) tab = 'rules';
		} catch (e) {
			console.error('Failed to load rules:', e);
		} finally {
			loading = false;
		}
	});
	onDestroy(() => window.removeEventListener('cairn:rule-executed', onRuleExecuted));

	function openBuilder(prefill?: RuleTemplate) {
		builderPrefill = prefill;
		showBuilder = true;
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
		// Use source registry for human-readable labels.
		const src = ruleStore.sources.find(s => s.name === rule.trigger.filter?.sourceType);
		const srcLabel = src?.label ?? rule.trigger.filter?.sourceType;
		const kind = rule.trigger.filter?.kind;
		if (srcLabel && kind) return `${srcLabel} ${kind}`;
		if (srcLabel) return `${srcLabel} events`;
		const eventLabels: Record<string, string> = {
			EventIngested: 'Signal event',
			TaskCreated: 'Task created',
			TaskCompleted: 'Task completed',
			TaskFailed: 'Task failed',
			MemoryProposed: 'Memory proposed',
		};
		return eventLabels[rule.trigger.eventType ?? ''] ?? rule.trigger.eventType ?? 'Unknown';
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
		<div class="flex gap-1.5">
			<Button variant={tab === 'templates' ? 'default' : 'outline'} size="sm" class="h-8 text-xs gap-1" onclick={() => tab = 'templates'}>
				<BookOpen class="h-3.5 w-3.5" /> Templates
			</Button>
			<Button variant={tab === 'rules' ? 'default' : 'outline'} size="sm" class="h-8 text-xs gap-1" onclick={() => tab = 'rules'}>
				<Wrench class="h-3.5 w-3.5" /> My Rules ({ruleStore.rules.length})
			</Button>
			<Button variant={tab === 'history' ? 'default' : 'outline'} size="sm" class="h-8 text-xs gap-1" onclick={() => tab = 'history'}>
				<History class="h-3.5 w-3.5" /> History
			</Button>
		</div>
	</div>

	<!-- Builder overlay -->
	{#if showBuilder}
		<div class="mb-6">
			<RuleBuilder
				sources={ruleStore.sources}
				prefill={builderPrefill}
				onclose={() => { showBuilder = false; builderPrefill = undefined; }}
				oncreated={() => { showBuilder = false; builderPrefill = undefined; tab = 'rules'; }}
			/>
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			<Skeleton class="h-16 w-full" />
			<Skeleton class="h-16 w-full" />
			<Skeleton class="h-16 w-full" />
		</div>
	{:else if tab === 'templates'}
		<!-- Templates Gallery -->
		<div class="space-y-4">
			<TemplateGallery
				templates={ruleStore.templates}
				sources={ruleStore.sources}
				onselect={(t) => openBuilder(t)}
			/>
			{#if !showBuilder}
				<div class="flex justify-center">
					<Button variant="outline" size="sm" class="h-8 text-xs gap-1.5" onclick={() => openBuilder()}>
						<Plus class="h-3.5 w-3.5" /> Build custom rule
					</Button>
				</div>
			{/if}
		</div>
	{:else if tab === 'rules'}
		<!-- Rules List -->
		{#if !showBuilder}
			<div class="mb-4 flex justify-end">
				<Button size="sm" class="h-8 text-xs gap-1.5" onclick={() => openBuilder()}>
					<Plus class="h-3.5 w-3.5" /> New Rule
				</Button>
			</div>
		{/if}
		{#if ruleStore.rules.length === 0}
			<div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-1)] p-8 text-center">
				<Zap class="mx-auto h-8 w-8 text-[var(--text-tertiary)]/40 mb-2" />
				<p class="text-sm text-[var(--text-tertiary)]">No automation rules yet</p>
				<p class="text-xs text-[var(--text-tertiary)]/60 mt-1">Use a template or build a custom rule</p>
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
