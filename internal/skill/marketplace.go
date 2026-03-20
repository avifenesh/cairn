package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultClawHubURL  = "https://clawhub.ai"
	marketplaceTimeout = 30 * time.Second
	maxJSONSize        = 5 * 1024 * 1024  // 5 MB
	maxZipSize         = 50 * 1024 * 1024 // 50 MB
	userAgent          = "Cairn/1.0 (personal agent)"
)

// RateLimitError is returned when the ClawHub API rate limit is hit.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("clawhub: rate limited, retry after %s", e.RetryAfter)
}

// MarketplaceSearchResult maps to a single result from ClawHub search.
type MarketplaceSearchResult struct {
	Score       float64 `json:"score"`
	Slug        string  `json:"slug"`
	DisplayName string  `json:"displayName"`
	Summary     string  `json:"summary"`
	Version     string  `json:"version"`
	UpdatedAt   int64   `json:"updatedAt"`
}

// MarketplaceSkillStats holds download/star counts.
type MarketplaceSkillStats struct {
	Downloads int `json:"downloads"`
	Stars     int `json:"stars"`
	Versions  int `json:"versions"`
	Installs  int `json:"installsAllTime"`
}

// MarketplaceOwner holds author information.
type MarketplaceOwner struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Image       string `json:"image"`
}

// MarketplaceVersion holds version metadata.
type MarketplaceVersion struct {
	Version   string `json:"version"`
	Changelog string `json:"changelog"`
}

// MarketplaceSkillDetail maps to ClawHub GET /api/v1/skills/<slug>.
type MarketplaceSkillDetail struct {
	Slug          string                 `json:"slug"`
	DisplayName   string                 `json:"displayName"`
	Summary       string                 `json:"summary"`
	Stats         MarketplaceSkillStats  `json:"stats"`
	Owner         MarketplaceOwner       `json:"owner"`
	LatestVersion MarketplaceVersion     `json:"latestVersion"`
	Tags          map[string]string      `json:"tags"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// Provenance records where a marketplace-installed skill came from.
type Provenance struct {
	Source      string `json:"source"`
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	InstalledAt string `json:"installedAt"`
	URL         string `json:"url"`
}

// MarketplaceClient communicates with the ClawHub skill registry API.
type MarketplaceClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewMarketplaceClient creates a marketplace client for the given ClawHub base URL.
func NewMarketplaceClient(baseURL string, logger *slog.Logger) *MarketplaceClient {
	if baseURL == "" {
		baseURL = defaultClawHubURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MarketplaceClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: marketplaceTimeout},
		logger:     logger,
	}
}

// Search queries ClawHub for skills matching the given text.
func (c *MarketplaceClient) Search(ctx context.Context, query string, limit int) ([]MarketplaceSearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	u := fmt.Sprintf("%s/api/v1/search?q=%s&limit=%d", c.baseURL, url.QueryEscape(query), limit)

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("clawhub: search: %w", err)
	}

	var resp struct {
		Results []MarketplaceSearchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("clawhub: search: parse response: %w", err)
	}
	return resp.Results, nil
}

// Browse lists skills from ClawHub sorted by the given criterion.
func (c *MarketplaceClient) Browse(ctx context.Context, sort string, limit int) ([]MarketplaceSkillDetail, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	validSorts := map[string]bool{"updated": true, "downloads": true, "stars": true, "trending": true, "installs": true}
	if !validSorts[sort] {
		sort = "trending"
	}
	u := fmt.Sprintf("%s/api/v1/skills?limit=%d&sort=%s", c.baseURL, limit, sort)

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("clawhub: browse: %w", err)
	}

	// The ClawHub browse endpoint wraps results differently than search.
	var resp struct {
		Skills []MarketplaceSkillDetail `json:"skills"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("clawhub: browse: parse response: %w", err)
	}
	return resp.Skills, nil
}

