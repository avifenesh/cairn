package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// staticHandler serves files from the frontend/dist/ directory with SPA
// fallback: any path that doesn't match a real file returns index.html.
func (s *Server) staticHandler() http.Handler {
	distDir := "frontend/dist"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only serve GET/HEAD for static files.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// Don't serve static for /v1/ API paths (should have been caught by mux).
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		// Clean the path and resolve against dist directory.
		reqPath := filepath.Clean(r.URL.Path)
		if reqPath == "/" {
			reqPath = "/index.html"
		}

		filePath := filepath.Join(distDir, reqPath)

		// Check if the file exists.
		info, err := os.Stat(filePath)
		if err != nil || info.IsDir() {
			// SPA fallback: serve index.html for any non-file path.
			indexPath := filepath.Join(distDir, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
			// If no index.html either, 404.
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		// Serve the actual file.
		http.ServeFile(w, r, filePath)
	})
}
