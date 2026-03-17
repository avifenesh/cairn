// TTS playback - Plan: fetch /v1/assistant/voice/tts?text=... -> play Audio element

let currentAudio: HTMLAudioElement | null = null;

export async function playTTS(text: string, format: string = 'mp3'): Promise<void> {
	if (currentAudio) {
		currentAudio.pause();
		currentAudio = null;
	}
	const token = localStorage.getItem('pub_api_token');
	const params = new URLSearchParams({ text, format });
	if (token) params.set('token', token);
	const url = `/v1/assistant/voice/tts?${params}`;
	const audio = new Audio(url);
	currentAudio = audio;
	return new Promise((resolve, reject) => {
		audio.onended = () => { currentAudio = null; resolve(); };
		audio.onerror = () => { currentAudio = null; reject(new Error('TTS playback failed')); };
		audio.play().catch(reject);
	});
}

export function stopTTS(): void {
	if (currentAudio) {
		currentAudio.pause();
		currentAudio = null;
	}
}
