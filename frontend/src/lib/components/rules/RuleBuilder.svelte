<script lang="ts">
	import type { SourceInfo, RuleTemplate, Rule, RuleTrigger, RuleAction } from '$lib/types';
	import { createRule } from '$lib/api/client';
	import { ruleStore } from '$lib/stores/rules.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Loader2, ArrowRight, ArrowLeft, Zap, Antenna, ListTodo, Brain, Clock, Plus, X, Check } from '@lucide/svelte';

	interface Props {
		sources: SourceInfo[];
		prefill?: RuleTemplate;
		onclose?: () => void;
		oncreated?: (rule: Rule) => void;
	}
	let { sources, prefill, onclose, oncreated }: Props = $props();

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
	let actions = $state<ActionDraft[]>([{ type: 'notify', message: '', priority: '2' }]);

	// Step 4: Settings
	let ruleName = $state('');
	let ruleDesc = $state('');
	let throttleSec = $state(60);

	const stepMeta = [
		{ num: 1, label: 'When', color: 'var(--cairn-accent)' },
		{ num: 2, label: 'Filter', color: 'var(--color-warning)' },
		{ num: 3, label: 'Then', color: 'var(--src-github)' },
		{ num: 4, label: 'Save', color: 'var(--src-x)' },
	];

	const triggerOptions = [
		{ key: 'signal', label: 'Signal arrives', sub: 'GitHub, HN, Reddit, Gmail...', icon: Antenna, color: 'var(--cairn-accent)' },
		{ key: 'task-failed', label: 'Task fails', sub: 'Error alerts', icon: ListTodo, color: 'var(--color-error)' },
		{ key: 'task-created', label: 'Task created', sub: 'New work items', icon: ListTodo, color: 'var(--color-warning)' },
		{ key: 'task-completed', label: 'Task completes', sub: 'Success tracking', icon: ListTodo, color: 'var(--color-success)' },
		{ key: 'memory', label: 'Memory proposed', sub: 'Knowledge extraction', icon: Brain, color: 'var(--src-github)' },
		{ key: 'cron', label: 'On schedule', sub: 'Cron expression', icon: Clock, color: 'var(--src-x)' },
	];

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

	const availableKinds = $derived(() => {
		if (triggerCategory !== 'signal' || !selectedSource) return [];
		return sources.find(s => s.name === selectedSource)?.kinds ?? [];
	});

	const suggestedName = $derived(() => {
		const actionLabel = actions[0]?.type === 'notify' ? 'Notify' : 'Task';
		if (triggerCategory === 'signal') {
			const src = sources.find(s => s.name === selectedSource)?.label ?? selectedSource ?? 'any source';
			if (selectedKinds.length === 1) return `${actionLabel} on ${src} ${selectedKinds[0]}`;
			return `${actionLabel} on ${src} events`;
		}
		const labels: Record<string, string> = {
			'task-created': 'task creation', 'task-completed': 'task completion',
			'task-failed': 'task failure', 'memory': 'memory proposals', 'cron': 'schedule',
		};
		return `${actionLabel} on ${labels[triggerCategory] ?? triggerCategory}`;
	});

	$effect(() => {
		if (!prefill) return;
		if (prefill.source) selectedSource = prefill.source;
		if (prefill.category === 'signal') triggerCategory = 'signal';
		else if (prefill.id?.includes('completed')) triggerCategory = 'task-completed';
		else if (prefill.id?.includes('created')) triggerCategory = 'task-created';
		else if (prefill.category === 'task') triggerCategory = 'task-failed';
		else if (prefill.category === 'memory') triggerCategory = 'memory';
		else if (prefill.category === 'scheduled') triggerCategory = 'cron';
		ruleName = prefill.name;
	});

	function addCondition() {
		const fields = availableFields();
		conditions = [...conditions, { field: fields[0] ?? 'title', operator: 'contains', value: '' }];
	}
	function removeCondition(i: number) { conditions = conditions.filter((_, idx) => idx !== i); }
	function addAction() { if (actions.length < 10) actions = [...actions, { type: 'notify', message: '', priority: '2' }]; }
	function removeAction(i: number) { if (actions.length > 1) actions = actions.filter((_, idx) => idx !== i); }
	function insertVariable(actionIdx: number, varName: string) { actions[actionIdx].message += `{{.${varName}}}`; }

	function escapeExprString(value: string): string {
		const json = JSON.stringify(value);
		return json.slice(1, -1);
	}

	function buildTrigger(): RuleTrigger {
		if (triggerCategory === 'cron') return { type: 'cron', schedule: cronSchedule };
		const eventTypeMap: Record<string, string> = {
			'signal': 'EventIngested', 'task-created': 'TaskCreated',
			'task-completed': 'TaskCompleted', 'task-failed': 'TaskFailed', 'memory': 'MemoryProposed',
		};
		const trigger: RuleTrigger = { type: 'event', eventType: eventTypeMap[triggerCategory] ?? 'EventIngested' };
		const filter: Record<string, string> = {};
		if (triggerCategory === 'signal' && selectedSource) filter['sourceType'] = selectedSource;
		if (selectedKinds.length === 1) filter['kind'] = selectedKinds[0];
		for (const c of conditions) {
			// Skip fields already set by source/kind selectors to prevent overwrite.
			if (c.operator === 'equals' && c.value.trim() && !filter[c.field]) {
				filter[c.field] = c.value.trim();
			}
		}
		if (Object.keys(filter).length > 0) trigger.filter = filter;
		return trigger;
	}

	function buildCondition(): string {
		const parts: string[] = [];
		if (selectedKinds.length > 1) {
			parts.push(`(${selectedKinds.map(k => `kind == "${escapeExprString(k)}"`).join(' || ')})`);
		}
		for (const c of conditions) {
			const v = c.value.trim();
			if (!v) continue;
			const esc = escapeExprString(v);
			if (c.operator === 'contains') parts.push(`${c.field} contains "${esc}"`);
			else if (c.operator === 'startsWith') parts.push(`${c.field} startsWith "${esc}"`);
		}
		return parts.join(' && ');
	}

	function buildActions(): RuleAction[] {
		return actions.map(a => {
			const params: Record<string, string> = { priority: a.priority };
			if (a.type === 'notify') params.message = a.message;
			else { params.description = a.message; params.type = 'general'; }
			return { type: a.type, params };
		});
	}

	async function handleSave() {
		saving = true; error = '';
		try {
			const rule: Partial<Rule> = {
				name: ruleName || suggestedName(), description: ruleDesc, enabled: true,
				trigger: buildTrigger(), condition: buildCondition(),
				actions: buildActions(), throttleMs: throttleSec * 1000,
			};
			const res = await createRule(rule);
			ruleStore.addRule(res.rule);
			oncreated?.(res.rule);
			onclose?.();
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to create rule';
		} finally { saving = false; }
	}

	// Auto-generate a default message when entering step 3 with empty actions.
	function ensureDefaultMessage() {
		if (actions[0]?.message) return;
		const vars = availableFields();
		if (triggerCategory === 'signal') {
			const src = sources.find(s => s.name === selectedSource)?.label ?? 'Signal';
			actions[0].message = vars.includes('title') ? `[${src}] {{.title}}` : `New ${src} event`;
			if (vars.includes('url')) actions[0].message += ` — {{.url}}`;
		} else if (triggerCategory === 'task-failed') {
			actions[0].message = 'Task failed: {{.error}}';
			actions[0].priority = '3';
		} else if (triggerCategory === 'task-completed') {
			actions[0].message = 'Task completed: {{.taskId}}';
		} else if (triggerCategory === 'task-created') {
			actions[0].message = 'New task: {{.description}}';
		} else if (triggerCategory === 'memory') {
			actions[0].message = 'Memory proposed: {{.content}}';
		}
	}

	function goNext() {
		if (step === 1) {
			// Auto-skip filter step for non-signal triggers (they rarely need conditions).
			if (triggerCategory !== 'signal') {
				step = 3;
				ensureDefaultMessage();
				return;
			}
			step = 2;
		} else if (step === 2) {
			step = 3;
			ensureDefaultMessage();
		} else if (step < 4) {
			step++;
		}
	}

	function goBack() {
		if (step === 3 && triggerCategory !== 'signal') {
			// Skip back over the filter step we auto-skipped.
			step = 1;
		} else if (step > 1) {
			step--;
		} else {
			onclose?.();
		}
	}

	const canProceed = $derived(() => {
		if (step === 1 && triggerCategory === 'cron') return cronSchedule.trim().length > 0;
		if (step === 3) return actions.some(a => a.message.trim().length > 0);
		return true;
	});
