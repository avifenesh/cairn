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
	import { Zap, Plus, Trash2, ToggleLeft, ToggleRight, Clock, AlertCircle, CheckCircle2, XCircle } from '@lucide/svelte';

	let loading = $state(true);
	let tab = $state<'templates' | 'rules' | 'history'>('templates');
	let showBuilder = $state(false);
	let builderPrefill = $state<RuleTemplate | undefined>(undefined);
	let justCreatedCount = $state(0);

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
		if (!confirm(`Delete rule "${name}"?`)) return;
		try {
			await deleteRule(id);
			ruleStore.removeRule(id);
		} catch (e) {
			console.error('Failed to delete rule:', e);
		}
	}

	function triggerSummary(rule: Rule): string {
		if (rule.trigger.type === 'cron') return rule.trigger.schedule ?? 'scheduled';
		const src = ruleStore.sources.find(s => s.name === rule.trigger.filter?.sourceType);
		const srcLabel = src?.label ?? rule.trigger.filter?.sourceType;
		const kind = rule.trigger.filter?.kind;
		if (srcLabel && kind) return `${srcLabel} / ${kind}`;
		if (srcLabel) return srcLabel;
		const eventLabels: Record<string, string> = {
			EventIngested: 'Signal', TaskCreated: 'Task created', TaskCompleted: 'Task completed',
			TaskFailed: 'Task failed', MemoryProposed: 'Memory',
		};
		return eventLabels[rule.trigger.eventType ?? ''] ?? 'event';
	}

	function actionSummary(rule: Rule): string {
		return rule.actions.map(a => {
			if (a.type === 'notify') return a.params.message?.slice(0, 60) ?? 'notify';
			if (a.type === 'task') return a.params.description?.slice(0, 60) ?? 'task';
			return a.type;
		}).join(' + ');
	}

	const statusIcon = (status: string) => {
		if (status === 'success') return CheckCircle2;
		if (status === 'error') return XCircle;
		return AlertCircle;
	};
	const statusColor = (status: string) => {
		if (status === 'success') return 'var(--color-success)';
		if (status === 'error') return 'var(--color-error)';
		return 'var(--text-tertiary)';
	};
</script>

