package signal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const rssTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
    %s
  </channel>
</rss>`

func rssItem(guid, title, link, desc, pubDate, author string, categories []string) string {
	catXML := ""
	for _, c := range categories {
		catXML += fmt.Sprintf("<category>%s</category>", c)
	}
	authorXML := ""
	if author != "" {
		authorXML = fmt.Sprintf("<author>%s</author>", author)
	}
	guidXML := ""
	if guid != "" {
		guidXML = fmt.Sprintf("<guid>%s</guid>", guid)
	}
	linkXML := ""
	if link != "" {
		linkXML = fmt.Sprintf("<link>%s</link>", link)
	}
	return fmt.Sprintf(`<item>
      %s
      <title>%s</title>
      %s
      <description>%s</description>
      <pubDate>%s</pubDate>
      %s
      %s
    </item>`, guidXML, title, linkXML, desc, pubDate, authorXML, catXML)
}

func TestRSSPoller_Source(t *testing.T) {
	p := NewRSSPoller(RSSConfig{Feeds: []string{"https://example.com/feed"}})
	if p.Source() != SourceRSS {
		t.Errorf("source = %q, want %q", p.Source(), SourceRSS)
	}
}

func TestRSSPoller_Poll(t *testing.T) {
	now := time.Now().UTC()
	pubDate := now.Format(time.RFC1123Z)
	xml := fmt.Sprintf(rssTemplate, rssItem("item-1", "Test Article", "https://example.com/article", "Short description", pubDate, "alice@example.com (Alice)", []string{"tech"}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Source != SourceRSS {
		t.Errorf("source = %q", e.Source)
	}
	if e.Title != "Test Article" {
		t.Errorf("title = %q", e.Title)
	}
	if e.URL != "https://example.com/article" {
		t.Errorf("url = %q", e.URL)
	}
	if e.Body != "Short description" {
		t.Errorf("body = %q", e.Body)
	}
	if e.GroupKey != "Test Feed" {
		t.Errorf("groupKey = %q, want 'Test Feed'", e.GroupKey)
	}
	if e.Metadata["feedTitle"] != "Test Feed" {
		t.Errorf("feedTitle = %v", e.Metadata["feedTitle"])
	}
}

func TestRSSPoller_ReleaseDetection(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		title string
		kind  string
	}{
		{"v2.0.0 release notes", KindRelease},
		{"Changelog for March", KindRelease},
		{"Release v1.5.3", KindRelease},
		{"Regular blog post", KindPost},
	}
	for _, tt := range tests {
		pubDate := now.Format(time.RFC1123Z)
		xml := fmt.Sprintf(rssTemplate, rssItem("g-"+tt.title, tt.title, "https://example.com/"+tt.title, "desc", pubDate, "", nil))

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(xml))
		}))

		poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
		events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
		srv.Close()

		if err != nil {
			t.Fatalf("poll %q: %v", tt.title, err)
		}
		if len(events) != 1 {
			t.Fatalf("poll %q: events = %d, want 1", tt.title, len(events))
		}
		if events[0].Kind != tt.kind {
			t.Errorf("title %q: kind = %q, want %q", tt.title, events[0].Kind, tt.kind)
		}
	}
}

func TestRSSPoller_MissingGUID(t *testing.T) {
	now := time.Now().UTC()
	pubDate := now.Format(time.RFC1123Z)
	// No GUID, but has link - should use link as fallback.
	xml := fmt.Sprintf(rssTemplate, rssItem("", "No GUID", "https://example.com/no-guid", "desc", pubDate, "", nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	// SourceID should use link as fallback for GUID.
	if !strings.Contains(events[0].SourceID, "https://example.com/no-guid") {
		t.Errorf("sourceID = %q, want to contain link as fallback", events[0].SourceID)
	}
}

func TestRSSPoller_NoGUIDNoLink(t *testing.T) {
	now := time.Now().UTC()
	pubDate := now.Format(time.RFC1123Z)
	// No GUID, no link - should be skipped.
	xml := fmt.Sprintf(rssTemplate, rssItem("", "Orphan Item", "", "desc", pubDate, "", nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events = %d, want 0 (no guid + no link = skip)", len(events))
	}
}

func TestRSSPoller_AuthorNil(t *testing.T) {
	now := time.Now().UTC()
	pubDate := now.Format(time.RFC1123Z)
	xml := fmt.Sprintf(rssTemplate, rssItem("a1", "No Author", "https://example.com/a1", "desc", pubDate, "", nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Actor != "" {
		t.Errorf("actor = %q, want empty", events[0].Actor)
	}
}

func TestRSSPoller_BodyTruncation(t *testing.T) {
	now := time.Now().UTC()
	pubDate := now.Format(time.RFC1123Z)
	longDesc := strings.Repeat("x", 300)
	xml := fmt.Sprintf(rssTemplate, rssItem("b1", "Long Body", "https://example.com/b1", longDesc, pubDate, "", nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if len(events[0].Body) != 203 { // 200 + "..."
		t.Errorf("body len = %d, want 203", len(events[0].Body))
	}
	if !strings.HasSuffix(events[0].Body, "...") {
		t.Errorf("body should end with '...'")
	}
}

func TestRSSPoller_SinceFilter(t *testing.T) {
	old := time.Now().UTC().Add(-48 * time.Hour)
	pubDate := old.Format(time.RFC1123Z)
	xml := fmt.Sprintf(rssTemplate, rssItem("c1", "Old Article", "https://example.com/c1", "stale", pubDate, "", nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	poller := NewRSSPoller(RSSConfig{Feeds: []string{srv.URL}, Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events = %d, want 0 (old items filtered)", len(events))
	}
}

func TestRSSPoller_FeedError(t *testing.T) {
	now := time.Now().UTC()
	pubDate := now.Format(time.RFC1123Z)
	xml := fmt.Sprintf(rssTemplate, rssItem("d1", "Good Article", "https://example.com/d1", "works", pubDate, "", nil))

	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xml))
	}))
	defer goodSrv.Close()

	// Bad feed URL that will fail to parse.
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not valid XML or RSS"))
	}))
	defer badSrv.Close()

	poller := NewRSSPoller(RSSConfig{
		Feeds:  []string{badSrv.URL, goodSrv.URL},
		Logger: noopLogger(),
	})

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll should not return error on partial failure: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("events = %d, want 1 (second feed should succeed)", len(events))
	}
}
