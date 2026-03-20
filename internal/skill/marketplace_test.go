package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMarketplaceSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if q != "git" {
			t.Errorf("expected q=git, got %q", q)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"slug": "git-essentials", "displayName": "Git Essentials", "summary": "Git commands", "version": "1.0.0", "score": 3.77, "updatedAt": 1772075603999},
			},
		})
	}))
	defer srv.Close()

	c := NewMarketplaceClient(srv.URL, slog.Default())
	results, err := c.Search(context.Background(), "git", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Slug != "git-essentials" {
		t.Fatalf("expected slug git-essentials, got %q", results[0].Slug)
	}
	if results[0].Score != 3.77 {
		t.Fatalf("expected score 3.77, got %f", results[0].Score)
	}
}

func TestMarketplaceBrowse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		sort := r.URL.Query().Get("sort")
		if sort != "trending" {
			t.Errorf("expected sort=trending, got %q", sort)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"skills": []map[string]any{
				{"slug": "docker", "displayName": "Docker", "summary": "Docker stuff"},
			},
		})
	}))
	defer srv.Close()

	c := NewMarketplaceClient(srv.URL, slog.Default())
	results, err := c.Browse(context.Background(), "trending", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Slug != "docker" {
		t.Fatalf("expected slug docker, got %q", results[0].Slug)
	}
}

func TestMarketplaceDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"skill": map[string]any{
				"slug":        "git-essentials",
				"displayName": "Git Essentials",
				"summary":     "Essential Git commands",
				"stats":       map[string]any{"downloads": 18504, "stars": 25},
			},
			"latestVersion": map[string]any{"version": "1.0.0"},
			"owner":         map[string]any{"handle": "Arnarsson", "image": "https://example.com/avatar.png"},
		})
	}))
	defer srv.Close()

	c := NewMarketplaceClient(srv.URL, slog.Default())
	detail, err := c.Detail(context.Background(), "git-essentials")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Slug != "git-essentials" {
		t.Fatalf("expected slug git-essentials, got %q", detail.Slug)
	}
	if detail.Stats.Downloads != 18504 {
		t.Fatalf("expected 18504 downloads, got %d", detail.Stats.Downloads)
	}
	if detail.LatestVersion.Version != "1.0.0" {
		t.Fatalf("expected version 1.0.0, got %q", detail.LatestVersion.Version)
	}
	if detail.Owner.Handle != "Arnarsson" {
		t.Fatalf("expected owner Arnarsson, got %q", detail.Owner.Handle)
	}
}

func TestMarketplacePreview(t *testing.T) {
	skillContent := "---\nname: test-skill\ndescription: \"A test\"\n---\n\n# Test Skill\n\nHello world."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(skillContent))
	}))
	defer srv.Close()

	c := NewMarketplaceClient(srv.URL, slog.Default())
	content, err := c.Preview(context.Background(), "test-skill")
	if err != nil {
		t.Fatal(err)
	}
	if content != skillContent {
		t.Fatalf("expected skill content, got %q", content)
	}
}

func TestMarketplaceInstall(t *testing.T) {
	skillMD := "---\nname: test-skill\ndescription: \"A test skill\"\n---\n\n# Test\n\nDo things."
	zipData := makeTestZip(t, map[string]string{
		"SKILL.md": skillMD,
	})

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/api/v1/download" {
			w.Header().Set("Content-Type", "application/zip")
			w.Write(zipData)
			return
		}
		// Detail call for provenance.
		json.NewEncoder(w).Encode(map[string]any{
			"skill":         map[string]any{"slug": "test-skill"},
			"latestVersion": map[string]any{"version": "1.2.3"},
		})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c := NewMarketplaceClient(srv.URL, slog.Default())
	prov, err := c.Install(context.Background(), "test-skill", tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify SKILL.md was extracted.
	extracted, err := os.ReadFile(filepath.Join(tmpDir, "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not found: %v", err)
	}
	if string(extracted) != skillMD {
		t.Fatalf("SKILL.md content mismatch")
	}

	// Verify provenance.
	if prov.Source != "clawhub" {
		t.Fatalf("expected source clawhub, got %q", prov.Source)
	}
	if prov.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %q", prov.Version)
	}

	// Verify origin.json was written.
	originPath := filepath.Join(tmpDir, "test-skill", ".clawhub", "origin.json")
	if _, err := os.Stat(originPath); err != nil {
		t.Fatalf("origin.json not found: %v", err)
	}
}

func TestMarketplaceInstall_ZipSlip(t *testing.T) {
	// Create a zip with a path traversal entry.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("../../../etc/passwd")
	f.Write([]byte("malicious"))
	w.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c := NewMarketplaceClient(srv.URL, slog.Default())
	_, err := c.Install(context.Background(), "evil-skill", tmpDir)
	if err == nil {
		t.Fatal("expected error for zip-slip attack")
	}
	if !strings.Contains(err.Error(), "unsafe path") && !strings.Contains(err.Error(), "escapes target") {
		t.Fatalf("expected zip-slip error, got: %v", err)
	}
}

func TestMarketplaceInstall_NoSkillMD(t *testing.T) {
	zipData := makeTestZip(t, map[string]string{
		"README.md": "# Not a skill",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c := NewMarketplaceClient(srv.URL, slog.Default())
	_, err := c.Install(context.Background(), "bad-skill", tmpDir)
	if err == nil {
		t.Fatal("expected error for missing SKILL.md")
	}
	if !strings.Contains(err.Error(), "no SKILL.md") {
		t.Fatalf("expected no SKILL.md error, got: %v", err)
	}
}

func TestMarketplaceRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	c := NewMarketplaceClient(srv.URL, slog.Default())
	_, err := c.Search(context.Background(), "git", 5)
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	rlErr, ok := err.(*RateLimitError)
	if !ok {
		// The error is wrapped, check the underlying cause.
		var found bool
		for unwrapped := err; unwrapped != nil; {
			if rl, ok2 := unwrapped.(*RateLimitError); ok2 {
				rlErr = rl
				found = true
				break
			}
			if u, ok2 := unwrapped.(interface{ Unwrap() error }); ok2 {
				unwrapped = u.Unwrap()
			} else {
				break
			}
		}
		if !found {
			t.Fatalf("expected RateLimitError, got %T: %v", err, err)
		}
	}
	if rlErr.RetryAfter.Seconds() != 30 {
		t.Fatalf("expected 30s retry, got %s", rlErr.RetryAfter)
	}
}