<div class="mx-auto max-w-4xl px-4 py-4 sm:p-6">
	<!-- Header -->
	<div class="mb-6">
		<div class="flex items-center justify-between mb-4">
			<div>
				<h1 class="text-lg font-semibold tracking-tight" style="color: var(--text-primary)">Rules</h1>
				<p class="text-xs mt-0.5" style="color: var(--text-tertiary)">When X happens, do Y</p>
			</div>
			{#if !showBuilder}
				<Button size="sm" class="h-8 text-xs gap-1.5" onclick={() => openBuilder()}>
					<Plus class="h-3.5 w-3.5" /> New
				</Button>
			{/if}
		</div>

		<!-- Tab bar (underline style matching app convention) -->
		<div class="flex gap-4 border-b" style="border-color: var(--border-subtle)">
			{#each [
				{ key: 'templates', label: 'Templates' },
				{ key: 'rules', label: justCreatedCount > 0 ? `Active (${ruleStore.rules.length}) +${justCreatedCount}` : `Active (${ruleStore.rules.length})` },
				{ key: 'history', label: 'Log' },
			] as t}
				<button
					onclick={() => tab = t.key as typeof tab}
					class="relative pb-2 text-xs font-medium transition-colors"
					style="color: {tab === t.key ? 'var(--text-primary)' : 'var(--text-tertiary)'}"
				>
					{t.label}
					{#if tab === t.key}
						<div class="absolute bottom-0 left-0 right-0 h-[2px] rounded-full" style="background: var(--cairn-accent)"></div>
					{/if}
				</button>
			{/each}
		</div>
	</div>

	<!-- Builder (overlays content when open) -->
	{#if showBuilder}
		<div class="mb-6">
			<RuleBuilder
				sources={ruleStore.sources}
				prefill={builderPrefill}
				onclose={() => { showBuilder = false; builderPrefill = undefined; }}
				oncreated={() => {
					showBuilder = false; builderPrefill = undefined; tab = 'rules';
					justCreatedCount++;
					setTimeout(() => justCreatedCount = Math.max(0, justCreatedCount - 1), 3000);
				}}
			/>
		</div>
	{/if}

	<!-- Content -->
	{#if loading}
		<div class="space-y-2 mt-4">
			<Skeleton class="h-12 w-full rounded-lg" />
			<Skeleton class="h-12 w-full rounded-lg" />
			<Skeleton class="h-12 w-full rounded-lg" />
		</div>

	{:else if tab === 'templates'}
		<div class="mt-4">
			<TemplateGallery
				templates={ruleStore.templates}
				sources={ruleStore.sources}
				onselect={(t) => openBuilder(t)}
			/>
		</div>

	{:else if tab === 'rules'}
		<div class="mt-4">
			{#if ruleStore.rules.length === 0}
				<div class="rounded-xl border border-dashed p-10 text-center" style="border-color: var(--border-subtle)">
					<Zap class="mx-auto h-5 w-5 mb-2" style="color: var(--text-tertiary); opacity: 0.3" />
					<p class="text-xs" style="color: var(--text-tertiary)">No rules yet</p>
					<button onclick={() => tab = 'templates'} class="text-xs mt-1 font-medium" style="color: var(--cairn-accent)">
						Browse templates
					</button>
				</div>
			{:else}
				<div class="space-y-1">
					{#each ruleStore.rules as rule, i (rule.id)}
						<div
							class="group flex items-center gap-3 rounded-lg px-3 py-2.5 transition-all animate-in"
							style="
								background: var(--bg-1);
								border: 1px solid var(--border-subtle);
								animation-delay: {i * 30}ms;
							"
						>
							<!-- Toggle -->
							<button onclick={() => handleToggle(rule)} class="flex-shrink-0" aria-label={rule.enabled ? `Disable ${rule.name}` : `Enable ${rule.name}`}>
								{#if rule.enabled}
									<ToggleRight class="h-5 w-5" style="color: var(--cairn-accent)" />
								{:else}
									<ToggleLeft class="h-5 w-5" style="color: var(--text-tertiary); opacity: 0.5" />
								{/if}
							</button>

							<!-- Content -->
							<div class="flex-1 min-w-0" style="opacity: {rule.enabled ? 1 : 0.5}">
								<div class="flex items-center gap-2">
									<span class="text-sm font-medium truncate" style="color: var(--text-primary)">{rule.name}</span>
									<span class="text-[10px] font-mono rounded px-1.5 py-0.5" style="background: var(--bg-2); color: var(--text-tertiary)">
										{triggerSummary(rule)}
									</span>
									{#if rule.throttleMs > 0}
										<span class="text-[10px] font-mono" style="color: var(--text-tertiary)">
											{rule.throttleMs / 1000}s
										</span>
									{/if}
								</div>
								<p class="text-xs mt-0.5 truncate font-mono" style="color: var(--text-tertiary)">
									{actionSummary(rule)}
								</p>
							</div>

							<!-- Last fired -->
							{#if rule.lastFiredAt}
								<span class="text-[10px] font-mono flex-shrink-0 hidden sm:inline" style="color: var(--text-tertiary)">
									{relativeTime(rule.lastFiredAt)}
								</span>
							{/if}

							<!-- Delete -->
							<button
								onclick={() => handleDelete(rule.id, rule.name)}
								class="flex-shrink-0 p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity"
								aria-label={`Delete ${rule.name}`}
							>
								<Trash2 class="h-3.5 w-3.5" style="color: var(--text-tertiary)" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>

	{:else}
		<!-- Execution Log -->
		<div class="mt-4">
			{#if ruleStore.executions.length === 0}
				<div class="rounded-xl border border-dashed p-10 text-center" style="border-color: var(--border-subtle)">
					<Clock class="mx-auto h-5 w-5 mb-2" style="color: var(--text-tertiary); opacity: 0.3" />
					<p class="text-xs" style="color: var(--text-tertiary)">No executions yet</p>
				</div>
			{:else}
				<div class="space-y-px">
					{#each ruleStore.executions as exec, i (exec.id)}
						{@const Icon = statusIcon(exec.status)}
						{@const color = statusColor(exec.status)}
						<div
							class="flex items-center gap-3 rounded-md px-3 py-2 transition-colors animate-in"
							style="animation-delay: {i * 20}ms"
						>
							<Icon class="h-3 w-3 flex-shrink-0" style="color: {color}" />
							<span class="text-[11px] font-mono truncate" style="color: var(--text-secondary)">
								{ruleStore.rules.find(r => r.id === exec.ruleId)?.name ?? exec.ruleId.slice(0, 10)}
							</span>
							<span class="text-[10px] font-mono rounded px-1 py-0.5" style="background: color-mix(in srgb, {color} 10%, transparent); color: {color}">
								{exec.status}
							</span>
							{#if exec.error}
								<span class="text-[10px] truncate flex-1" style="color: var(--color-error)">{exec.error}</span>
							{:else}
								<span class="flex-1"></span>
							{/if}
							<span class="text-[10px] font-mono tabular-nums flex-shrink-0" style="color: var(--text-tertiary)">{exec.durationMs}ms</span>
							<time class="text-[10px] font-mono tabular-nums flex-shrink-0" style="color: var(--text-tertiary)">{relativeTime(exec.createdAt)}</time>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
