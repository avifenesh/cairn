// TTS playback via fetch + blob URL (avoids token leak in query params)

let currentAudio: HTMLAudioElement | null = null;
let currentObjectUrl: string | null = null;

export async function playTTS(text: string, format: string = 'mp3'): Promise<void> {
	stopTTS();
	const params = new URLSearchParams({ text, format });
	const h: HeadersInit = {};
	const token = localStorage.getItem('cairn_api_token');
	if (token) h['X-Api-Token'] = token;
	const res = await fetch(`/v1/assistant/voice/tts?${params}`, {
		credentials: 'include',
		headers: h,
	});
	if (!res.ok) throw new Error(`TTS fetch failed: ${res.status}`);
	const blob = await res.blob();
	const url = URL.createObjectURL(blob);
	currentObjectUrl = url;
	const audio = new Audio(url);
	currentAudio = audio;
	return new Promise((resolve, reject) => {
		audio.onended = () => { cleanup(); resolve(); };
		audio.onerror = () => { cleanup(); reject(new Error('TTS playback failed')); };
		audio.play().catch((err) => { cleanup(); reject(err); });
	});
}

function cleanup() {
	if (currentObjectUrl) {
		URL.revokeObjectURL(currentObjectUrl);
		currentObjectUrl = null;
	}
	currentAudio = null;
}

export function stopTTS(): void {
	if (currentAudio) {
		currentAudio.pause();
	}
	cleanup();
}
