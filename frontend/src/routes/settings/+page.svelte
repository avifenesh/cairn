<script lang="ts">
	import { onMount } from 'svelte';
	import { appStore, type Theme, type Density, type Mood } from '$lib/stores/app.svelte';
	import { getCosts, getStatusDetails, getEditableConfig, patchConfig, type EditableConfig, authRegisterStart, authRegisterComplete, listAuthCredentials, deleteAuthCredential, authLogout, type AuthCredential } from '$lib/api/client';
	import type { McpStatus, ChannelStatus } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Separator } from '$lib/components/ui/separator';
	import { Sun, Moon, Wifi, WifiOff, DollarSign, Server, Plug, Send, MessageSquare, Hash, Database, Layers, Save, Loader2, Github, Mail, Calendar, Rss, Code, BookOpen, Package, Bell, BellOff, Clock, Route, Fingerprint, Shield, Trash2 } from '@lucide/svelte';
	import CronManager from '$lib/components/cron/CronManager.svelte';

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
	let editGmailEnabled = $state(false);
	let editCalendarEnabled = $state(false);
	let editGmailFilter = $state('-category:promotions -category:social -category:forums');
	let editCalendarLookahead = $state(48);
	let editRssEnabled = $state(false);
	let editRssFeeds = $state('');
	let editSoEnabled = $state(false);
	let editSoTags = $state('');
	let editDevtoEnabled = $state(false);
	let editDevtoTags = $state('');
	let editDevtoUsername = $state('');
	let editNpmPackages = $state('');
	let editCratesPackages = $state('');
	let editPreferredChannel = $state('');
	let editQuietStart = $state(-1);
	let editQuietEnd = $state(-1);
	let editQuietTZ = $state('UTC');
	let editMutedSources = $state('');
	let editNotifPriority = $state('low');
	let editChannelRouting = $state('');
	let saving = $state('');

	const ALL_SOURCES = ['github', 'github_signal', 'gmail', 'calendar', 'hn', 'reddit', 'npm', 'crates', 'rss', 'stackoverflow', 'devto'];
	const CHANNELS = ['web', 'telegram', 'discord', 'slack'];
	const PRIORITIES = [
		{ value: 'low', label: 'All', desc: 'Every event' },
		{ value: 'medium', label: 'Medium+', desc: 'Skip low-priority' },
		{ value: 'high', label: 'High only', desc: 'Critical only' },
	];

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
				editGmailEnabled = cfg.gmailEnabled ?? false;
				editCalendarEnabled = cfg.calendarEnabled ?? false;
				editGmailFilter = cfg.gmailFilterQuery ?? '-category:promotions -category:social -category:forums';
				editCalendarLookahead = cfg.calendarLookaheadH ?? 48;
				editRssEnabled = cfg.rssEnabled ?? false;
				editRssFeeds = cfg.rssFeeds ?? '';
				editSoEnabled = cfg.soEnabled ?? false;
				editSoTags = cfg.soTags ?? '';
				editDevtoEnabled = cfg.devtoEnabled ?? false;
				editDevtoTags = cfg.devtoTags ?? '';
				editDevtoUsername = cfg.devtoUsername ?? '';
				editNpmPackages = cfg.npmPackages ?? '';
				editCratesPackages = cfg.cratesPackages ?? '';
				editPreferredChannel = cfg.preferredChannel ?? '';
				editQuietStart = cfg.quietHoursStart ?? -1;
				editQuietEnd = cfg.quietHoursEnd ?? -1;
				editQuietTZ = cfg.quietHoursTZ ?? 'UTC';
				editMutedSources = cfg.mutedSources ?? '';
				editNotifPriority = cfg.notifMinPriority ?? 'low';
				editChannelRouting = cfg.channelRouting ?? '';
			}
		} catch {
			// handled
		}
		// Load WebAuthn credentials.
		try {
			const res = await listAuthCredentials();
			credentials = res.credentials ?? [];
		} catch {}
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

	// WebAuthn credentials
	let credentials = $state<AuthCredential[]>([]);
	let credName = $state('');
	let registering = $state(false);
	let credError = $state('');

	function toggleAutoMood() {
		appStore.setAutoMood(!appStore.autoMoodEnabled);
	}

	function base64urlToBuffer(b64: string): ArrayBuffer {
		const base64 = b64.replace(/-/g, '+').replace(/_/g, '/');
		const padding = '='.repeat((4 - (base64.length % 4)) % 4);
		const binary = atob(base64 + padding);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
		return bytes.buffer;
	}

	function bufferToBase64url(buffer: ArrayBuffer): string {
		const bytes = new Uint8Array(buffer);
		let binary = '';
		for (const b of bytes) binary += String.fromCharCode(b);
		return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
	}

	async function registerCredential() {
		registering = true;
		credError = '';
		try {
			const options = await authRegisterStart();
			const pk = options.publicKey;
			const credential = await navigator.credentials.create({
				publicKey: {
					...pk,
					challenge: base64urlToBuffer(pk.challenge),
					user: { ...pk.user, id: base64urlToBuffer(pk.user.id) },
					excludeCredentials: pk.excludeCredentials?.map((c: { id: string; type: string }) => ({
						...c, id: base64urlToBuffer(c.id),
					})),
				},
			});
			if (!credential) { credError = 'Registration cancelled'; return; }
			const attestation = credential as PublicKeyCredential;
			const response = attestation.response as AuthenticatorAttestationResponse;
			await authRegisterComplete({
				name: credName || 'Credential',
				credential: {
					id: attestation.id,
					rawId: bufferToBase64url(attestation.rawId),
					type: attestation.type,
					response: {
						attestationObject: bufferToBase64url(response.attestationObject),
						clientDataJSON: bufferToBase64url(response.clientDataJSON),
					},
				},
			});
			appStore.addNotification('Biometric credential registered', 'success');
			credName = '';
			const res = await listAuthCredentials();
			credentials = res.credentials ?? [];
		} catch (e: unknown) {
			credError = e instanceof Error ? e.message : 'Registration failed';
		} finally {
			registering = false;
		}
	}

	async function removeCred(id: string) {
		try {
			await deleteAuthCredential(id);
			credentials = credentials.filter(c => c.id !== id);
			appStore.addNotification('Credential removed', 'success');
		} catch {
			appStore.addNotification('Failed to remove credential', 'error');
		}
	}

	async function handleLogout() {
		try { await authLogout(); } catch {}
		localStorage.removeItem('cairn_api_token');
		window.location.reload();
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
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Control how and when you get notified</p>

		<div class="space-y-3">
			<!-- Toast duration -->
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

			<!-- Priority threshold -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3 mb-3">
					<Bell class="h-4 w-4 text-[var(--cairn-accent)]" />
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Priority Threshold</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Minimum priority to trigger a notification</p>
					</div>
				</div>
				<div class="flex gap-2">
					{#each PRIORITIES as p}
						<button
							class="flex-1 rounded-lg border px-3 py-2 text-left transition-all duration-[var(--dur-fast)]
								{editNotifPriority === p.value
								? 'border-[var(--cairn-accent)] bg-[var(--accent-dim)] shadow-sm'
								: 'border-border-subtle hover:bg-[var(--bg-2)]'}"
							onclick={() => editNotifPriority = p.value}
						>
							<span class="text-xs font-medium {editNotifPriority === p.value ? 'text-[var(--cairn-accent)]' : 'text-[var(--text-primary)]'}">{p.label}</span>
							<span class="block text-[10px] text-[var(--text-tertiary)]">{p.desc}</span>
						</button>
					{/each}
				</div>
			</div>

			<!-- Preferred channel -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3 mb-3">
					<Route class="h-4 w-4 text-[var(--cairn-accent)]" />
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Default Channel</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Where notifications are sent by default</p>
					</div>
				</div>
				<div class="flex gap-2">
					{#each CHANNELS as ch}
						<button
							class="rounded-lg border px-3 py-1.5 text-xs font-medium transition-all duration-[var(--dur-fast)]
								{editPreferredChannel === ch
								? 'border-[var(--cairn-accent)] bg-[var(--accent-dim)] text-[var(--cairn-accent)]'
								: 'border-border-subtle text-[var(--text-secondary)] hover:bg-[var(--bg-2)]'}"
							onclick={() => editPreferredChannel = ch}
						>
							{ch}
						</button>
					{/each}
				</div>
			</div>

			<!-- Quiet hours -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3 mb-3">
					<Clock class="h-4 w-4 text-[var(--cairn-accent)]" />
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Quiet Hours</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Suppress notifications during these hours (-1 = disabled)</p>
					</div>
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Start (hour)</p>
						<Input type="number" bind:value={editQuietStart} min={-1} max={23} class="h-7 text-xs font-mono" />
					</div>
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">End (hour)</p>
						<Input type="number" bind:value={editQuietEnd} min={-1} max={23} class="h-7 text-xs font-mono" />
					</div>
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Timezone</p>
						<Input type="text" bind:value={editQuietTZ} placeholder="UTC" class="h-7 text-xs font-mono" />
					</div>
				</div>
			</div>

			<!-- Muted sources -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3 mb-3">
					<BellOff class="h-4 w-4 text-[var(--text-tertiary)]" />
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Muted Sources</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">These sources won't trigger notifications (still in feed)</p>
					</div>
				</div>
				<div class="flex flex-wrap gap-2">
					{#each ALL_SOURCES as src}
						{@const muted = editMutedSources.split(',').map(s => s.trim()).filter(Boolean).includes(src)}
						<button
							class="rounded-full px-2.5 py-0.5 text-[11px] font-medium transition-colors
								{muted
								? 'bg-[var(--color-error)]/10 text-[var(--color-error)] border border-[var(--color-error)]/20'
								: 'bg-[var(--bg-2)] text-[var(--text-secondary)] hover:bg-[var(--bg-3)] border border-transparent'}"
							onclick={() => {
								const current = editMutedSources.split(',').map(s => s.trim()).filter(Boolean);
								if (muted) {
									editMutedSources = current.filter(s => s !== src).join(',');
								} else {
									editMutedSources = [...current, src].join(',');
								}
							}}
						>
							{src}
						</button>
					{/each}
				</div>
			</div>

			<!-- Channel routing per source -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-3 mb-3">
					<Route class="h-4 w-4 text-[var(--text-tertiary)]" />
					<div>
						<p class="text-sm font-medium text-[var(--text-primary)]">Channel Routing</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Override default channel per source (JSON)</p>
					</div>
				</div>
				<textarea
					bind:value={editChannelRouting}
					placeholder={`{"github_signal":"telegram","gmail":"slack"}`}
					class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs font-mono text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/40 focus:border-[var(--cairn-accent)] focus:outline-none resize-none h-16"
				></textarea>
			</div>

			<div class="flex justify-end">
				<Button
					size="sm" class="h-7 text-xs gap-1 px-3"
					onclick={() => saveConfig('notifications', {
						preferredChannel: editPreferredChannel,
						quietHoursStart: editQuietStart,
						quietHoursEnd: editQuietEnd,
						quietHoursTZ: editQuietTZ,
						mutedSources: editMutedSources,
						notifMinPriority: editNotifPriority,
						channelRouting: editChannelRouting,
					})}
					disabled={saving === 'notifications'}
				>
					{#if saving === 'notifications'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
					Save
				</Button>
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

	<!-- Scheduled Jobs -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Scheduled Jobs</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Cron jobs that cairn runs automatically on schedule</p>
		<CronManager />
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

	<Separator class="mb-8" />

	<!-- Gmail & Calendar -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Gmail & Calendar</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Email monitoring and calendar awareness via Google Workspace</p>

		<div class="space-y-3">
			<!-- Gmail -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md {editGmailEnabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
						<Mail class="h-4 w-4 {editGmailEnabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">Gmail</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">GitHub notification emails auto-archived (cairn knows, feed stays clean)</p>
					</div>
					<label class="relative inline-flex items-center cursor-pointer">
						<input type="checkbox" bind:checked={editGmailEnabled} class="sr-only peer" />
						<div class="w-9 h-5 bg-[var(--bg-3)] peer-checked:bg-[var(--cairn-accent)] rounded-full transition-colors after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
					</label>
				</div>

				{#if editGmailEnabled}
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Filter Query</p>
						<Input type="text" bind:value={editGmailFilter} class="h-7 text-xs font-mono" />
						<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">Gmail search syntax to exclude unwanted emails</p>
					</div>
				{/if}
			</div>

			<!-- Calendar -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md {editCalendarEnabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
						<Calendar class="h-4 w-4 {editCalendarEnabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">Calendar</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Upcoming events and invitations in your feed</p>
					</div>
					<label class="relative inline-flex items-center cursor-pointer">
						<input type="checkbox" bind:checked={editCalendarEnabled} class="sr-only peer" />
						<div class="w-9 h-5 bg-[var(--bg-3)] peer-checked:bg-[var(--cairn-accent)] rounded-full transition-colors after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
					</label>
				</div>

				{#if editCalendarEnabled}
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Lookahead (hours)</p>
						<Input type="number" bind:value={editCalendarLookahead} min={1} max={168} class="h-7 w-24 text-xs font-mono" />
						<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">How far ahead to show events (default 48h)</p>
					</div>
				{/if}
			</div>

			<div class="flex justify-end">
				<Button
					size="sm" class="h-7 text-xs gap-1 px-3"
					onclick={() => saveConfig('gws-pollers', {
						gmailEnabled: editGmailEnabled,
						calendarEnabled: editCalendarEnabled,
						gmailFilterQuery: editGmailFilter,
						calendarLookaheadH: editCalendarLookahead,
					})}
					disabled={saving === 'gws-pollers'}
				>
					{#if saving === 'gws-pollers'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
					Save
				</Button>
			</div>
		</div>
	</section>

	<Separator class="mb-8" />

	<!-- RSS, Stack Overflow, Dev.to -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Content Sources</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">RSS feeds, Stack Overflow questions, Dev.to articles</p>

		<div class="space-y-3">
			<!-- RSS -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md {editRssEnabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
						<Rss class="h-4 w-4 {editRssEnabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">RSS / Atom Feeds</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Blogs, changelogs, release notes</p>
					</div>
					<label class="relative inline-flex items-center cursor-pointer">
						<input type="checkbox" bind:checked={editRssEnabled} class="sr-only peer" />
						<div class="w-9 h-5 bg-[var(--bg-3)] peer-checked:bg-[var(--cairn-accent)] rounded-full transition-colors after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
					</label>
				</div>
				{#if editRssEnabled}
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Feed URLs</p>
						<textarea
							bind:value={editRssFeeds}
							placeholder="https://go.dev/blog/feed.atom, https://github.com/org/repo/releases.atom"
							class="w-full rounded-md border border-border-subtle bg-[var(--bg-0)] px-2.5 py-1.5 text-xs font-mono text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)]/40 focus:border-[var(--cairn-accent)] focus:outline-none resize-none h-16"
						></textarea>
					</div>
				{/if}
			</div>

			<!-- Stack Overflow -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md {editSoEnabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
						<Code class="h-4 w-4 {editSoEnabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">Stack Overflow</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Questions by tag (e.g. go, svelte, sqlite)</p>
					</div>
					<label class="relative inline-flex items-center cursor-pointer">
						<input type="checkbox" bind:checked={editSoEnabled} class="sr-only peer" />
						<div class="w-9 h-5 bg-[var(--bg-3)] peer-checked:bg-[var(--cairn-accent)] rounded-full transition-colors after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
					</label>
				</div>
				{#if editSoEnabled}
					<div>
						<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Tags</p>
						<Input type="text" bind:value={editSoTags} placeholder="go, svelte, sqlite" class="h-7 text-xs font-mono" />
					</div>
				{/if}
			</div>

			<!-- Dev.to -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md {editDevtoEnabled ? 'bg-[var(--color-success)]/10' : 'bg-[var(--bg-2)]'}">
						<BookOpen class="h-4 w-4 {editDevtoEnabled ? 'text-[var(--color-success)]' : 'text-[var(--text-tertiary)]'}" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">Dev.to</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Articles by tag or your own profile</p>
					</div>
					<label class="relative inline-flex items-center cursor-pointer">
						<input type="checkbox" bind:checked={editDevtoEnabled} class="sr-only peer" />
						<div class="w-9 h-5 bg-[var(--bg-3)] peer-checked:bg-[var(--cairn-accent)] rounded-full transition-colors after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
					</label>
				</div>
				{#if editDevtoEnabled}
					<div class="grid grid-cols-2 gap-3">
						<div>
							<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Tags</p>
							<Input type="text" bind:value={editDevtoTags} placeholder="go, webdev" class="h-7 text-xs font-mono" />
						</div>
						<div>
							<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Username</p>
							<Input type="text" bind:value={editDevtoUsername} placeholder="avifenesh" class="h-7 text-xs font-mono" />
						</div>
					</div>
				{/if}
			</div>

			<!-- NPM Packages -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--bg-2)]">
						<Package class="h-4 w-4 text-[var(--text-tertiary)]" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">npm Packages</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Track download metrics for your npm packages</p>
					</div>
				</div>
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Packages</p>
					<Input type="text" bind:value={editNpmPackages} placeholder="@anthropic-ai/sdk, svelte" class="h-7 text-xs font-mono" />
					<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">Comma-separated. Tracks weekly + total downloads over time.</p>
				</div>
			</div>

			<!-- Crates.io -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4 space-y-3">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--bg-2)]">
						<Package class="h-4 w-4 text-[var(--text-tertiary)]" />
					</div>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--text-primary)]">Crates.io</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Track download metrics for your Rust crates</p>
					</div>
				</div>
				<div>
					<p class="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Crates</p>
					<Input type="text" bind:value={editCratesPackages} placeholder="tokio, serde" class="h-7 text-xs font-mono" />
					<p class="text-[10px] text-[var(--text-tertiary)]/60 mt-0.5">Comma-separated. Tracks recent + total downloads over time.</p>
				</div>
			</div>

			<div class="flex justify-end">
				<Button
					size="sm" class="h-7 text-xs gap-1 px-3"
					onclick={() => saveConfig('content-sources', {
						rssEnabled: editRssEnabled,
						rssFeeds: editRssFeeds,
						soEnabled: editSoEnabled,
						soTags: editSoTags,
						devtoEnabled: editDevtoEnabled,
						devtoTags: editDevtoTags,
						devtoUsername: editDevtoUsername,
						npmPackages: editNpmPackages,
						cratesPackages: editCratesPackages,
					})}
					disabled={saving === 'content-sources'}
				>
					{#if saving === 'content-sources'}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Save class="h-3 w-3" />{/if}
					Save
				</Button>
			</div>
		</div>
	</section>

	<!-- Security / Auth -->
	<section class="mb-8">
		<h2 class="mb-1 text-sm font-medium text-[var(--text-primary)]">Security</h2>
		<p class="mb-4 text-xs text-[var(--text-tertiary)]">Biometric login and session management</p>

		<div class="space-y-4">
			<!-- Register credential -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center gap-2 mb-3">
					<Fingerprint class="h-4 w-4 text-[var(--cairn-accent)]" />
					<span class="text-xs font-medium text-[var(--text-secondary)] uppercase tracking-wider">Register biometric</span>
				</div>
				<p class="text-[10px] text-[var(--text-tertiary)] mb-3">Add a fingerprint, face, or security key for passwordless login</p>
				<div class="flex gap-2">
					<Input
						type="text"
						placeholder="Credential name (e.g. MacBook fingerprint)"
						bind:value={credName}
						class="h-8 text-xs flex-1"
					/>
					<Button
						size="sm" class="h-8 text-xs gap-1 px-3"
						onclick={registerCredential}
						disabled={registering}
					>
						{#if registering}<Loader2 class="h-3 w-3 animate-spin" />{:else}<Fingerprint class="h-3 w-3" />{/if}
						Register
					</Button>
				</div>
				{#if credError}
					<p class="mt-2 text-xs text-[var(--color-error)]">{credError}</p>
				{/if}
			</div>

			<!-- Existing credentials -->
			{#if credentials.length > 0}
				<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
					<div class="flex items-center gap-2 mb-3">
						<Shield class="h-4 w-4 text-[var(--cairn-accent)]" />
						<span class="text-xs font-medium text-[var(--text-secondary)] uppercase tracking-wider">Registered credentials</span>
						<span class="text-[10px] text-[var(--text-tertiary)] ml-auto">{credentials.length}</span>
					</div>
					<div class="space-y-2">
						{#each credentials as cred (cred.id)}
							<div class="flex items-center gap-3 rounded-md bg-[var(--bg-0)] px-3 py-2">
								<Fingerprint class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
								<div class="flex-1 min-w-0">
									<p class="text-xs text-[var(--text-primary)] truncate">{cred.name || 'Unnamed'}</p>
									<p class="text-[10px] text-[var(--text-tertiary)]">
										Added {new Date(cred.createdAt).toLocaleDateString()}
										{#if cred.lastUsedAt} · Last used {new Date(cred.lastUsedAt).toLocaleDateString()}{/if}
									</p>
								</div>
								<button
									class="p-1 rounded hover:bg-[var(--bg-2)] text-[var(--text-tertiary)] hover:text-[var(--color-error)] transition-colors"
									onclick={() => removeCred(cred.id)}
									title="Remove credential"
								>
									<Trash2 class="h-3.5 w-3.5" />
								</button>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Logout -->
			<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-xs font-medium text-[var(--text-secondary)]">Session</p>
						<p class="text-[10px] text-[var(--text-tertiary)]">Clear API token and session cookie</p>
					</div>
					<Button variant="outline" size="sm" class="h-7 text-xs" onclick={handleLogout}>
						Logout
					</Button>
				</div>
			</div>
		</div>
	</section>
</div>
