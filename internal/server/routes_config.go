package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/avifenesh/cairn/internal/config"
)

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeJSON(w, http.StatusOK, config.PatchableConfig{})
		return
	}
	writeJSON(w, http.StatusOK, s.config.GetPatchable())
}

func (s *Server) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeError(w, http.StatusServiceUnavailable, "config not available")
		return
	}

	var patch config.PatchableConfig
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	s.config.ApplyPatch(patch)

	if err := s.config.SaveOverrides(s.config.DataDir); err != nil {
		s.logger.Warn("failed to save config overrides", "error", err)
	}

	if s.OnConfigPatch != nil {
		s.OnConfigPatch()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": s.config.GetPatchable()})
}

// handleUpload accepts a multipart file upload (images/videos for vision tools).
// Files are saved to {DataDir}/uploads/ with a random filename.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 32 << 20 // 32MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file too large (max 32MB)"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing file field"})
		return
	}
	defer file.Close()

	// Validate MIME type.
	ct := header.Header.Get("Content-Type")
	allowed := map[string]bool{
		"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
		"video/mp4": true, "video/quicktime": true, "video/x-m4v": true,
	}
	if !allowed[ct] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("unsupported file type: %s", ct)})
		return
	}

	// Generate random filename.
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to generate filename"})
		return
	}
	exts, _ := mime.ExtensionsByType(ct)
	ext := ".bin"
	if len(exts) > 0 {
		ext = exts[0]
	}
	// Prefer common extensions.
	switch ct {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "video/mp4":
		ext = ".mp4"
	case "video/quicktime":
		ext = ".mov"
	}
	filename := hex.EncodeToString(buf[:]) + ext

	// Save to uploads directory.
	dataDir := "./data"
	if s.config != nil && s.config.DataDir != "" {
		dataDir = s.config.DataDir
	}
	uploadDir := filepath.Join(dataDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create upload directory"})
		return
	}

	destPath := filepath.Join(uploadDir, filename)
	dest, err := os.Create(destPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save file"})
		return
	}
	defer dest.Close()

	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to write file"})
		return
	}

	absPath, _ := filepath.Abs(destPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     absPath,
		"name":     header.Filename,
		"size":     written,
		"mimeType": ct,
	})
}
