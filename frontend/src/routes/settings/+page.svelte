<script lang="ts">
	import { appStore, type Theme, type Density, type Mood } from '$lib/stores/app.svelte';
	import { Sun, Moon } from '@lucide/svelte';

	const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
		{ value: 'dark', label: 'Dark', icon: Moon },
		{ value: 'light', label: 'Light', icon: Sun },
	];

	const densities: { value: Density; label: string }[] = [
		{ value: 'comfortable', label: 'Comfortable' },
		{ value: 'balanced', label: 'Balanced' },
		{ value: 'dense', label: 'Dense' },
	];

	const moods: { value: Mood; label: string; color: string }[] = [
		{ value: 'default', label: 'Default', color: '#E87B9A' },
		{ value: 'dawn', label: 'Dawn', color: '#F59E0B' },
		{ value: 'ocean', label: 'Ocean', color: '#06B6D4' },
		{ value: 'night', label: 'Night', color: '#818CF8' },
	];

	let toastDuration = $state(Number(localStorage.getItem('pub_toast_duration')) || 5);

	function toggleAutoMood() {
		appStore.setAutoMood(!appStore.autoMoodEnabled);
	}

	function setToastDuration(seconds: number) {
		toastDuration = seconds;
		localStorage.setItem('pub_toast_duration', String(seconds));
	}
</script>

<div class="mx-auto max-w-2xl p-6">
	<h1 class="mb-8 text-2xl font-semibold text-[var(--text-primary)]">Settings</h1>

	<!-- Theme -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-medium text-[var(--text-secondary)]">Theme</h2>
		<div class="flex gap-2">
			{#each themes as t}
				<button
					class="flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm transition-colors duration-[var(--dur-fast)]
						{appStore.theme === t.value
						? 'border-[var(--pub-accent)] bg-[var(--accent-dim)] text-[var(--pub-accent)]'
						: 'border-border-subtle bg-[var(--bg-1)] text-[var(--text-secondary)] hover:bg-[var(--bg-2)]'}"
					onclick={() => appStore.setTheme(t.value)}
				>
					<t.icon class="h-4 w-4" />
					{t.label}
				</button>
			{/each}
		</div>
	</section>

	<!-- Density -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-medium text-[var(--text-secondary)]">Density</h2>
		<div class="flex gap-2">
			{#each densities as d}
				<button
					class="rounded-lg border px-4 py-2.5 text-sm transition-colors duration-[var(--dur-fast)]
						{appStore.density === d.value
						? 'border-[var(--pub-accent)] bg-[var(--accent-dim)] text-[var(--pub-accent)]'
						: 'border-border-subtle bg-[var(--bg-1)] text-[var(--text-secondary)] hover:bg-[var(--bg-2)]'}"
					onclick={() => appStore.setDensity(d.value)}
				>
					{d.label}
				</button>
			{/each}
		</div>
	</section>

	<!-- Mood -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-medium text-[var(--text-secondary)]">Mood</h2>
		<div class="flex gap-3">
			{#each moods as m}
				<button
					class="flex flex-col items-center gap-2 rounded-lg border px-4 py-3 text-sm transition-colors duration-[var(--dur-fast)]
						{appStore.mood === m.value
						? 'border-[var(--pub-accent)] bg-[var(--accent-dim)]'
						: 'border-border-subtle bg-[var(--bg-1)] hover:bg-[var(--bg-2)]'}"
					onclick={() => appStore.setMood(m.value)}
				>
					<span class="h-6 w-6 rounded-full" style="background: {m.color}"></span>
					<span class="text-xs text-[var(--text-secondary)]">{m.label}</span>
				</button>
			{/each}
		</div>
		<label class="mt-3 flex items-center gap-2 text-xs text-[var(--text-secondary)]">
			<input
				type="checkbox"
				checked={appStore.autoMoodEnabled}
				onchange={toggleAutoMood}
				class="h-4 w-4 rounded accent-[var(--pub-accent)]"
			/>
			Auto-mood (changes by time of day)
		</label>
	</section>

	<!-- Notifications -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-medium text-[var(--text-secondary)]">Notifications</h2>
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<label class="flex items-center justify-between">
				<span class="text-sm text-[var(--text-primary)]">Toast duration</span>
				<div class="flex items-center gap-2">
					{#each [3, 5, 8] as sec}
						<button
							class="rounded-md px-2.5 py-1 text-xs transition-colors duration-[var(--dur-fast)]
								{toastDuration === sec
								? 'bg-[var(--accent-dim)] text-[var(--pub-accent)]'
								: 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}"
							onclick={() => setToastDuration(sec)}
						>
							{sec}s
						</button>
					{/each}
				</div>
			</label>
		</div>
	</section>

	<!-- Connection -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-medium text-[var(--text-secondary)]">Connection</h2>
		<div class="rounded-lg border border-border-subtle bg-[var(--bg-1)] p-4">
			<div class="flex items-center gap-2">
				<span
					class="h-2 w-2 rounded-full {appStore.sseConnected ? 'bg-[var(--color-success)]' : 'bg-[var(--color-error)]'}"
				></span>
				<span class="text-sm text-[var(--text-primary)]">
					SSE {appStore.sseConnected ? 'Connected' : 'Disconnected'}
				</span>
			</div>
		</div>
	</section>
</div>
