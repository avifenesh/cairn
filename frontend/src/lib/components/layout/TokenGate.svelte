<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { KeyRound, Fingerprint } from '@lucide/svelte';
	import { authLoginStart, authLoginComplete, authSession } from '$lib/api/client';
	import { base64urlToBuffer, bufferToBase64url } from '$lib/utils/webauthn';
	import { onMount } from 'svelte';

	let token = $state('');
	let error = $state('');
	let checking = $state(false);
	let biometricAvailable = $state(false);
	let biometricLoading = $state(false);

	onMount(async () => {
		// Check if already authenticated via session cookie.
		try {
			const sess = await authSession();
			if (sess.authenticated) {
				localStorage.setItem('cairn_api_token', '__session__');
				window.location.reload();
				return;
			}
		} catch {}

		// Check if WebAuthn + credentials are available.
		if (window.PublicKeyCredential) {
			try {
				await authLoginStart();
				biometricAvailable = true;
			} catch {
				// No credentials registered or server doesn't support WebAuthn.
			}
		}
	});

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
				window.location.reload();
				return;
			} else {
				error = 'Invalid token';
			}
		} catch (e) {
			console.warn('[TokenGate] validation failed:', e);
			error = 'Cannot reach server';
		} finally {
			checking = false;
		}
	}

	async function handleBiometric() {
		biometricLoading = true;
		error = '';
		try {
			const options = await authLoginStart();
			const credential = await navigator.credentials.get({
				publicKey: {
					...options.publicKey,
					challenge: base64urlToBuffer(options.publicKey.challenge),
					allowCredentials: options.publicKey.allowCredentials?.map((c: { id: string; type: string; transports?: string[] }) => ({
						...c,
						id: base64urlToBuffer(c.id),
					})),
				},
			});
			if (!credential) {
				error = 'No credential selected';
				return;
			}
			const assertionResponse = credential as PublicKeyCredential;
			const response = assertionResponse.response as AuthenticatorAssertionResponse;
			await authLoginComplete({
				id: assertionResponse.id,
				rawId: bufferToBase64url(assertionResponse.rawId),
				type: assertionResponse.type,
				response: {
					authenticatorData: bufferToBase64url(response.authenticatorData),
					clientDataJSON: bufferToBase64url(response.clientDataJSON),
					signature: bufferToBase64url(response.signature),
					userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : null,
				},
			});
			localStorage.setItem('cairn_api_token', '__session__');
			window.location.reload();
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Biometric login failed';
			if (msg.includes('API')) error = msg;
			else error = 'Biometric login failed';
			console.warn('[TokenGate] biometric failed:', e);
		} finally {
			biometricLoading = false;
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
			{#if biometricAvailable}
				<Button
					class="w-full h-12 gap-2 text-base"
					onclick={handleBiometric}
					disabled={biometricLoading}
				>
					<Fingerprint class="h-5 w-5" />
					{biometricLoading ? 'Verifying...' : 'Login with biometric'}
				</Button>
				<div class="relative">
					<div class="absolute inset-0 flex items-center"><span class="w-full border-t border-border-subtle"></span></div>
					<div class="relative flex justify-center text-[10px] uppercase"><span class="bg-[var(--bg-0)] px-2 text-[var(--text-tertiary)]">or use token</span></div>
				</div>
			{/if}
			<Input
				type="password"
				placeholder="API token"
				bind:value={token}
				onkeydown={handleKeydown}
				class="h-10 bg-[var(--bg-1)] text-center font-mono text-sm"
				autofocus={!biometricAvailable}
			/>
			{#if error}
				<p class="text-center text-xs text-[var(--color-error)]">{error}</p>
			{/if}
			<Button
				variant={biometricAvailable ? 'outline' : 'default'}
				class="w-full h-10"
				onclick={handleSubmit}
				disabled={!token.trim() || checking}
			>
				{checking ? 'Checking...' : 'Continue with token'}
			</Button>
		</div>

		<p class="mt-6 text-center text-[10px] text-[var(--text-tertiary)]">
			{biometricAvailable ? 'Biometric registered — use fingerprint or face to login' : 'Set via WRITE_API_TOKEN environment variable'}
		</p>
	</div>
</div>
