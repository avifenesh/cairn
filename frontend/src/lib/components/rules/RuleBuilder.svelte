<script lang="ts">
	import type { SourceInfo, RuleTemplate, Rule, RuleTrigger, RuleAction } from '$lib/types';
	import { createRule } from '$lib/api/client';
	import { ruleStore } from '$lib/stores/rules.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Loader2, ArrowRight, ArrowLeft, Zap, Antenna, ListTodo, Brain, Clock, Plus, X } from '@lucide/svelte';

	interface Props {
		sources: SourceInfo[];
		prefill?: RuleTemplate;
		onclose?: () => void;
		oncreated?: (rule: Rule) => void;
	}
	let { sources, prefill, onclose, oncreated }: Props = $props();

	// --- Step state ---
	let step = $state(1);
	let saving = $state(false);
	let error = $state('');

	// Step 1: Trigger
	type TriggerCategory = 'signal' | 'task-created' | 'task-completed' | 'task-failed' | 'memory' | 'cron';
	let triggerCategory = $state<TriggerCategory>('signal');
	let selectedSource = $state('');
	let selectedKinds = $state<string[]>([]);
	let cronSchedule = $state('0 9 * * *');

	// Step 2: Conditions
	interface Condition {
		field: string;
		operator: 'equals' | 'contains' | 'startsWith';
		value: string;
	}
	let conditions = $state<Condition[]>([]);

	// Step 3: Actions
	interface ActionDraft {
		type: 'notify' | 'task';
		message: string;
		priority: string;
	}
	let actions = $state<ActionDraft[]>([{ type: 'notify', message: '', priority: '1' }]);

	// Step 4: Settings
	let ruleName = $state('');
	let ruleDesc = $state('');
	let throttleSec = $state(60);

	// Derive available fields based on trigger type.
	const availableFields = $derived(() => {
		const eventMap: Record<string, string[]> = {
			'signal': ['sourceType', 'kind', 'title', 'url', 'actor', 'repo'],
			'task-created': ['taskId', 'type', 'description'],
			'task-completed': ['taskId', 'result'],
			'task-failed': ['taskId', 'error'],
			'memory': ['memoryId', 'content'],
		};
		return eventMap[triggerCategory] ?? [];
	});

	// Derive available variable chips for action message.
	const variableChips = $derived(() => availableFields());

	// Derive kinds for selected source.
	const availableKinds = $derived(() => {
		if (triggerCategory !== 'signal' || !selectedSource) return [];
		return sources.find(s => s.name === selectedSource)?.kinds ?? [];
	});

	// Auto-suggest name.
	const suggestedName = $derived(() => {
		const actionLabel = actions[0]?.type === 'notify' ? 'Notify' : 'Task';
		if (triggerCategory === 'signal') {
			const src = sources.find(s => s.name === selectedSource)?.label ?? selectedSource ?? 'any source';
			if (selectedKinds.length === 1) return `${actionLabel} on ${src} ${selectedKinds[0]}`;
			return `${actionLabel} on ${src} events`;
		}
		const labels: Record<string, string> = {
			'task-created': 'task creation',
			'task-completed': 'task completion',
			'task-failed': 'task failure',
			'memory': 'memory proposals',
			'cron': 'schedule',
		};
		return `${actionLabel} on ${labels[triggerCategory] ?? triggerCategory}`;
	});

	// Apply prefill from template.
	$effect(() => {
		if (!prefill) return;
		// We don't have the factory output on the client — just set what we can from template metadata.
		if (prefill.source) selectedSource = prefill.source;
		if (prefill.category === 'signal') triggerCategory = 'signal';
		else if (prefill.category === 'task') triggerCategory = 'task-failed';
		else if (prefill.category === 'memory') triggerCategory = 'memory';
		else if (prefill.category === 'scheduled') triggerCategory = 'cron';
		ruleName = prefill.name;
	});

	function addCondition() {
		const fields = availableFields();
		conditions = [...conditions, { field: fields[0] ?? 'title', operator: 'contains', value: '' }];
	}

	function removeCondition(i: number) {
		conditions = conditions.filter((_, idx) => idx !== i);
	}

	function addAction() {
		if (actions.length >= 10) return;
		actions = [...actions, { type: 'notify', message: '', priority: '1' }];
	}

	function removeAction(i: number) {
		if (actions.length <= 1) return;
		actions = actions.filter((_, idx) => idx !== i);
	}

	function insertVariable(actionIdx: number, varName: string) {
		actions[actionIdx].message += `{{.${varName}}}`;
	}

	function buildTrigger(): RuleTrigger {
		if (triggerCategory === 'cron') {
			return { type: 'cron', schedule: cronSchedule };
		}
		const eventTypeMap: Record<string, string> = {
			'signal': 'EventIngested',
			'task-created': 'TaskCreated',
			'task-completed': 'TaskCompleted',
			'task-failed': 'TaskFailed',
			'memory': 'MemoryProposed',
		};
		const trigger: RuleTrigger = {
			type: 'event',
			eventType: eventTypeMap[triggerCategory] ?? 'EventIngested',
		};
		// Build filter from source + kinds + "equals" conditions.
		const filter: Record<string, string> = {};
		if (triggerCategory === 'signal' && selectedSource) {
			filter['sourceType'] = selectedSource;
		}
		if (selectedKinds.length === 1) {
			filter['kind'] = selectedKinds[0];
		}
		for (const c of conditions) {
			if (c.operator === 'equals' && c.value.trim()) {
				filter[c.field] = c.value.trim();
			}
		}
		if (Object.keys(filter).length > 0) trigger.filter = filter;
		return trigger;
	}

	function buildCondition(): string {
		const parts: string[] = [];
		// Kinds with multiple selections become OR.
		if (selectedKinds.length > 1) {
			const kindExprs = selectedKinds.map(k => `kind == "${k}"`);
			parts.push(`(${kindExprs.join(' || ')})`);
		}
		// Non-equals conditions become expr-lang.
		for (const c of conditions) {
			if (!c.value.trim()) continue;
			if (c.operator === 'contains') {
				parts.push(`${c.field} contains "${c.value.trim()}"`);
			} else if (c.operator === 'startsWith') {
				parts.push(`${c.field} startsWith "${c.value.trim()}"`);
			}
			// "equals" handled in filter, not condition.
		}
		return parts.join(' && ');
	}

	function buildActions(): RuleAction[] {
		return actions.map(a => {
			const params: Record<string, string> = { priority: a.priority };
			if (a.type === 'notify') {
				params.message = a.message;
			} else {
				params.description = a.message;
				params.type = 'general';
			}
			return { type: a.type, params };
		});
	}

	async function handleSave() {
		saving = true;
		error = '';
		try {
			const rule: Partial<Rule> = {
				name: ruleName || suggestedName(),
				description: ruleDesc,
				enabled: true,
				trigger: buildTrigger(),
				condition: buildCondition(),
				actions: buildActions(),
				throttleMs: throttleSec * 1000,
			};
			const res = await createRule(rule);
			ruleStore.addRule(res.rule);
			oncreated?.(res.rule);
			onclose?.();
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to create rule';
		} finally {
			saving = false;
		}
	}

	const canProceed = $derived(() => {
		if (step === 1) {
			if (triggerCategory === 'cron') return cronSchedule.trim().length > 0;
			return true;
		}
		if (step === 3) return actions.some(a => a.message.trim().length > 0);
		return true;
	});
