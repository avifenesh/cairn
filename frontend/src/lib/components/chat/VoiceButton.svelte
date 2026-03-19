<script lang="ts">
	import { uploadVoice } from '$lib/api/client';
	import { chatStore } from '$lib/stores/chat.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Mic, MicOff, Loader2 } from '@lucide/svelte';

	let recording = $state(false);
	let processing = $state(false);
	let mediaRecorder: MediaRecorder | null = null;
	let chunks: Blob[] = [];

	async function toggleRecording() {
		if (recording) {
			stopRecording();
			return;
		}

		try {
			const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			mediaRecorder = new MediaRecorder(stream);
			chunks = [];

			mediaRecorder.ondataavailable = (e) => {
				if (e.data.size > 0) chunks.push(e.data);
			};

			mediaRecorder.onstop = async () => {
				stream.getTracks().forEach((t) => t.stop());
				const blob = new Blob(chunks, { type: 'audio/webm' });
				processing = true;
				try {
					const res = await uploadVoice(blob, chatStore.mode, chatStore.currentSessionId ?? undefined);
					if (res.sessionId && !chatStore.currentSessionId) {
						chatStore.setCurrentSession(res.sessionId);
					}
					chatStore.addUserMessage(res.transcript);
					chatStore.startStreaming(res.taskId);
				} catch {
					// handled
				} finally {
					processing = false;
				}
			};

			mediaRecorder.start();
			recording = true;
		} catch {
			// mic permission denied
		}
	}

	function stopRecording() {
		if (mediaRecorder && mediaRecorder.state === 'recording') {
			mediaRecorder.stop();
		}
		recording = false;
	}
</script>

<Button
	variant={recording ? 'destructive' : 'ghost'}
	size="icon"
	class="h-10 w-10 rounded-lg {recording ? '' : 'text-[var(--text-tertiary)]'}"
	onclick={toggleRecording}
	disabled={processing}
	aria-label={recording ? 'Stop recording' : 'Voice input'}
>
	{#if processing}
		<Loader2 class="h-4 w-4 animate-spin" />
	{:else if recording}
		<MicOff class="h-4 w-4" />
	{:else}
		<Mic class="h-4 w-4" />
	{/if}
</Button>
