package server

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/avifenesh/cairn/frontend"
)

// staticHandler serves files from either the embedded frontend/dist (if built
// with -tags embed_frontend) or the filesystem frontend/dist/ directory.
// SPA fallback: any path that doesn't match a real file returns index.html.
func (s *Server) staticHandler() http.Handler {
	if frontend.Dist != nil {
		return s.embeddedStaticHandler(frontend.Dist)
	}
	return s.fsStaticHandler("frontend/dist")
}

// embeddedStaticHandler serves from an embed.FS with SPA fallback.
func (s *Server) embeddedStaticHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Use fs.Stat to check existence and whether it's a file (not dir).
		info, err := fs.Stat(dist, path)
		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for any non-file path.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// fsStaticHandler serves from the filesystem with SPA fallback (dev mode).
func (s *Server) fsStaticHandler(distDir string) http.Handler {
	// Resolve distDir to absolute once at init time (not per-request).
	absDistDir, err := filepath.Abs(distDir)
	if err != nil {
		// If we can't resolve the dist dir, return a handler that always 404s.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "static files not available")
		})
	}
	indexPath := filepath.Join(absDistDir, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		// Resolve the safe path within the dist directory.
		safePath, ok := safeFSPath(absDistDir, r.URL.Path)
		if !ok {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		info, statErr := os.Stat(safePath)
		if statErr != nil || info.IsDir() {
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		http.ServeFile(w, r, safePath)
	})
}

// safeFSPath resolves a URL path to a safe filesystem path within baseDir.
// Returns the absolute path and true if safe, or empty string and false if
// the path would escape baseDir.
func safeFSPath(absBaseDir, urlPath string) (string, bool) {
	// Clean the URL path and strip leading slash.
	cleaned := filepath.Clean(urlPath)
	if cleaned == "/" || cleaned == "." {
		cleaned = "index.html"
	} else {
		cleaned = strings.TrimPrefix(cleaned, "/")
	}

	// Join with the pre-resolved absolute base dir.
	candidate := filepath.Join(absBaseDir, cleaned)

	// Verify the candidate is within baseDir.
	if !strings.HasPrefix(candidate, absBaseDir+string(filepath.Separator)) && candidate != absBaseDir {
		return "", false
	}
	return candidate, true
}
