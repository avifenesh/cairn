package server

import (
	"io/fs"
	"net/http"
	"path"
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

func isAPIPath(p string) bool {
	return p == "/v1" || strings.HasPrefix(p, "/v1/")
}

// embeddedStaticHandler serves from an embed.FS with SPA fallback.
func (s *Server) embeddedStaticHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if isAPIPath(r.URL.Path) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		// Use path.Clean (not filepath.Clean) for URL paths — OS-agnostic.
		cleaned := path.Clean(r.URL.Path)
		p := strings.TrimPrefix(cleaned, "/")
		if p == "" {
			p = "index.html"
		}

		info, err := fs.Stat(dist, p)
		if err == nil && !info.IsDir() {
			// Serve with the cleaned path so FileServer resolves the same file.
			r.URL.Path = cleaned
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
		if isAPIPath(r.URL.Path) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		// Use path.Clean for URL paths.
		cleaned := path.Clean(r.URL.Path)
		if cleaned == "/" {
			cleaned = "/index.html"
		}

		f, err := root.Open(cleaned)
		if err != nil {
			r.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil || info.IsDir() {
			r.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r)
			return
		}

		r.URL.Path = cleaned
		fileServer.ServeHTTP(w, r)
	})
}
