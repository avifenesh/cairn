package server

import (
	"io/fs"
	"net/http"
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

		info, err := fs.Stat(dist, path)
		if err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// fsStaticHandler serves from the filesystem with SPA fallback (dev mode).
// Uses http.Dir which provides built-in path sanitization (rooted at distDir).
func (s *Server) fsStaticHandler(distDir string) http.Handler {
	root := http.Dir(distDir)
	fileServer := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		// Try to open the file via http.Dir (handles path traversal protection).
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		f, err := root.Open(path)
		if err != nil {
			// SPA fallback: serve index.html for any missing path.
			r.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil || info.IsDir() {
			// Directory or stat error: SPA fallback.
			r.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r)
			return
		}

		// Serve the actual file.
		fileServer.ServeHTTP(w, r)
	})
}
