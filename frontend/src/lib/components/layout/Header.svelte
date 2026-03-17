<script lang="ts">
	import { appStore } from '$lib/stores/app.svelte';
	import { Circle, Search, Sun, Moon, Menu } from '@lucide/svelte';

	function handleKeyboardShortcut() {
		appStore.openCommandPalette();
	}
</script>

<header
	class="flex h-[var(--header-h)] items-center justify-between border-b border-border-subtle bg-[var(--bg-1)] px-4"
>
	<div class="flex items-center gap-3">
		<button
			class="md:hidden rounded p-1.5 hover:bg-[var(--bg-3)] transition-colors duration-[var(--dur-fast)]"
			onclick={() => appStore.toggleSidebar()}
			aria-label="Toggle menu"
		>
			<Menu class="h-5 w-5 text-[var(--text-secondary)]" />
		</button>
		<span class="text-sm font-semibold tracking-tight text-[var(--text-primary)]">Pub</span>
		<span class="flex items-center gap-1.5 text-xs">
			<Circle
				class="h-2 w-2 fill-current {appStore.sseConnected
					? 'text-[var(--color-success)]'
					: 'text-[var(--color-error)]'}"
			/>
			<span class="text-[var(--text-tertiary)] hidden sm:inline">
				{appStore.sseConnected ? 'Live' : 'Offline'}
			</span>
		</span>
	</div>

	<div class="flex items-center gap-2">
		<button
			class="flex items-center gap-2 rounded-md border border-border-subtle bg-[var(--bg-2)] px-3 py-1.5 text-xs text-[var(--text-tertiary)] hover:border-border-default transition-colors duration-[var(--dur-fast)]"
			onclick={handleKeyboardShortcut}
		>
			<Search class="h-3.5 w-3.5" />
			<span class="hidden sm:inline">Search...</span>
			<kbd class="hidden sm:inline rounded border border-border-subtle bg-[var(--bg-3)] px-1 py-0.5 text-[10px]">
				⌘K
			</kbd>
		</button>

		<button
			class="rounded p-1.5 hover:bg-[var(--bg-3)] transition-colors duration-[var(--dur-fast)]"
			onclick={() => appStore.toggleTheme()}
			aria-label="Toggle theme"
		>
			{#if appStore.theme === 'dark'}
				<Sun class="h-4 w-4 text-[var(--text-secondary)]" />
			{:else}
				<Moon class="h-4 w-4 text-[var(--text-secondary)]" />
			{/if}
		</button>
	</div>
</header>
