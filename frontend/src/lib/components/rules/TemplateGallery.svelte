<script lang="ts">
	import type { RuleTemplate, SourceInfo, Rule } from '$lib/types';
	import { instantiateRuleTemplate } from '$lib/api/client';
	import { ruleStore } from '$lib/stores/rules.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Zap, Loader2, ListTodo, Brain, Clock, Antenna, ChevronRight } from '@lucide/svelte';

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
		{ key: 'signal', label: 'Signals', icon: Antenna, desc: 'React to incoming events' },
		{ key: 'task', label: 'Tasks', icon: ListTodo, desc: 'Monitor task lifecycle' },
		{ key: 'memory', label: 'Memory', icon: Brain, desc: 'Memory extraction flow' },
		{ key: 'scheduled', label: 'Scheduled', icon: Clock, desc: 'Time-based triggers' },
	];
	let activeCategory = $state('signal');

	const sourceColorMap: Record<string, string> = {
		github: 'var(--src-github)',
		hn: 'var(--src-hackernews)',
		reddit: 'var(--src-reddit)',
		npm: 'var(--src-npm)',
		crates: 'var(--src-crates)',
		gmail: 'var(--src-gmail)',
		stackoverflow: 'var(--src-stackoverflow)',
	};

	const categoryColor: Record<string, string> = {
		signal: 'var(--cairn-accent)',
		task: 'var(--color-warning)',
		memory: 'var(--src-github)',
		scheduled: 'var(--src-x)',
	};

	function getAccentColor(tmpl: RuleTemplate): string {
		if (tmpl.source && sourceColorMap[tmpl.source]) return sourceColorMap[tmpl.source];
		return categoryColor[tmpl.category] ?? 'var(--cairn-accent)';
	}

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

<div class="space-y-5">
	<!-- Category selector - horizontal cards -->
	<div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
		{#each categories as cat, i}
			{@const Icon = cat.icon}
			{@const isActive = activeCategory === cat.key}
			{@const color = categoryColor[cat.key]}
			<button
				onclick={() => activeCategory = cat.key}
				class="group relative rounded-xl border p-3 text-left transition-all"
				style="
					border-color: {isActive ? color : 'var(--border-subtle)'};
					background: {isActive ? `color-mix(in srgb, ${color} 8%, var(--bg-1))` : 'var(--bg-1)'};
					animation-delay: {i * 50}ms;
				"
				class:animate-in={true}
			>
				<div class="flex items-center gap-2 mb-1">
					<Icon
						class="h-4 w-4 transition-colors"
						style="color: {isActive ? color : 'var(--text-tertiary)'}"
					/>
					<span class="text-xs font-medium" style="color: {isActive ? 'var(--text-primary)' : 'var(--text-secondary)'}">
						{cat.label}
					</span>
				</div>
				<p class="text-[10px] leading-tight" style="color: var(--text-tertiary)">{cat.desc}</p>
				{#if isActive}
					<div
						class="absolute bottom-0 left-3 right-3 h-[2px] rounded-full"
						style="background: {color}"
					></div>
				{/if}
			</button>
		{/each}
	</div>

	<!-- Source filter (signal category only) -->
	{#if activeCategory === 'signal' && sources.length > 0}
		<div class="flex items-center gap-2 px-1">
			<span class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Source</span>
			<div class="h-px flex-1" style="background: var(--border-subtle)"></div>
			<select
				bind:value={sourceFilter}
				class="rounded-md border px-2 py-1 text-xs"
				style="border-color: var(--border-subtle); background: var(--bg-0); color: var(--text-secondary)"
			>
				<option value="">All</option>
				{#each sources as src}
					<option value={src.name}>{src.label}</option>
				{/each}
			</select>
		</div>
	{/if}

	<!-- Error display -->
	{#if instantiateError}
		<div class="rounded-lg border px-3 py-2 text-xs" style="border-color: var(--color-error); background: color-mix(in srgb, var(--color-error) 8%, var(--bg-1)); color: var(--color-error)">
			{instantiateError}
		</div>
	{/if}

	<!-- Template cards -->
	{#if filtered().length === 0}
		<div class="rounded-xl border p-8 text-center" style="border-color: var(--border-subtle); background: var(--bg-1)">
			<Zap class="mx-auto h-5 w-5 mb-2" style="color: var(--text-tertiary); opacity: 0.4" />
			<p class="text-xs" style="color: var(--text-tertiary)">No templates match this filter</p>
		</div>
	{:else}
		<div class="space-y-1.5">
			{#each filtered() as tmpl, i (tmpl.id)}
				{@const color = getAccentColor(tmpl)}
				<button
					onclick={() => handleUse(tmpl)}
					disabled={instantiating === tmpl.id}
					class="group w-full text-left rounded-lg border transition-all animate-in"
					style="
						border-color: var(--border-subtle);
						background: var(--bg-1);
						border-left: 3px solid {color};
						animation-delay: {i * 40}ms;
					"
				>
					<div class="flex items-center gap-3 px-3 py-2.5">
						<!-- Content -->
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium truncate" style="color: var(--text-primary)">
									{tmpl.name}
								</span>
								{#if tmpl.source}
									<span
										class="rounded-full px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wider"
										style="background: color-mix(in srgb, {color} 15%, transparent); color: {color}"
									>
										{sourceLabel(tmpl.source)}
									</span>
								{/if}
							</div>
							<p class="text-xs mt-0.5 truncate" style="color: var(--text-tertiary)">
								{tmpl.description}
							</p>
						</div>

						<!-- Params indicator -->
						{#if tmpl.params.length > 0}
							<div class="flex gap-1 flex-shrink-0">
								{#each tmpl.params as p}
									<span
										class="rounded px-1.5 py-0.5 text-[9px] font-mono"
										style="background: var(--bg-2); color: var(--text-tertiary)"
									>
										{p.key}{#if p.required}<span style="color: {color}">*</span>{/if}
									</span>
								{/each}
							</div>
						{/if}

						<!-- Action -->
						<div class="flex-shrink-0 flex items-center gap-1 text-xs font-medium transition-colors" style="color: {color}">
							{#if instantiating === tmpl.id}
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
							{:else}
								<span class="hidden sm:inline opacity-0 group-hover:opacity-100 transition-opacity">
									{hasRequiredParams(tmpl) ? 'Configure' : 'Enable'}
								</span>
								<ChevronRight class="h-3.5 w-3.5 opacity-40 group-hover:opacity-100 group-hover:translate-x-0.5 transition-all" />
							{/if}
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</div>
