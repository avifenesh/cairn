<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { KeyRound } from '@lucide/svelte';

	let { onauth }: { onauth: () => void } = $props();

	let token = $state('');
	let error = $state('');
	let checking = $state(false);

	async function handleSubmit() {
		const t = token.trim();
		if (!t) return;
		checking = true;
		error = '';
		try {
			const res = await fetch('/v1/dashboard', {
				headers: { 'X-Api-Token': t },
			});
			if (res.ok) {
				localStorage.setItem('cairn_api_token', t);
				onauth();
			} else {
				error = 'Invalid token';
			}
		} catch {
			error = 'Cannot reach server';
		} finally {
			checking = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') handleSubmit();
	}
</script>

<div class="flex h-dvh items-center justify-center bg-[var(--bg-0)]">
	<div class="w-full max-w-sm px-6">
		<div class="mb-8 text-center">
			<div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--accent-dim)]">
				<KeyRound class="h-7 w-7 text-[var(--cairn-accent)]" />
			</div>
			<h1 class="text-xl font-semibold tracking-tight text-[var(--text-primary)]">Cairn</h1>
			<p class="mt-1 text-sm text-[var(--text-tertiary)]">Enter your API token to continue</p>
		</div>

		<div class="space-y-3">
			<Input
				type="password"
				placeholder="API token"
				bind:value={token}
				onkeydown={handleKeydown}
				class="h-10 bg-[var(--bg-1)] text-center font-mono text-sm"
				autofocus
			/>
			{#if error}
				<p class="text-center text-xs text-[var(--color-error)]">{error}</p>
			{/if}
			<Button
				class="w-full h-10"
				onclick={handleSubmit}
				disabled={!token.trim() || checking}
			>
				{checking ? 'Checking...' : 'Continue'}
			</Button>
		</div>

		<p class="mt-6 text-center text-[10px] text-[var(--text-tertiary)]">
			Set via WRITE_API_TOKEN environment variable
		</p>
	</div>
</div>
