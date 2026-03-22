<script lang="ts">
	import type { RuleTemplate, SourceInfo, Rule } from '$lib/types';
	import { instantiateRuleTemplate } from '$lib/api/client';
	import { ruleStore } from '$lib/stores/rules.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Zap, Loader2, ListTodo, Brain, Clock, Antenna } from '@lucide/svelte';

	interface Props {
		templates: RuleTemplate[];
		sources: SourceInfo[];
		onselect?: (template: RuleTemplate) => void;
	}
	let { templates, sources, onselect }: Props = $props();

	let sourceFilter = $state('');
	let instantiating = $state<string | null>(null);
	let instantiateError = $state('');

	const categories = [
		{ key: 'signal', label: 'Signal', icon: Antenna },
		{ key: 'task', label: 'Tasks', icon: ListTodo },
		{ key: 'memory', label: 'Memory', icon: Brain },
		{ key: 'scheduled', label: 'Scheduled', icon: Clock },
	];
	let activeCategory = $state('signal');

	const filtered = $derived(() => {
		let items = templates;
		if (activeCategory) {
			items = items.filter(t => t.category === activeCategory);
		}
		if (sourceFilter) {
			items = items.filter(t => !t.source || t.source === sourceFilter);
		}
		return items;
	});

	function sourceLabel(name: string): string {
		return sources.find(s => s.name === name)?.label ?? name;
	}

	function hasRequiredParams(t: RuleTemplate): boolean {
		return t.params.some(p => p.required);
	}

	async function handleUse(t: RuleTemplate) {
		if (hasRequiredParams(t)) {
			onselect?.(t);
			return;
		}
		// Zero required params — instantiate directly.
		instantiating = t.id;
		instantiateError = '';
		try {
			const res = await instantiateRuleTemplate(t.id, {});
			ruleStore.addRule(res.rule);
		} catch (e) {
			instantiateError = e instanceof Error ? e.message : 'Failed to create rule';
		} finally {
			instantiating = null;
		}
	}
</script>

<div class="space-y-4">
	<!-- Category tabs -->
	<div class="flex gap-1.5 flex-wrap">
		{#each categories as cat}
			{@const Icon = cat.icon}
			<button
				onclick={() => activeCategory = cat.key}
				class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors {activeCategory === cat.key
					? 'bg-[var(--cairn-accent)] text-white'
					: 'bg-[var(--bg-1)] text-[var(--text-secondary)] hover:bg-[var(--bg-2)]'}"
			>
				<Icon class="h-3.5 w-3.5" />
				{cat.label}
			</button>
		{/each}
	</div>

	<!-- Source filter (only for signal category) -->
	{#if activeCategory === 'signal' && sources.length > 0}
		<div class="flex items-center gap-2">
			<span class="text-xs text-[var(--text-tertiary)]">Source:</span>
			<select
				bind:value={sourceFilter}
				class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1 text-xs text-[var(--text-primary)]"
			>
				<option value="">All sources</option>
				{#each sources as src}
					<option value={src.name}>{src.label}</option>
				{/each}
			</select>
		</div>
	{/if}

	<!-- Template cards -->
	{#if instantiateError}
		<p class="text-xs text-[var(--color-error)] bg-[var(--color-error)]/10 rounded-md px-3 py-2">{instantiateError}</p>
	{/if}

	{#if filtered().length === 0}
		<div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-1)] p-6 text-center">
			<Zap class="mx-auto h-6 w-6 text-[var(--text-tertiary)]/40 mb-2" />
			<p class="text-sm text-[var(--text-tertiary)]">No templates match this filter</p>
		</div>
	{:else}
		<div class="grid gap-2 sm:grid-cols-2">
			{#each filtered() as tmpl (tmpl.id)}
				<div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-1)] p-3 flex flex-col gap-2 hover:border-[var(--cairn-accent)]/30 transition-colors">
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<p class="text-sm font-medium text-[var(--text-primary)] truncate">{tmpl.name}</p>
							<p class="text-xs text-[var(--text-tertiary)] mt-0.5 line-clamp-2">{tmpl.description}</p>
						</div>
						{#if tmpl.source}
							<Badge variant="outline" class="text-[10px] flex-shrink-0">{sourceLabel(tmpl.source)}</Badge>
						{/if}
					</div>
					{#if tmpl.params.length > 0}
						<div class="flex gap-1 flex-wrap">
							{#each tmpl.params as p}
								<Badge variant="outline" class="text-[10px] {p.required ? 'border-[var(--cairn-accent)]/40' : ''}">
									{p.label}{#if p.required}*{/if}
								</Badge>
							{/each}
						</div>
					{/if}
					<div class="flex justify-end">
						<Button
							size="sm"
							class="h-7 text-xs gap-1"
							onclick={() => handleUse(tmpl)}
							disabled={instantiating === tmpl.id}
						>
							{#if instantiating === tmpl.id}
								<Loader2 class="h-3 w-3 animate-spin" />
							{:else}
								<Zap class="h-3 w-3" />
							{/if}
							{hasRequiredParams(tmpl) ? 'Customize' : 'Enable'}
						</Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