// Detail fetches full metadata for a single skill.
func (c *MarketplaceClient) Detail(ctx context.Context, slug string) (*MarketplaceSkillDetail, error) {
	u := fmt.Sprintf("%s/api/v1/skills/%s", c.baseURL, url.PathEscape(slug))

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("clawhub: detail %q: %w", slug, err)
	}

	// The detail endpoint nests the skill data under a "skill" key.
	var resp struct {
		Skill         MarketplaceSkillDetail `json:"skill"`
		LatestVersion MarketplaceVersion     `json:"latestVersion"`
		Owner         MarketplaceOwner       `json:"owner"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("clawhub: detail %q: parse response: %w", slug, err)
	}
	// Merge top-level owner and version into the skill object.
	detail := resp.Skill
	if detail.Owner.Handle == "" {
		detail.Owner = resp.Owner
	}
	if detail.LatestVersion.Version == "" {
		detail.LatestVersion = resp.LatestVersion
	}
	return &detail, nil
}

// Preview fetches the raw SKILL.md content for a skill.
func (c *MarketplaceClient) Preview(ctx context.Context, slug string) (string, error) {
	u := fmt.Sprintf("%s/api/v1/skills/%s/file?path=SKILL.md", c.baseURL, url.PathEscape(slug))

	body, err := c.doGet(ctx, u)
	if err != nil {
		return "", fmt.Errorf("clawhub: preview %q: %w", slug, err)
	}
	return string(body), nil
}

// Install downloads a skill zip from ClawHub and extracts it to targetDir/<slug>/.
// Returns provenance information on success.
func (c *MarketplaceClient) Install(ctx context.Context, slug, targetDir string) (*Provenance, error) {
	// Download the zip.
	u := fmt.Sprintf("%s/api/v1/download?slug=%s", c.baseURL, url.QueryEscape(slug))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("clawhub: install %q: %w", slug, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clawhub: install %q: %w", slug, err)
	}
	defer resp.Body.Close()

	if err := c.checkRateLimit(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("clawhub: install %q: HTTP %d: %s", slug, resp.StatusCode, string(errBody))
	}

	zipData, err := io.ReadAll(io.LimitReader(resp.Body, maxZipSize+1))
	if err != nil {
		return nil, fmt.Errorf("clawhub: install %q: read zip: %w", slug, err)
	}
	if int64(len(zipData)) > maxZipSize {
		return nil, fmt.Errorf("clawhub: install %q: zip exceeds %d MB limit", slug, maxZipSize/(1024*1024))
	}

	// Extract the zip.
	destDir := filepath.Join(targetDir, slug)
	if err := extractZip(zipData, destDir); err != nil {
		return nil, fmt.Errorf("clawhub: install %q: extract: %w", slug, err)
	}

	// Verify SKILL.md exists in extracted content.
	skillMDPath := filepath.Join(destDir, "SKILL.md")
	if _, err := os.Stat(skillMDPath); err != nil {
		// Clean up failed install.
		os.RemoveAll(destDir)
		return nil, fmt.Errorf("clawhub: install %q: no SKILL.md found in package", slug)
	}

	// Fetch version info for provenance.
	version := ""
	detail, detailErr := c.Detail(ctx, slug)
	if detailErr == nil && detail != nil {
		version = detail.LatestVersion.Version
	}

	// Write provenance.
	prov := &Provenance{
		Source:      "clawhub",
		Slug:        slug,
		Version:     version,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
		URL:         fmt.Sprintf("%s/skills/%s", c.baseURL, slug),
	}

	clawhubDir := filepath.Join(destDir, ".clawhub")
	if err := os.MkdirAll(clawhubDir, 0755); err != nil {
		c.logger.Warn("clawhub: failed to create provenance dir", "error", err)
	} else {
		provJSON, _ := json.MarshalIndent(prov, "", "  ")
		if err := os.WriteFile(filepath.Join(clawhubDir, "origin.json"), provJSON, 0644); err != nil {
			c.logger.Warn("clawhub: failed to write provenance", "error", err)
		}
	}

	c.logger.Info("skill installed from clawhub", "slug", slug, "version", version, "dir", destDir)
	return prov, nil
}

// doGet performs a GET request and returns the response body.
func (c *MarketplaceClient) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkRateLimit(resp); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONSize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// checkRateLimit inspects the response for 429 status and returns a RateLimitError.
func (c *MarketplaceClient) checkRateLimit(resp *http.Response) error {
	if resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	retryAfter := 60 * time.Second // default
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			retryAfter = time.Duration(secs) * time.Second
		}
	}
	io.ReadAll(io.LimitReader(resp.Body, 1024)) // drain body
	return &RateLimitError{RetryAfter: retryAfter}
}

// extractZip extracts a zip archive into destDir with zip-slip protection.
func extractZip(data []byte, destDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve dest: %w", err)
	}

	if err := os.MkdirAll(absDestDir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	var totalSize int64

	for _, file := range reader.File {
		// Zip-slip protection: resolve the path and verify it's within destDir.
		cleanName := filepath.Clean(file.Name)
		if strings.Contains(cleanName, "..") {
			return fmt.Errorf("zip contains unsafe path: %q", file.Name)
		}

		target := filepath.Join(absDestDir, cleanName)
		if !strings.HasPrefix(target, absDestDir+string(os.PathSeparator)) && target != absDestDir {
			return fmt.Errorf("zip entry escapes target dir: %q", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create dir %q: %w", cleanName, err)
			}
			continue
		}

		// Size check.
		totalSize += int64(file.UncompressedSize64)
		if totalSize > maxZipSize {
			return fmt.Errorf("extracted content exceeds %d MB limit", maxZipSize/(1024*1024))
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("create parent for %q: %w", cleanName, err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", cleanName, err)
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create file %q: %w", cleanName, err)
		}

		_, copyErr := io.Copy(out, io.LimitReader(rc, maxZipSize))
		rc.Close()
		out.Close()
		if copyErr != nil {
			return fmt.Errorf("write file %q: %w", cleanName, copyErr)
		}
	}

	return nil
}
