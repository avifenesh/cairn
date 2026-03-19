<script lang="ts">
	import { uploadFile } from '$lib/api/client';
	import type { Attachment } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Paperclip, Loader2 } from '@lucide/svelte';

	let { onattach, disabled = false }: { onattach: (a: Attachment) => void; disabled?: boolean } =
		$props();

	let uploading = $state(false);
	let inputEl: HTMLInputElement | undefined = $state();

	function openPicker() {
		inputEl?.click();
	}

	async function handleFileChange(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		input.value = '';

		uploading = true;
		try {
			const result = await uploadFile(file);
			onattach({ path: result.path, name: result.name, size: result.size, mimeType: result.mimeType });
		} catch {
			// upload failed silently
		} finally {
			uploading = false;
		}
	}
</script>

<input
	bind:this={inputEl}
	type="file"
	accept="image/*,video/mp4,video/quicktime,video/x-m4v"
	class="hidden"
	onchange={handleFileChange}
/>
<Button
	size="icon"
	variant="ghost"
	class="h-10 w-10 rounded-lg text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
	onclick={openPicker}
	disabled={disabled || uploading}
	title="Attach file"
>
	{#if uploading}
		<Loader2 class="h-4 w-4 animate-spin" />
	{:else}
		<Paperclip class="h-4 w-4" />
	{/if}
</Button>
