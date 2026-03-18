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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		reqPath := filepath.Clean(r.URL.Path)
		if reqPath == "/" || reqPath == "." {
			reqPath = "index.html"
		} else {
			reqPath = strings.TrimPrefix(reqPath, "/")
		}

		filePath := filepath.Join(distDir, reqPath)

		// Path traversal protection: resolved path must be within distDir.
		absFilePath, err := filepath.Abs(filePath)
		if err != nil {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		absDistDir, err := filepath.Abs(distDir)
		if err != nil {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if !strings.HasPrefix(absFilePath, absDistDir+string(filepath.Separator)) && absFilePath != absDistDir {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		// Use the validated absolute path for all file operations.
		info, statErr := os.Stat(absFilePath)
		if statErr != nil || info.IsDir() {
			absIndexPath := filepath.Join(absDistDir, "index.html")
			if _, err := os.Stat(absIndexPath); err == nil {
				http.ServeFile(w, r, absIndexPath)
				return
			}
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		http.ServeFile(w, r, absFilePath)
	})
}
