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
	// Use embedded FS if available, else filesystem.
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

		// Try to open the file in the embedded FS.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		f, err := dist.Open(path)
		if err == nil {
			f.Close()
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

		// Path traversal protection.
		absFilePath, _ := filepath.Abs(filePath)
		absDistDir, _ := filepath.Abs(distDir)
		if !strings.HasPrefix(absFilePath, absDistDir+string(filepath.Separator)) && absFilePath != absDistDir {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		info, err := os.Stat(filePath)
		if err != nil || info.IsDir() {
			indexPath := filepath.Join(distDir, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		http.ServeFile(w, r, filePath)
	})
}
