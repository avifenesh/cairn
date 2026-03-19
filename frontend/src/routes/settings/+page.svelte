<script lang="ts">
	import { onMount } from 'svelte';
	import { appStore, type Theme, type Density, type Mood } from '$lib/stores/app.svelte';
	import { getCosts, getStatusDetails, getEditableConfig, patchConfig, type EditableConfig } from '$lib/api/client';
	import type { McpStatus, ChannelStatus } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Separator } from '$lib/components/ui/separator';
	import { Sun, Moon, Wifi, WifiOff, DollarSign, Server, Plug, Send, MessageSquare, Hash, Database, Layers, Save, Loader2, Github } from '@lucide/svelte';

	let costs = $state<Record<string, number> | null>(null);
	let mcpStatus = $state<McpStatus | null>(null);
	let channelStatus = $state<ChannelStatus | null>(null);
	let embeddingStatus = $state<{ enabled: boolean; model: string; dimensions: number } | null>(null);
	let compactionConfig = $state<{ triggerTokens: number; keepRecent: number; maxToolOutput: number } | null>(null);

	// Editable config state
	let editTriggerTokens = $state(80000);
	let editKeepRecent = $state(10);
	let editMaxToolOutput = $state(8000);
	let editBudgetDaily = $state(0);
	let editBudgetWeekly = $state(0);
	let editSessionTimeout = $state(240);
	let editGhOwner = $state('');
	let editGhTrackedRepos = $state('');
	let editGhBotFilter = $state('');
	let editGhMetricsHours = $state(4);
	let saving = $state('');

	const knownChannels = [
		{ id: 'telegram', label: 'Telegram', icon: Send },
		{ id: 'discord', label: 'Discord', icon: MessageSquare },
		{ id: 'slack', label: 'Slack', icon: Hash },
	];

	onMount(async () => {
		try {
			const [c, details, cfg] = await Promise.all([getCosts(), getStatusDetails(), getEditableConfig()]);
			costs = c as unknown as Record<string, number>;
			mcpStatus = details.mcp;
			channelStatus = details.channels;
			embeddingStatus = details.embeddings ?? null;
			compactionConfig = details.compaction ?? null;
			if (cfg) {
				editTriggerTokens = cfg.compactionTriggerTokens ?? 80000;
				editKeepRecent = cfg.compactionKeepRecent ?? 10;
				editMaxToolOutput = cfg.compactionMaxToolOutput ?? 8000;
				editBudgetDaily = cfg.budgetDailyCap ?? 0;
				editBudgetWeekly = cfg.budgetWeeklyCap ?? 0;
				editSessionTimeout = cfg.channelSessionTimeout ?? 240;
				editGhOwner = cfg.ghOwner ?? '';
				editGhTrackedRepos = cfg.ghTrackedRepos ?? '';
				editGhBotFilter = cfg.ghBotFilter ?? '';
				editGhMetricsHours = Math.round((cfg.ghMetricsInterval ?? 14400) / 3600);
			}
		} catch {
			// handled
		}
	});

	async function saveConfig(section: string, patch: Partial<EditableConfig>) {
		saving = section;
		try {
			await patchConfig(patch);
			appStore.addNotification('Settings saved', 'success');
		} catch {
			appStore.addNotification('Failed to save settings', 'error');
		} finally {
			saving = '';
		}
	}

	function budgetPercent(spent: number, cap: number): number {
		if (!cap || cap <= 0) return 0;
		return Math.min(100, (spent / cap) * 100);
	}

	function budgetColor(pct: number): string {
		if (pct >= 90) return 'var(--color-error)';
		if (pct >= 70) return 'var(--color-warning)';
		return 'var(--color-success)';
	}

	const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
		{ value: 'dark', label: 'Dark', icon: Moon },
		{ value: 'light', label: 'Light', icon: Sun },
	];

	const densities: { value: Density; label: string; desc: string }[] = [
		{ value: 'comfortable', label: 'Comfortable', desc: 'Default spacing' },
		{ value: 'balanced', label: 'Balanced', desc: 'Tighter layout' },
		{ value: 'dense', label: 'Dense', desc: 'Maximum density' },
	];

	const moods: { value: Mood; label: string; color: string }[] = [
		{ value: 'default', label: 'Emerald', color: '#10B981' },
		{ value: 'dawn', label: 'Dawn', color: '#F59E0B' },
		{ value: 'ocean', label: 'Ocean', color: '#06B6D4' },
		{ value: 'night', label: 'Night', color: '#818CF8' },
	];

	let toastDuration = $state((() => { try { return Number(localStorage.getItem('cairn_toast_duration')) || 5; } catch { return 5; } })());

	function toggleAutoMood() {
		appStore.setAutoMood(!appStore.autoMoodEnabled);
	}

	function setToastDuration(seconds: number) {
		toastDuration = seconds;
		try { localStorage.setItem('cairn_toast_duration', String(seconds)); } catch {}
	}