</script>

<!-- Builder container -->
<div class="rounded-xl border overflow-hidden animate-in" style="border-color: var(--border-default); background: var(--bg-1)">

	<!-- Pipeline step indicator -->
	<div class="flex items-center px-4 py-3 gap-0" style="background: var(--bg-0); border-bottom: 1px solid var(--border-subtle)">
		{#each stepMeta as s, i}
			{@const isActive = step === s.num}
			{@const isDone = step > s.num}
			{@const color = isActive ? s.color : isDone ? 'var(--cairn-accent)' : 'var(--text-tertiary)'}

			<!-- Connector line (before each step except first) -->
			{#if i > 0}
				<div
					class="h-px flex-1 mx-1 transition-all"
					style="background: {isDone ? 'var(--cairn-accent)' : 'var(--border-subtle)'}; max-width: 48px"
				></div>
			{/if}

			<button
				onclick={() => { if (isDone) step = s.num; }}
				class="flex items-center gap-1.5 transition-all"
				class:cursor-pointer={isDone}
				class:cursor-default={!isDone}
			>
				<span
					class="flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-mono font-medium transition-all"
					style="
						background: {isActive ? color : isDone ? 'var(--cairn-accent)' : 'var(--bg-2)'};
						color: {isActive || isDone ? 'var(--bg-0)' : 'var(--text-tertiary)'};
					"
				>
					{#if isDone}<Check class="h-3 w-3" />{:else}{s.num}{/if}
				</span>
				<span
					class="text-[11px] font-medium hidden sm:inline transition-colors"
					style="color: {isActive ? 'var(--text-primary)' : isDone ? 'var(--cairn-accent)' : 'var(--text-tertiary)'}"
				>
					{s.label}
				</span>
			</button>
		{/each}

		<div class="flex-1"></div>
		<button onclick={() => onclose?.()} class="p-1 rounded-md transition-colors hover:bg-[var(--bg-2)]" style="color: var(--text-tertiary)" aria-label="Close builder">
			<X class="h-4 w-4" />
		</button>
	</div>

	<!-- Step content -->
	<div class="p-4 space-y-4">

		<!-- STEP 1: WHEN -->
		{#if step === 1}
			<div class="space-y-4 animate-in">
				<p class="text-xs uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">
					When should this rule fire?
				</p>

				<div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
					{#each triggerOptions as opt, i}
						{@const Icon = opt.icon}
						{@const isSelected = triggerCategory === opt.key}
						<button
							onclick={() => triggerCategory = opt.key as TriggerCategory}
							aria-pressed={isSelected}
							aria-label={opt.label}
							class="relative rounded-lg border p-3 text-left transition-all animate-in"
							style="
								border-color: {isSelected ? opt.color : 'var(--border-subtle)'};
								background: {isSelected ? `color-mix(in srgb, ${opt.color} 6%, var(--bg-0))` : 'var(--bg-0)'};
								animation-delay: {i * 30}ms;
							"
						>
							{#if isSelected}
								<div class="absolute top-2 right-2 h-1.5 w-1.5 rounded-full" style="background: {opt.color}"></div>
							{/if}
							<Icon class="h-4 w-4 mb-1.5" style="color: {isSelected ? opt.color : 'var(--text-tertiary)'}" />
							<p class="text-xs font-medium" style="color: {isSelected ? 'var(--text-primary)' : 'var(--text-secondary)'}">{opt.label}</p>
							<p class="text-[10px] mt-0.5" style="color: var(--text-tertiary)">{opt.sub}</p>
						</button>
					{/each}
				</div>

				{#if triggerCategory === 'signal'}
					<div class="space-y-3 rounded-lg border p-3" style="border-color: var(--border-subtle); background: var(--bg-0)">
						<div class="space-y-1.5">
							<label for="source-select" class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Source</label>
							<select
								id="source-select"
								bind:value={selectedSource}
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--border-subtle); background: var(--bg-1); color: var(--text-primary)"
							>
								<option value="">Any source</option>
								{#each sources as src}
									<option value={src.name}>{src.label}</option>
								{/each}
							</select>
						</div>

						{#if availableKinds().length > 0}
							<div class="space-y-1.5">
								<span class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Event kind</span>
								<div class="flex flex-wrap gap-1.5">
									{#each availableKinds() as kind}
										{@const isSelected = selectedKinds.includes(kind)}
										<button
											onclick={() => {
												if (isSelected) selectedKinds = selectedKinds.filter(k => k !== kind);
												else selectedKinds = [...selectedKinds, kind];
											}}
											class="rounded-md border px-2 py-1 text-xs font-mono transition-all"
											style="
												border-color: {isSelected ? 'var(--cairn-accent)' : 'var(--border-subtle)'};
												background: {isSelected ? 'var(--accent-dim)' : 'transparent'};
												color: {isSelected ? 'var(--cairn-accent)' : 'var(--text-tertiary)'};
											"
										>
											{kind}
										</button>
									{/each}
								</div>
							</div>
						{/if}
					</div>
				{/if}

				{#if triggerCategory === 'cron'}
					<div class="rounded-lg border p-3 space-y-1.5" style="border-color: var(--border-subtle); background: var(--bg-0)">
						<label for="cron-input" class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Cron expression</label>
						<Input id="cron-input" bind:value={cronSchedule} placeholder="0 9 * * *" class="font-mono text-sm" />
						<p class="text-[10px] font-mono" style="color: var(--text-tertiary)">min hour day month weekday</p>
					</div>
				{/if}
			</div>
		{/if}

		<!-- STEP 2: FILTER -->
		{#if step === 2}
			<div class="space-y-3 animate-in">
				<div class="flex items-center justify-between">
					<p class="text-xs uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Conditions</p>
					<button
						onclick={addCondition}
						class="flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-medium transition-colors"
						style="border-color: var(--border-subtle); color: var(--text-secondary); background: var(--bg-0)"
					>
						<Plus class="h-3 w-3" /> Add filter
					</button>
				</div>

				{#if conditions.length === 0}
					<div class="rounded-lg border border-dashed p-6 text-center" style="border-color: var(--border-subtle)">
						<p class="text-xs" style="color: var(--text-tertiary)">No filters - matches all events of this type</p>
						<p class="text-[10px] mt-1" style="color: var(--text-tertiary); opacity: 0.6">This step is optional</p>
					</div>
				{:else}
					<div class="space-y-2">
						{#each conditions as cond, i}
							<div class="flex items-center gap-1.5 animate-in" style="animation-delay: {i * 40}ms">
								<select
									bind:value={cond.field}
									class="rounded-md border px-2 py-1.5 text-xs font-mono"
									style="border-color: var(--border-subtle); background: var(--bg-0); color: var(--text-primary)"
								>
									{#each availableFields() as f}
										<option value={f}>{f}</option>
									{/each}
								</select>
								<select
									bind:value={cond.operator}
									class="rounded-md border px-2 py-1.5 text-xs"
									style="border-color: var(--border-subtle); background: var(--bg-0); color: var(--cairn-accent)"
								>
									<option value="equals">==</option>
									<option value="contains">contains</option>
									<option value="startsWith">starts with</option>
								</select>
								<input
									bind:value={cond.value}
									placeholder="value"
									class="flex-1 rounded-md border px-2 py-1.5 text-xs font-mono focus:outline-none"
									style="border-color: var(--border-subtle); background: var(--bg-0); color: var(--text-primary); min-width: 0"
								/>
								<button onclick={() => removeCondition(i)} class="p-1 rounded transition-colors hover:bg-[var(--color-error)]/10" aria-label="Remove filter">
									<X class="h-3.5 w-3.5" style="color: var(--text-tertiary)" />
								</button>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<!-- STEP 3: THEN -->
		{#if step === 3}
			<div class="space-y-3 animate-in">
				<div class="flex items-center justify-between">
					<p class="text-xs uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Actions</p>
					{#if actions.length < 10}
						<button
							onclick={addAction}
							class="flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-medium transition-colors"
							style="border-color: var(--border-subtle); color: var(--text-secondary); background: var(--bg-0)"
						>
							<Plus class="h-3 w-3" /> Add
						</button>
					{/if}
				</div>

				{#each actions as action, i}
					<div class="rounded-lg border p-3 space-y-2.5 animate-in" style="border-color: var(--border-subtle); background: var(--bg-0); animation-delay: {i * 40}ms">
						<div class="flex items-center gap-2">
							<select
								bind:value={action.type}
								class="rounded-md border px-2 py-1 text-xs font-medium"
								style="border-color: var(--border-subtle); background: var(--bg-1); color: var(--text-primary)"
							>
								<option value="notify">Notify</option>
								<option value="task">Create task</option>
							</select>
							<select
								bind:value={action.priority}
								class="rounded-md border px-2 py-1 text-xs"
								style="border-color: var(--border-subtle); background: var(--bg-1); color: var(--text-secondary)"
							>
								<option value="1">Low</option>
								<option value="2">Normal</option>
								<option value="3">Urgent</option>
							</select>
							{#if actions.length > 1}
								<button onclick={() => removeAction(i)} class="ml-auto p-1 rounded transition-colors hover:bg-[var(--color-error)]/10" aria-label="Remove action">
									<X class="h-3.5 w-3.5" style="color: var(--text-tertiary)" />
								</button>
							{/if}
						</div>

						<textarea
							bind:value={action.message}
							placeholder={action.type === 'notify' ? 'Message text...' : 'Task description...'}
							rows={2}
							class="w-full rounded-md border px-3 py-2 text-sm font-mono focus:outline-none resize-none"
							style="border-color: var(--border-subtle); background: var(--bg-1); color: var(--text-primary)"
						></textarea>

						<!-- Variable chips -->
						{#if availableFields().length > 0}
							<div class="flex items-center gap-1 flex-wrap">
								<span class="text-[9px] uppercase tracking-wider" style="color: var(--text-tertiary)">vars</span>
								{#each availableFields() as v}
									<button
										onclick={() => insertVariable(i, v)}
										class="rounded border px-1.5 py-0.5 text-[10px] font-mono transition-all hover:scale-105"
										style="border-color: var(--border-subtle); background: var(--bg-1); color: var(--cairn-accent)"
									>
										.{v}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		<!-- STEP 4: SETTINGS -->
		{#if step === 4}
			<div class="space-y-4 animate-in">
				<p class="text-xs uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Configuration</p>

				<div class="space-y-3">
					<div class="space-y-1.5">
						<label for="rule-name" class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Name</label>
						<Input id="rule-name" bind:value={ruleName} placeholder={suggestedName()} class="text-sm" />
					</div>
					<div class="space-y-1.5">
						<label for="rule-desc" class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Description</label>
						<Input id="rule-desc" bind:value={ruleDesc} placeholder="Optional" class="text-sm" />
					</div>
					<div class="space-y-1.5">
						<label for="throttle-input" class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--text-tertiary)">Cooldown</label>
						<div class="flex items-center gap-2">
							<input
								id="throttle-input" type="number" bind:value={throttleSec} min="0"
								class="w-20 rounded-md border px-2 py-1.5 text-sm font-mono"
								style="border-color: var(--border-subtle); background: var(--bg-0); color: var(--text-primary)"
							/>
							<span class="text-xs" style="color: var(--text-tertiary)">seconds between fires</span>
						</div>
					</div>
				</div>

				<!-- Summary card -->
				<div class="rounded-lg border p-3 space-y-1.5 font-mono text-[11px]" style="border-color: var(--border-subtle); background: var(--bg-0)">
					<p class="text-[9px] uppercase tracking-wider font-sans font-medium mb-2" style="color: var(--text-tertiary)">Preview</p>
					<p><span style="color: var(--cairn-accent)">when</span> <span style="color: var(--text-primary)">{triggerCategory === 'cron' ? `schedule "${cronSchedule}"` : triggerCategory}{selectedSource ? ` from ${selectedSource}` : ''}</span></p>
					{#if conditions.length > 0}
						<p><span style="color: var(--color-warning)">where</span> <span style="color: var(--text-secondary)">{conditions.filter(c => c.value.trim()).map(c => `${c.field} ${c.operator === 'equals' ? '==' : c.operator} ${JSON.stringify(c.value)}`).join(' and ')}</span></p>
					{/if}
					<p><span style="color: var(--src-github)">then</span> <span style="color: var(--text-secondary)">{actions.map(a => a.type === 'notify' ? 'notify' : 'create task').join(', ')}</span></p>
					{#if throttleSec > 0}<p><span style="color: var(--src-x)">cooldown</span> <span style="color: var(--text-secondary)">{throttleSec}s</span></p>{/if}
				</div>

				{#if error}
					<div class="rounded-lg border px-3 py-2 text-xs" style="border-color: var(--color-error); background: color-mix(in srgb, var(--color-error) 8%, var(--bg-1)); color: var(--color-error)">
						{error}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Navigation bar -->
	<div class="flex items-center justify-between px-4 py-3" style="border-top: 1px solid var(--border-subtle); background: var(--bg-0)">
		<Button
			variant="ghost"
			size="sm"
			class="h-8 text-xs gap-1"
			onclick={goBack}
		>
			<ArrowLeft class="h-3 w-3" />
			{step > 1 ? 'Back' : 'Cancel'}
		</Button>

		{#if step < 4}
			<Button
				size="sm"
				class="h-8 text-xs gap-1.5"
				onclick={goNext}
				disabled={!canProceed()}
			>
				Next <ArrowRight class="h-3 w-3" />
			</Button>
		{:else}
			<Button
				size="sm"
				class="h-8 text-xs gap-1.5"
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