</script>

<div class="rounded-lg border border-[var(--cairn-accent)]/30 bg-[var(--bg-1)] p-4 space-y-4 animate-in">
	<!-- Step indicator -->
	<div class="flex items-center gap-2 text-xs text-[var(--text-tertiary)]">
		{#each ['When', 'Where', 'Then', 'Settings'] as label, i}
			<span class="flex items-center gap-1 {step === i + 1 ? 'text-[var(--cairn-accent)] font-medium' : step > i + 1 ? 'text-[var(--text-secondary)]' : ''}">
				<span class="flex h-5 w-5 items-center justify-center rounded-full text-[10px] {step === i + 1 ? 'bg-[var(--cairn-accent)] text-white' : step > i + 1 ? 'bg-[var(--text-tertiary)]/20' : 'bg-[var(--bg-2)]'}">{i + 1}</span>
				{label}
			</span>
			{#if i < 3}<span class="text-[var(--border-subtle)]">/</span>{/if}
		{/each}
		<div class="flex-1"></div>
		<button onclick={() => onclose?.()} class="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
			<X class="h-4 w-4" />
		</button>
	</div>

	<!-- Step 1: When -->
	{#if step === 1}
		<div class="space-y-3">
			<p class="text-sm font-medium text-[var(--text-primary)]">When should this rule fire?</p>
			<div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
				{#each [
					{ key: 'signal', label: 'A signal arrives', icon: Antenna },
					{ key: 'task-failed', label: 'A task fails', icon: ListTodo },
					{ key: 'task-created', label: 'A task is created', icon: ListTodo },
					{ key: 'task-completed', label: 'A task completes', icon: ListTodo },
					{ key: 'memory', label: 'Memory proposed', icon: Brain },
					{ key: 'cron', label: 'On a schedule', icon: Clock },
				] as opt}
					{@const Icon = opt.icon}
					<button
						onclick={() => triggerCategory = opt.key as TriggerCategory}
						class="flex items-center gap-2 rounded-lg border p-2.5 text-xs transition-colors {triggerCategory === opt.key
							? 'border-[var(--cairn-accent)] bg-[var(--cairn-accent)]/10 text-[var(--cairn-accent)]'
							: 'border-[var(--border-subtle)] bg-[var(--bg-0)] text-[var(--text-secondary)] hover:border-[var(--cairn-accent)]/30'}"
					>
						<Icon class="h-4 w-4 flex-shrink-0" />
						{opt.label}
					</button>
				{/each}
			</div>

			{#if triggerCategory === 'signal'}
				<div class="space-y-2">
					<label for="source-select" class="text-xs text-[var(--text-tertiary)]">From source:</label>
					<select
						id="source-select"
						bind:value={selectedSource}
						class="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-3 py-1.5 text-sm text-[var(--text-primary)]"
					>
						<option value="">Any source</option>
						{#each sources as src}
							<option value={src.name}>{src.label}</option>
						{/each}
					</select>

					{#if availableKinds().length > 0}
						<span class="text-xs text-[var(--text-tertiary)]">Event types:</span>
						<div class="flex flex-wrap gap-1.5">
							{#each availableKinds() as kind}
								<button
									onclick={() => {
										if (selectedKinds.includes(kind)) {
											selectedKinds = selectedKinds.filter(k => k !== kind);
										} else {
											selectedKinds = [...selectedKinds, kind];
										}
									}}
									class="rounded-md border px-2.5 py-1 text-xs transition-colors {selectedKinds.includes(kind)
										? 'border-[var(--cairn-accent)] bg-[var(--cairn-accent)]/10 text-[var(--cairn-accent)]'
										: 'border-[var(--border-subtle)] bg-[var(--bg-0)] text-[var(--text-secondary)] hover:border-[var(--cairn-accent)]/30'}"
								>
									{kind}
								</button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			{#if triggerCategory === 'cron'}
				<div class="space-y-1">
					<label for="cron-input" class="text-xs text-[var(--text-tertiary)]">Cron expression:</label>
					<Input id="cron-input" bind:value={cronSchedule} placeholder="0 9 * * *" class="font-mono text-sm" />
					<p class="text-[10px] text-[var(--text-tertiary)]">min hour day month weekday (e.g. "0 9 * * 1" = Monday 9 AM)</p>
				</div>
			{/if}
		</div>
	{/if}

	<!-- Step 2: Where (conditions) -->
	{#if step === 2}
		<div class="space-y-3">
			<div class="flex items-center justify-between">
				<p class="text-sm font-medium text-[var(--text-primary)]">Add conditions (optional)</p>
				<Button variant="outline" size="sm" class="h-7 text-xs gap-1" onclick={addCondition}>
					<Plus class="h-3 w-3" /> Add
				</Button>
			</div>

			{#if conditions.length === 0}
				<p class="text-xs text-[var(--text-tertiary)] italic">No conditions — rule will match all events of this type.</p>
			{:else}
				<div class="space-y-2">
					{#each conditions as cond, i}
						<div class="flex items-center gap-2">
							<select
								bind:value={cond.field}
								class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1.5 text-xs text-[var(--text-primary)]"
							>
								{#each availableFields() as f}
									<option value={f}>{f}</option>
								{/each}
							</select>
							<select
								bind:value={cond.operator}
								class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1.5 text-xs text-[var(--text-primary)]"
							>
								<option value="equals">equals</option>
								<option value="contains">contains</option>
								<option value="startsWith">starts with</option>
							</select>
							<Input bind:value={cond.value} placeholder="value" class="text-xs flex-1" />
							<button onclick={() => removeCondition(i)} class="p-1 rounded hover:bg-[var(--color-error)]/10">
								<X class="h-3.5 w-3.5 text-[var(--text-tertiary)]" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Step 3: Then (actions) -->
	{#if step === 3}
		<div class="space-y-3">
			<div class="flex items-center justify-between">
				<p class="text-sm font-medium text-[var(--text-primary)]">What should happen?</p>
				{#if actions.length < 10}
					<Button variant="outline" size="sm" class="h-7 text-xs gap-1" onclick={addAction}>
						<Plus class="h-3 w-3" /> Add action
					</Button>
				{/if}
			</div>

			{#each actions as action, i}
				<div class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] p-3 space-y-2">
					<div class="flex items-center gap-2">
						<select
							bind:value={action.type}
							class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1 text-xs text-[var(--text-primary)]"
						>
							<option value="notify">Send notification</option>
							<option value="task">Create a task</option>
						</select>
						<select
							bind:value={action.priority}
							class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1 text-xs text-[var(--text-primary)]"
						>
							<option value="1">Low</option>
							<option value="2">Medium</option>
							<option value="3">High</option>
						</select>
						{#if actions.length > 1}
							<button onclick={() => removeAction(i)} class="p-1 rounded hover:bg-[var(--color-error)]/10 ml-auto">
								<X class="h-3.5 w-3.5 text-[var(--text-tertiary)]" />
							</button>
						{/if}
					</div>
					<textarea
						bind:value={action.message}
						placeholder={action.type === 'notify' ? 'Notification message...' : 'Task description...'}
						rows={2}
						class="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-1)] px-3 py-1.5 text-sm text-[var(--text-primary)] focus:border-[var(--cairn-accent)] focus:outline-none resize-none"
					></textarea>
					<!-- Variable chips -->
					{#if variableChips().length > 0}
						<div class="flex items-center gap-1 flex-wrap">
							<span class="text-[10px] text-[var(--text-tertiary)]">Insert:</span>
							{#each variableChips() as v}
								<button
									onclick={() => insertVariable(i, v)}
									class="rounded border border-[var(--border-subtle)] bg-[var(--bg-1)] px-1.5 py-0.5 text-[10px] font-mono text-[var(--cairn-accent)] hover:bg-[var(--cairn-accent)]/10 transition-colors"
								>
									{v}
								</button>
							{/each}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<!-- Step 4: Settings -->
	{#if step === 4}
		<div class="space-y-3">
			<p class="text-sm font-medium text-[var(--text-primary)]">Name and settings</p>
			<div class="space-y-2">
				<Input bind:value={ruleName} placeholder={suggestedName()} class="text-sm" />
				<Input bind:value={ruleDesc} placeholder="Description (optional)" class="text-sm" />
			</div>
			<div class="flex items-center gap-2">
				<label for="throttle-input" class="text-xs text-[var(--text-tertiary)]">Throttle (seconds):</label>
				<input
					id="throttle-input"
					type="number"
					bind:value={throttleSec}
					min="0"
					class="w-20 rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] px-2 py-1 text-sm text-[var(--text-primary)]"
				/>
			</div>

			<!-- Summary -->
			<div class="rounded-md border border-[var(--border-subtle)] bg-[var(--bg-0)] p-3 text-xs text-[var(--text-secondary)] space-y-1">
				<p class="font-medium text-[var(--text-primary)]">Summary</p>
				<p>Trigger: {triggerCategory === 'cron' ? `Schedule: ${cronSchedule}` : `On ${triggerCategory}${selectedSource ? ` from ${selectedSource}` : ''}`}</p>
				{#if conditions.length > 0}
					<p>Conditions: {conditions.filter(c => c.value.trim()).map(c => `${c.field} ${c.operator} "${c.value}"`).join(', ')}</p>
				{/if}
				<p>Actions: {actions.map(a => a.type === 'notify' ? 'Notify' : 'Create task').join(', ')}</p>
				{#if throttleSec > 0}<p>Throttle: {throttleSec}s</p>{/if}
			</div>

			{#if error}
				<p class="text-xs text-[var(--color-error)]">{error}</p>
			{/if}
		</div>
	{/if}

	<!-- Navigation -->
	<div class="flex items-center justify-between pt-2 border-t border-[var(--border-subtle)]">
		<Button
			variant="outline"
			size="sm"
			class="h-8 text-xs gap-1"
			onclick={() => { if (step > 1) step--; else onclose?.(); }}
		>
			<ArrowLeft class="h-3 w-3" />
			{step > 1 ? 'Back' : 'Cancel'}
		</Button>
		{#if step < 4}
			<Button
				size="sm"
				class="h-8 text-xs gap-1"
				onclick={() => step++}
				disabled={!canProceed()}
			>
				Next <ArrowRight class="h-3 w-3" />
			</Button>
		{:else}
			<Button
				size="sm"
				class="h-8 text-xs gap-1"
				onclick={handleSave}
				disabled={saving || !canProceed()}
			>
				{#if saving}
					<Loader2 class="h-3 w-3 animate-spin" />
				{:else}
					<Zap class="h-3 w-3" />
				{/if}
				Create Rule
			</Button>
		{/if}
	</div>
</div>