</script>

<div class="mx-auto max-w-2xl p-6">
	<h1 class="mb-8 text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Settings</h1>

	<!-- Appearance -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Appearance</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Customize the look and feel</p>

		<div class="space-y-4">
			<!-- Theme -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<label class="text-xs font-medium text-[var(--text-secondary)] uppercase tracking-wider">Theme</label>
				<div class="mt-2 flex gap-2">
					{#each themes as t}
						<button
							class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition-all duration-[var(--dur-fast)]
								{appStore.theme === t.value
								? 'border-[var(--cairn-accent)] bg-[var(--accent-dim)] text-[var(--cairn-accent)] shadow-sm'
								: 'border-border-subtle text-[var(--text-secondary)] hover:bg-[var(--bg-2)] hover:border-border-default'}"
							onclick={() => appStore.setTheme(t.value)}
						>
							<t.icon class="h-4 w-4" />
							{t.label}
						</button>
					{/each}
				</div>
			</div>

			<!-- Density -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<label class="text-xs font-medium text-[var(--text-secondary)] uppercase tracking-wider">Density</label>
				<div class="mt-2 flex gap-2">
					{#each densities as d}
						<button
							class="flex flex-col items-start rounded-lg border px-3 py-2 text-left transition-all duration-[var(--dur-fast)]
								{appStore.density === d.value
								? 'border-[var(--cairn-accent)] bg-[var(--accent-dim)] shadow-sm'
								: 'border-border-subtle hover:bg-[var(--bg-2)] hover:border-border-default'}"
							onclick={() => appStore.setDensity(d.value)}
						>
							<span class="text-sm font-medium {appStore.density === d.value ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-primary)]'}">{d.label}</span>
							<span class="text-[10px] text-[var(--text-tertiary)]">{d.desc}</span>
						</button>
					{/each}
				</div>
			</div>

			<!-- Mood -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<label class="text-xs font-medium text-[var(--text-secondary)] uppercase tracking-wider">Accent Color</label>
				<div class="mt-2 flex gap-3">
					{#each moods as m}
						<button
							class="flex flex-col items-center gap-1.5 rounded-lg border px-3 py-2.5 transition-all duration-[var(--dur-fast)]
								{appStore.mood === m.value
								? 'border-[var(--cairn-accent)] bg-[var(--accent-dim)] shadow-sm'
								: 'border-border-subtle hover:bg-[var(--bg-2)]'}"
							onclick={() => appStore.setMood(m.value)}
						>
							<span class="h-5 w-5 rounded-full ring-2 ring-offset-2 ring-offset-[var(--bg-1)]
								{appStore.mood === m.value ? 'ring-[var(--cairn-accent)]' : 'ring-transparent'}"
								style="background: {m.color}"
							></span>
							<span class="text-[10px] text-[var(--text-secondary)]">{m.label}</span>
						</button>
					{/each}
				</div>
				<label class="mt-3 flex items-center gap-2 text-xs text-[var(--text-tertiary)] cursor-pointer">
					<input
						type="checkbox"
						checked={appStore.autoMoodEnabled}
						onchange={toggleAutoMood}
						class="h-3.5 w-3.5 rounded accent-[var(--cairn-accent)]"
					/>
					Auto-mood (changes by time of day)
				</label>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- Notifications -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Notifications</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Control how notifications behave</p>

		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-sm text-[var(--text-primary)]">Toast duration</p>
					<p class="text-[10px] text-[var(--text-tertiary)]">How long notifications stay visible</p>
				</div>
				<div class="flex items-center gap-1 rounded-md border border-border-subtle bg-[var(--bg-0)] p-0.5">
					{#each [3, 5, 8] as sec}
						<button
							class="rounded px-2.5 py-1 text-xs font-mono transition-colors duration-[var(--dur-fast)]
								{toastDuration === sec
								? 'bg-[var(--bg-2)] text-[var(--cairn-accent)]'
								: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
							onclick={() => setToastDuration(sec)}
						>
							{sec}s
						</button>
					{/each}
				</div>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- Budget -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Budget</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">LLM spend tracking</p>

		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-4">
			{#if costs}
				{@const dailySpent = costs.todayUsd ?? costs.today ?? 0}
				{@const dailyCap = costs.budgetDailyUsd ?? 0}
				{@const weeklySpent = costs.weekUsd ?? costs.thisMonth ?? 0}
				{@const weeklyCap = costs.budgetWeeklyUsd ?? 0}
				{@const dailyPct = budgetPercent(dailySpent, dailyCap)}
				{@const weeklyPct = budgetPercent(weeklySpent, weeklyCap)}

				<div>
					<div class="flex items-center justify-between mb-1">
						<span class="text-xs text-[var(--text-secondary)]">Today</span>
						<span class="text-xs text-[var(--text-primary)] font-mono tabular-nums">
							${dailySpent.toFixed(4)}{#if dailyCap > 0} / ${dailyCap.toFixed(2)}{/if}
						</span>
					</div>
					{#if dailyCap > 0}
						<div class="h-1.5 rounded-full bg-[var(--bg-3)] overflow-hidden">
							<div class="h-full rounded-full transition-all" style="width: {dailyPct}%; background: {budgetColor(dailyPct)}"></div>
						</div>
					{/if}
				</div>

				<div>
					<div class="flex items-center justify-between mb-1">
						<span class="text-xs text-[var(--text-secondary)]">This week</span>
						<span class="text-xs text-[var(--text-primary)] font-mono tabular-nums">
							${weeklySpent.toFixed(4)}{#if weeklyCap > 0} / ${weeklyCap.toFixed(2)}{/if}
						</span>
					</div>
					{#if weeklyCap > 0}
						<div class="h-1.5 rounded-full bg-[var(--bg-3)] overflow-hidden">
							<div class="h-full rounded-full transition-all" style="width: {weeklyPct}%; background: {budgetColor(weeklyPct)}"></div>
						</div>
					{/if}
				</div>
			{:else}
				<div class="flex items-center gap-2 text-xs text-[var(--text-tertiary)]">
					<DollarSign class="h-3.5 w-3.5" />
					<span>Loading budget data...</span>
				</div>
			{/if}

			<div class="border-t border-border-subtle pt-3 mt-1">
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Budget Caps (USD, 0 = unlimited)</p>
				<div class="grid grid-cols-2 gap-3">
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] mb-1">Daily</p>
						<Input type="number" bind:value={editBudgetDaily} min={0} step={0.5} class="h-7 text-xs font-mono" />
					</div>
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] mb-1">Weekly</p>
						<Input type="number" bind:value={editBudgetWeekly} min={0} step={1} class="h-7 text-xs font-mono" />
					</div>
				</div>
				<div class="flex justify-end mt-2">
					<Button
						size="sm" class="h-7 text-xs gap-1 px-3"
						onclick={() => saveConfig('budget', { budgetDailyCap: editBudgetDaily, budgetWeeklyCap: editBudgetWeekly })}
						disabled={saving === 'budget'}
					>
						{#if saving === 'budget'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
						Save
					</Button>
				</div>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- Connection -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Connection</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Real-time connection status</p>

		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center gap-3">
				{#if appStore.sseConnected}
					<div class="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--color-success)]/10">
						<Wifi class="h-4 w-4 text-[var(--color-success)]" />
					</div>
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Connected</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">SSE stream active — real-time updates enabled</p>
					</div>
					<span class="ml-auto h-2 w-2 rounded-full bg-[var(--color-success)] animate-pulse-dot"></span>
				{:else}
					<div class="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--color-error)]/10">
						<WifiOff class="h-4 w-4 text-[var(--color-error)]" />
					</div>
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Disconnected</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Attempting to reconnect...</p>
					</div>
					<span class="ml-auto h-2 w-2 rounded-full bg-[var(--color-error)]"></span>
				{/if}
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- MCP Server -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">MCP Server</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Model Context Protocol server configuration</p>

		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center gap-3">
				<div class="flex h-8 w-8 items-center justify-center rounded-md {mcpStatus?.enabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
					<Server class="h-4 w-4 {mcpStatus?.enabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
				</div>
				<div>
					<p class="text-sm font-medium text-[var(--text-primary)]">{mcpStatus?.enabled ? 'Enabled' : 'Disabled'}</p>
					<p class="text-[10px] text-[var(--text-tertiary)]">
						{mcpStatus?.enabled ? `Port ${mcpStatus.port} · ${mcpStatus.transport} transport` : 'MCP server is not running'}
					</p>
				</div>
				<span class="ml-auto h-2 w-2 rounded-full {mcpStatus?.enabled ? 'bg-[var(--color-success)]' : 'bg-[var(--text-tertiary)]'}"></span>
			</div>
			{#if mcpStatus?.enabled}
				<div class="grid grid-cols-2 gap-3 pt-3 mt-3 border-t border-border-subtle">
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Port</p>
						<p class="text-xs text-[var(--text-primary)] font-mono">{mcpStatus.port}</p>
					</div>
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Transport</p>
						<p class="text-xs text-[var(--text-primary)] font-mono">{mcpStatus.transport}</p>
					</div>
				</div>
			{/if}
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- MCP Connections -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">MCP Connections</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">External MCP client connections</p>

		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center gap-3">
				<Plug class="h-4 w-4 text-[var(--text-tertiary)]" />
				<div>
					<p class="text-sm text-[var(--text-tertiary)]">No MCP clients connected</p>
					<p class="text-[10px] text-[var(--text-tertiary)]/60">Connect Claude Code, Cursor, or other MCP clients</p>
				</div>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- Channels -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Channels</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">External messaging platform integrations</p>

		<div class="space-y-3">
			{#each knownChannels as ch}
				{@const active = channelStatus?.items.some(i => i.name === ch.id && i.connected) ?? false}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
					<div class="flex items-center gap-3">
						<div class="flex h-8 w-8 items-center justify-center rounded-md {active ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
							<ch.icon class="h-4 w-4 {active ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
						</div>
						<div>
							<p class="text-sm font-medium text-[var(--text-primary)]">{ch.label}</p>
							<p class="text-[10px] text-[var(--text-tertiary)]">
								{active ? 'Connected' : 'Not configured'}
							</p>
						</div>
						<span class="ml-auto h-2 w-2 rounded-full {active ? 'bg-[var(--color-success)]' : 'bg-[var(--text-tertiary)]'}"></span>
					</div>
				</div>
			{/each}
		</div>

		<div class="mt-3 rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center justify-between">
				<div class="flex-1">
					<p class="text-sm text-[var(--text-primary)]">Session timeout</p>
					<p class="text-[10px] text-[var(--text-tertiary)]">Idle channel sessions expire after this duration (minutes)</p>
				</div>
				<div class="flex items-center gap-2">
					<Input type="number" bind:value={editSessionTimeout} min={1} max={1440} class="h-7 w-20 text-xs font-mono text-right" />
					<Button
						size="sm" class="h-7 text-xs gap-1 px-2"
						onclick={() => saveConfig('channels', { channelSessionTimeout: editSessionTimeout })}
						disabled={saving === 'channels'}
					>
						{#if saving === 'channels'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
					</Button>
				</div>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- Intelligence -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Intelligence</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Embeddings and session compaction</p>

		<div class="space-y-3">
			<!-- Embeddings -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md {embeddingStatus?.enabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
						<Database class="h-4 w-4 {embeddingStatus?.enabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
					</div>
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Embeddings</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">
							{embeddingStatus?.enabled ? `${embeddingStatus.model} · ${embeddingStatus.dimensions}d` : 'Not configured'}
						</p>
					</div>
					<span class="ml-auto h-2 w-2 rounded-full {embeddingStatus?.enabled ? 'bg-[var(--color-success)]' : 'bg-[var(--text-tertiary)]'}"></span>
				</div>
			</div>

			<!-- Compaction (editable) -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3 mb-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--cairn-accent)]/10">
						<Layers class="h-4 w-4 text-[var(--cairn-accent)]" />
					</div>
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Session Compaction</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Controls when long conversations are summarized</p>
					</div>
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Trigger (tokens)</p>
						<Input type="number" bind:value={editTriggerTokens} min={10000} max={200000} step={10000} class="h-7 text-xs font-mono" />
					</div>
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Keep Recent (pairs)</p>
						<Input type="number" bind:value={editKeepRecent} min={1} max={50} class="h-7 text-xs font-mono" />
					</div>
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Max Tool Output</p>
						<Input type="number" bind:value={editMaxToolOutput} min={1000} max={50000} step={1000} class="h-7 text-xs font-mono" />
					</div>
				</div>
				<div class="flex justify-end mt-3">
					<Button
						size="sm" class="h-7 text-xs gap-1 px-3"
						onclick={() => saveConfig('compaction', { compactionTriggerTokens: editTriggerTokens, compactionKeepRecent: editKeepRecent, compactionMaxToolOutput: editMaxToolOutput })}
						disabled={saving === 'compaction'}
					>
						{#if saving === 'compaction'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
						Save
					</Button>
				</div>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- GitHub Signal -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">GitHub Signal</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">External engagement tracking, growth metrics, stargazers, followers</p>

		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-4">
			<div class="flex items-center gap-3 mb-1">
				<div class="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--cairn-accent)]/10">
					<Github class="h-4 w-4 text-[var(--cairn-accent)]" />
				</div>
				<div>
					<p class="text-sm font-medium text-[var(--text-primary)]">Signal Intelligence</p>
					<p class="text-[10px] text-[var(--text-tertiary)]">Tracks external issues, PRs, comments, stars, forks, followers. Filters bots.</p>
				</div>
			</div>

			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">GitHub Username</p>
				<Input type="text" bind:value={editGhOwner} placeholder="avifenesh" class="h-7 text-xs font-mono" />
				<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">Your login - used to filter out your own activity</p>
			</div>

			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Tracked Repos</p>
				<textarea
					bind:value={editGhTrackedRepos}
					placeholder="owner/repo1, owner/repo2 (empty = auto-detect)"
					class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs font-mono text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/40 focus:border-[var(--cairn-accent)] focus:outline-none resize-none h-16"
				></textarea>
				<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">Comma-separated. Empty = auto-detect from your repos + orgs</p>
			</div>

			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Additional Bot Filter</p>
				<Input type="text" bind:value={editGhBotFilter} placeholder="my-ci-bot, internal-bot" class="h-7 text-xs font-mono" />
				<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">Extra bot logins to filter (common bots already included)</p>
			</div>

			<div>
				<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Metrics Interval (hours)</p>
				<Input type="number" bind:value={editGhMetricsHours} min={1} max={48} class="h-7 w-24 text-xs font-mono" />
				<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">How often to check stars, forks, followers (default 4h)</p>
			</div>

			<div class="flex justify-end pt-1">
				<Button
					size="sm" class="h-7 text-xs gap-1 px-3"
					onclick={() => saveConfig('ghsignal', {
						ghOwner: editGhOwner,
						ghTrackedRepos: editGhTrackedRepos,
						ghBotFilter: editGhBotFilter,
						ghMetricsInterval: editGhMetricsHours * 3600,
					})}
					disabled={saving === 'ghsignal'}
				>
					{#if saving === 'ghsignal'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
					Save
				</Button>
			</div>
		</div>
	</section>
</div>
