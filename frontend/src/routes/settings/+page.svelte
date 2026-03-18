<script lang="ts">
	import { appStore, type Theme, type Density, type Mood } from '$lib/stores/app.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { Sun, Moon, Monitor, Circle, Wifi, WifiOff } from '@lucide/svelte';

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
</div>
