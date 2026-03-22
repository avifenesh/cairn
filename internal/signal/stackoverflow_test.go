package signal

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSOPoller_Source(t *testing.T) {
	p := NewSOPoller(SOConfig{Tags: []string{"go"}})
	if p.Source() != SourceStackOverflow {
		t.Errorf("source = %q, want %q", p.Source(), SourceStackOverflow)
	}
}

func gzipJSON(data string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(data))
	gz.Close()
	return buf.Bytes()
}

func soJSON(ts int64) string {
	return fmt.Sprintf(`{
		"items": [{
			"question_id": 12345,
			"title": "How to use channels in Go?",
			"link": "https://stackoverflow.com/q/12345",
			"tags": ["go", "concurrency"],
			"score": 15,
			"answer_count": 3,
			"view_count": 500,
			"is_answered": true,
			"owner": {"display_name": "GoDevAlice", "link": "https://stackoverflow.com/u/1"},
			"creation_date": %d
		}]
	}`, ts)
}

func TestSOPoller_Poll(t *testing.T) {
	now := time.Now().UTC()
	body := gzipJSON(soJSON(now.Unix()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(body)
	}))
	defer srv.Close()

	poller := NewSOPoller(SOConfig{Tags: []string{"go"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Source != SourceStackOverflow {
		t.Errorf("source = %q", e.Source)
	}
	if e.Kind != KindPost {
		t.Errorf("kind = %q, want %q", e.Kind, KindPost)
	}
	if e.Title != "How to use channels in Go?" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Actor != "GoDevAlice" {
		t.Errorf("actor = %q", e.Actor)
	}
	if e.SourceID != "so:12345" {
		t.Errorf("sourceID = %q", e.SourceID)
	}
	if e.Metadata["score"] != 15 {
		t.Errorf("score = %v, want 15", e.Metadata["score"])
	}
	if e.Metadata["isAnswered"] != true {
		t.Errorf("isAnswered = %v, want true", e.Metadata["isAnswered"])
	}
}

func TestSOPoller_NoTags(t *testing.T) {
	poller := NewSOPoller(SOConfig{Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if events != nil {
		t.Errorf("events = %v, want nil", events)
	}
}

func TestSOPoller_APIKeyInURL(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	poller := NewSOPoller(SOConfig{Tags: []string{"go"}, APIKey: "mykey123", Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))

	if !strings.Contains(capturedURL, "key=mykey123") {
		t.Errorf("URL = %q, want to contain key=mykey123", capturedURL)
	}
}

func TestSOPoller_TagJoining(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	poller := NewSOPoller(SOConfig{Tags: []string{"go", "rust", "zig"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))

	if !strings.Contains(capturedURL, "tagged=go;rust;zig") {
		t.Errorf("URL = %q, want to contain tagged=go;rust;zig", capturedURL)
	}
}

func TestSOPoller_NonGzipResponse(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No gzip Content-Encoding
		w.Write([]byte(soJSON(now.Unix())))
	}))
	defer srv.Close()

	poller := NewSOPoller(SOConfig{Tags: []string{"go"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Title != "How to use channels in Go?" {
		t.Errorf("title = %q", events[0].Title)
	}
}

func TestSOPoller_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	poller := NewSOPoller(SOConfig{Tags: []string{"go"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	_, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err == nil {
		t.Fatal("expected error for HTTP 429")
	}
	if !strings.Contains(err.Error(), "status 429") {
		t.Errorf("error = %q, want to contain 'status 429'", err.Error())
	}
}
