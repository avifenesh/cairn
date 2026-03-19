// Package voice provides speech-to-text (STT) and text-to-speech (TTS) services.
//
// STT: whisper.cpp HTTP server (local, free, large-v3-turbo model)
// TTS: edge-tts CLI (free Microsoft neural voices, no API key needed)
package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Config holds voice service configuration.
type Config struct {
	WhisperURL string // Whisper.cpp server URL (default: http://127.0.0.1:8178)
	TTSVoice   string // Edge-TTS voice name (default: en-US-BrianNeural)
	TTSEnabled bool   // Enable TTS (default: true)
	STTEnabled bool   // Enable STT (default: true)
	TempDir    string // Temp directory for audio files (default: os.TempDir())
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		WhisperURL: "http://127.0.0.1:8178",
		TTSVoice:   "en-US-BrianNeural",
		TTSEnabled: true,
		STTEnabled: true,
		TempDir:    os.TempDir(),
	}
}

// Service provides STT and TTS capabilities.
type Service struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger
}

// New creates a voice service.
func New(cfg Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
		logger: logger,
	}
}

// Transcribe converts audio data to text using whisper.cpp.
// Accepts raw audio bytes (wav, ogg, mp3, webm, m4a, flac).
// Returns the transcribed text.
func (s *Service) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if !s.cfg.STTEnabled {
		return "", fmt.Errorf("voice: STT is disabled")
	}
	if len(audio) == 0 {
		return "", fmt.Errorf("voice: empty audio data")
	}

	// Convert to wav if needed (whisper.cpp works best with wav).
	ext := strings.ToLower(filepath.Ext(filename))
	wavData := audio
	if ext != ".wav" {
		var err error
		wavData, err = s.convertToWav(ctx, audio, filename)
		if err != nil {
			return "", fmt.Errorf("voice: convert audio: %w", err)
		}
	}

	// Send to whisper.cpp HTTP server.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("voice: create form: %w", err)
	}
	if _, err := part.Write(wavData); err != nil {
		return "", fmt.Errorf("voice: write form: %w", err)
	}
	// Set response format to text (not JSON).
	writer.WriteField("response_format", "text")
	writer.Close()

	url := strings.TrimRight(s.cfg.WhisperURL, "/") + "/inference"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("voice: create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("voice: whisper request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("voice: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("voice: whisper HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	text := strings.TrimSpace(string(respBody))
	s.logger.Info("voice: transcribed", "chars", len(text), "audioBytes", len(audio))
	return text, nil
}

// Synthesize converts text to speech using edge-tts.
// If voiceOverride is non-empty, it overrides the configured voice.
// Returns MP3 audio bytes.
func (s *Service) Synthesize(ctx context.Context, text string, voiceOverride string) ([]byte, error) {
	if !s.cfg.TTSEnabled {
		return nil, fmt.Errorf("voice: TTS is disabled")
	}
	text = SanitizeForSpeech(text)
	if text == "" {
		return nil, fmt.Errorf("voice: empty text")
	}

	// Create temp file for output.
	outFile, err := os.CreateTemp(s.cfg.TempDir, "cairn-tts-*.mp3")
	if err != nil {
		return nil, fmt.Errorf("voice: create temp: %w", err)
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	// Determine voice.
	ttsVoice := s.cfg.TTSVoice
	if voiceOverride != "" {
		ttsVoice = voiceOverride
	}

	// Run edge-tts CLI.
	cmd := exec.CommandContext(ctx, "edge-tts",
		"--voice", ttsVoice,
		"--text", text,
		"--write-media", outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("voice: TTS synthesis failed: %w", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("voice: read output: %w", err)
	}

	s.logger.Info("voice: synthesized", "textLen", len(text), "audioBytes", len(data), "voice", s.cfg.TTSVoice)
	return data, nil
}

// convertToWav uses ffmpeg to convert audio to WAV format.
func (s *Service) convertToWav(ctx context.Context, audio []byte, filename string) ([]byte, error) {
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".ogg"
	}

	// Create temp input file.
	inFile, err := os.CreateTemp(s.cfg.TempDir, "cairn-stt-in-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	inPath := inFile.Name()
	defer os.Remove(inPath)
	if _, err := inFile.Write(audio); err != nil {
		inFile.Close()
		return nil, fmt.Errorf("write temp input: %w", err)
	}
	inFile.Close()

	// Create temp output file.
	outFile, err := os.CreateTemp(s.cfg.TempDir, "cairn-stt-out-*.wav")
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	// Convert with ffmpeg: mono, 16kHz, 16-bit PCM (whisper.cpp preferred format).
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", inPath,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		"-y",
		outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("voice: audio conversion failed: %w", err)
	}

	return os.ReadFile(outPath)
}

// Regex patterns for speech sanitization.
var (
	reURL          = regexp.MustCompile(`https?://\S+`)
	reMDLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reMDImage      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	reCodeBlock    = regexp.MustCompile("(?s)```[\\w]*\n.*?```")
	reInlineCode   = regexp.MustCompile("`[^`]+`")
	reMDHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reMDBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reMDItalic     = regexp.MustCompile(`\*([^*]+)\*`)
	reMDStrike     = regexp.MustCompile(`~~([^~]+)~~`)
	reBulletList   = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	reNumberedList = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	reHTMLTags     = regexp.MustCompile(`<[^>]+>`)
	reMultiNewline = regexp.MustCompile(`\n{3,}`)
	reMultiSpace   = regexp.MustCompile(`\s{2,}`)
)

// SanitizeForSpeech strips markdown formatting, URLs, code blocks, and other
// elements that sound bad when read aloud by TTS.
func SanitizeForSpeech(text string) string {
	// Remove code blocks entirely (they're unreadable as speech).
	text = reCodeBlock.ReplaceAllString(text, "")

	// Remove inline code backticks (keep the text inside).
	text = reInlineCode.ReplaceAllStringFunc(text, func(s string) string {
		return strings.Trim(s, "`")
	})

	// Replace markdown images with alt text or nothing.
	text = reMDImage.ReplaceAllString(text, "$1")

	// Replace markdown links with just the link text.
	text = reMDLink.ReplaceAllString(text, "$1")

	// Remove bare URLs.
	text = reURL.ReplaceAllString(text, "")

	// Remove HTML tags.
	text = reHTMLTags.ReplaceAllString(text, "")

	// Strip markdown formatting (keep the text).
	text = reMDHeading.ReplaceAllString(text, "")
	text = reMDBold.ReplaceAllString(text, "$1")
	text = reMDItalic.ReplaceAllString(text, "$1")
	text = reMDStrike.ReplaceAllString(text, "$1")

	// Clean up list markers.
	text = reBulletList.ReplaceAllString(text, "")
	text = reNumberedList.ReplaceAllString(text, "")

	// Collapse whitespace.
	text = reMultiNewline.ReplaceAllString(text, "\n\n")
	text = reMultiSpace.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}
