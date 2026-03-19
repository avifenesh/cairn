package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// CratesPoller tracks download metrics for configured crates.io packages.
// Stores snapshots in SourceState.Extra, emits events on download deltas.
type CratesPoller struct {
	crates []string
	state  *SourceState
	client *http.Client
	logger *slog.Logger
}

// CratesConfig holds configuration for the crates.io poller.
type CratesConfig struct {
	Crates []string     // Crate names to track (e.g. "tokio", "serde")
	State  *SourceState // for storing download snapshots
	Logger *slog.Logger
}

// NewCratesPoller creates a crates.io download metrics poller.
func NewCratesPoller(cfg CratesConfig) *CratesPoller {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &CratesPoller{
		crates: cfg.Crates,
		state:  cfg.State,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

func (c *CratesPoller) Source() string { return SourceCrates }

type crateDownloadSnapshot struct {
	Downloads      int    `json:"downloads"`
	RecentDL       int    `json:"recentDownloads"`
	CurrentVersion string `json:"currentVersion"`
}

func (c *CratesPoller) Poll(ctx context.Context, _ time.Time) ([]*RawEvent, error) {
	extra, err := c.state.GetExtra(ctx, SourceCrates)
	if err != nil {
		extra = map[string]any{}
	}

	var events []*RawEvent
	for _, crate := range c.crates {
		if ctx.Err() != nil {
			break
		}

		current, err := c.fetchMetrics(ctx, crate)
		if err != nil {
			c.logger.Warn("crates: fetch metrics failed", "crate", crate, "error", err)
			continue
		}

		// Load previous snapshot.
		key := "crate:" + crate
		var prev crateDownloadSnapshot
		if raw, ok := extra[key]; ok {
			if b, err := json.Marshal(raw); err == nil {
				json.Unmarshal(b, &prev)
			}
		}

		dTotal := current.Downloads - prev.Downloads
		dRecent := current.RecentDL - prev.RecentDL

		// Always emit metrics event for charting.
		date := time.Now().UTC().Format("2006-01-02")
		events = append(events, &RawEvent{
			Source:   SourceCrates,
			SourceID: fmt.Sprintf("crate:metrics:%s:%s", crate, date),
			Kind:     KindMetrics,
			Title:    fmt.Sprintf("%s: %d recent downloads", crate, current.RecentDL),
			URL:      fmt.Sprintf("https://crates.io/crates/%s", crate),
			GroupKey: "crates",
			Metadata: map[string]any{
				"crate":           crate,
				"version":         current.CurrentVersion,
				"downloads":       current.Downloads,
				"recentDownloads": current.RecentDL,
				"dTotal":          dTotal,
				"dRecent":         dRecent,
			},
			OccurredAt: time.Now().UTC(),
		})

		extra[key] = current
	}

	if err := c.state.SetExtra(ctx, SourceCrates, extra); err != nil {
		c.logger.Warn("crates: failed to save download snapshots", "error", err)
	}

	return events, nil
}

func (c *CratesPoller) fetchMetrics(ctx context.Context, crate string) (crateDownloadSnapshot, error) {
	url := fmt.Sprintf("https://crates.io/api/v1/crates/%s", crate)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return crateDownloadSnapshot{}, err
	}
	req.Header.Set("User-Agent", "cairn/1.0 (signal poller)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return crateDownloadSnapshot{}, fmt.Errorf("crates: request %s: %w", crate, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return crateDownloadSnapshot{}, fmt.Errorf("crates: %s status %d", crate, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return crateDownloadSnapshot{}, err
	}

	var info struct {
		Crate struct {
			Downloads      int    `json:"downloads"`
			RecentDownload int    `json:"recent_downloads"`
			MaxVersion     string `json:"max_version"`
		} `json:"crate"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return crateDownloadSnapshot{}, fmt.Errorf("crates: parse %s: %w", crate, err)
	}

	return crateDownloadSnapshot{
		Downloads:      info.Crate.Downloads,
		RecentDL:       info.Crate.RecentDownload,
		CurrentVersion: info.Crate.MaxVersion,
	}, nil
}
