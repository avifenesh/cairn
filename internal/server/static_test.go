package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestEmbeddedStaticHandler_ServesFile(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":   {Data: []byte("<html>home</html>")},
		"_app/main.js": {Data: []byte("console.log('hi')")},
		"chat.html":    {Data: []byte("<html>chat</html>")},
	}

	s := &Server{}
	handler := s.embeddedStaticHandler(dist)

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/", 200, "<html>home</html>"},
		{"/index.html", 301, ""}, // FileServer redirects to /
		{"/chat.html", 200, "<html>chat</html>"},
		{"/_app/main.js", 200, "console.log('hi')"},
		{"/nonexistent", 200, "<html>home</html>"}, // SPA fallback
		{"/v1/feed", 404, ""},                      // API path excluded
		{"/v1", 404, ""},                           // Bare /v1 excluded
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d", tt.path, rr.Code, tt.wantStatus)
		}
		if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
			t.Errorf("%s: body = %q, want %q", tt.path, rr.Body.String(), tt.wantBody)
		}
	}
}

func TestEmbeddedStaticHandler_MethodNotAllowed(t *testing.T) {
	dist := fstest.MapFS{"index.html": {Data: []byte("ok")}}
	s := &Server{}
	handler := s.embeddedStaticHandler(dist)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: status = %d, want 405", rr.Code)
	}
}

func TestEmbeddedStaticHandler_DirFallsBackToIndex(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":   {Data: []byte("<html>home</html>")},
		"_app/main.js": {Data: []byte("js")},
	}
	s := &Server{}
	handler := s.embeddedStaticHandler(dist)

	// Request a directory path — should SPA fallback.
	req := httptest.NewRequest(http.MethodGet, "/_app", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Errorf("/_app: status = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "<html>home</html>" {
		t.Errorf("/_app: body = %q, want index.html content", rr.Body.String())
	}
}

func TestIsAPIPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/v1", true},
		{"/v1/", true},
		{"/v1/feed", true},
		{"/v1/tasks/123", true},
		{"/", false},
		{"/chat", false},
		{"/v10", false},
		{"/v1extra", false},
	}

	for _, tt := range tests {
		if got := isAPIPath(tt.path); got != tt.want {
			t.Errorf("isAPIPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
