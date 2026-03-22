package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// handleVoiceTranscribe accepts audio (multipart file) and returns transcribed text.
func (s *Server) handleVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB hard limit
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form or file too large"})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'audio' file field"})
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "read audio failed"})
		return
	}

	text, err := s.voice.Transcribe(r.Context(), audioData, header.Filename)
	if err != nil {
		s.logger.Error("voice transcribe failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "transcription failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"text": text,
	})
}

// handleVoiceTTS converts text to speech and returns MP3 audio.
func (s *Server) handleVoiceTTS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	if body.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text is required"})
		return
	}

	audio, err := s.voice.Synthesize(r.Context(), body.Text, body.Voice)
	if err != nil {
		s.logger.Error("voice TTS failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "speech synthesis failed"})
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.WriteHeader(http.StatusOK)
	w.Write(audio)
}
