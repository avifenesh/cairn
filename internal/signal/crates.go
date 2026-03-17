package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CratesPoller tracks new versions of configured crates.io packages.
type CratesPoller struct {
	crates []string
	client *http.Client
}

// CratesConfig holds configuration for the crates.io poller.
type CratesConfig struct {
	Crates []string // Crate names to track (e.g. "tokio", "serde")
}

// NewCratesPoller creates a crates.io registry poller.
func NewCratesPoller(cfg CratesConfig) *CratesPoller {
	return &CratesPoller{
		crates: cfg.Crates,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *CratesPoller) Source() string { return SourceCrates }

func (c *CratesPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	var all []*RawEvent
	var lastErr error
	for _, crate := range c.crates {
		ev, err := c.checkCrate(ctx, crate, since)
		if err != nil {
			lastErr = err
			continue
		}
		if ev != nil {
			all = append(all, ev)
		}
	}
	if len(all) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return all, nil
}

type crateInfo struct {
	Crate struct {
		Name       string `json:"name"`
		MaxVersion string `json:"max_version"`
		Updated    string `json:"updated_at"` // ISO 8601
	} `json:"crate"`
}

func (c *CratesPoller) checkCrate(ctx context.Context, crate string, since time.Time) (*RawEvent, error) {
	url := fmt.Sprintf("https://crates.io/api/v1/crates/%s", crate)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cairn/1.0 (signal poller)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crates: request %s: %w", crate, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crates: %s status %d", crate, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("crates: read %s: %w", crate, err)
	}

	var info crateInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("crates: parse %s: %w", crate, err)
	}

	version := info.Crate.MaxVersion
	if version == "" {
		return nil, nil
	}

	updated, err := time.Parse(time.RFC3339, info.Crate.Updated)
	if err != nil {
		return nil, fmt.Errorf("crates: parse time for %s: %w", crate, err)
	}
	if updated.Before(since) {
		return nil, nil
	}

	return &RawEvent{
		Source:   SourceCrates,
		SourceID: fmt.Sprintf("crate:%s@%s", crate, version),
		Kind:     KindPackage,
		Title:    fmt.Sprintf("%s %s published", crate, version),
		URL:      fmt.Sprintf("https://crates.io/crates/%s/%s", crate, version),
		GroupKey: "crates",
		Metadata: map[string]any{
			"crate":   crate,
			"version": version,
		},
		OccurredAt: updated,
	}, nil
}
