package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// NPMPoller tracks download metrics for configured npm packages.
// Stores snapshots in SourceState.Extra, emits events on download deltas.
type NPMPoller struct {
	packages []string
	state    *SourceState
	client   *http.Client
	logger   *slog.Logger
}

// NPMConfig holds configuration for the npm poller.
type NPMConfig struct {
	Packages []string     // Package names to track (e.g. "svelte", "@anthropic-ai/sdk")
	State    *SourceState // for storing download snapshots
	Logger   *slog.Logger
}

// NewNPMPoller creates an npm download metrics poller.
func NewNPMPoller(cfg NPMConfig) *NPMPoller {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &NPMPoller{
		packages: cfg.Packages,
		state:    cfg.State,
		client:   &http.Client{Timeout: 30 * time.Second},
		logger:   logger,
	}
}

func (n *NPMPoller) Source() string { return SourceNPM }

type npmDownloadSnapshot struct {
	WeeklyDownloads int `json:"weeklyDownloads"`
	TotalDownloads  int `json:"totalDownloads"`
}

func (n *NPMPoller) Poll(ctx context.Context, _ time.Time) ([]*RawEvent, error) {
	extra, err := n.state.GetExtra(ctx, SourceNPM)
	if err != nil {
		extra = map[string]any{}
	}

	var events []*RawEvent
	for _, pkg := range n.packages {
		if ctx.Err() != nil {
			break
		}

		current, err := n.fetchMetrics(ctx, pkg)
		if err != nil {
			n.logger.Warn("npm: fetch metrics failed", "package", pkg, "error", err)
			continue
		}

		// Load previous snapshot.
		key := "npm:" + pkg
		var prev npmDownloadSnapshot
		if raw, ok := extra[key]; ok {
			if b, err := json.Marshal(raw); err == nil {
				json.Unmarshal(b, &prev)
			}
		}

		// Compute deltas.
		dWeekly := current.WeeklyDownloads - prev.WeeklyDownloads
		dTotal := current.TotalDownloads - prev.TotalDownloads

		// Always emit a metrics event (for charting over time).
		date := time.Now().UTC().Format("2006-01-02")
		events = append(events, &RawEvent{
			Source:   SourceNPM,
			SourceID: fmt.Sprintf("npm:metrics:%s:%s", pkg, date),
			Kind:     KindMetrics,
			Title:    fmt.Sprintf("%s: %d weekly downloads", pkg, current.WeeklyDownloads),
			URL:      fmt.Sprintf("https://www.npmjs.com/package/%s", pkg),
			GroupKey: "npm",
			Metadata: map[string]any{
				"package":         pkg,
				"weeklyDownloads": current.WeeklyDownloads,
				"totalDownloads":  current.TotalDownloads,
				"dWeekly":         dWeekly,
				"dTotal":          dTotal,
			},
			OccurredAt: time.Now().UTC(),
		})

		// Save snapshot.
		extra[key] = current
	}

	if err := n.state.SetExtra(ctx, SourceNPM, extra); err != nil {
		n.logger.Warn("npm: failed to save download snapshots", "error", err)
	}

	return events, nil
}

func (n *NPMPoller) fetchMetrics(ctx context.Context, pkg string) (npmDownloadSnapshot, error) {
	var snapshot npmDownloadSnapshot

	// Weekly downloads.
	weekURL := fmt.Sprintf("https://api.npmjs.org/downloads/point/last-week/%s", url.PathEscape(pkg))
	weekData, err := n.httpGet(ctx, weekURL)
	if err != nil {
		return snapshot, err
	}
	var weekResp struct {
		Downloads int `json:"downloads"`
	}
	if err := json.Unmarshal(weekData, &weekResp); err != nil {
		return snapshot, fmt.Errorf("npm: parse weekly %s: %w", pkg, err)
	}
	snapshot.WeeklyDownloads = weekResp.Downloads

	// Total downloads (all time = last 18 months, npm doesn't have true total).
	totalURL := fmt.Sprintf("https://api.npmjs.org/downloads/point/2020-01-01:%s/%s",
		time.Now().UTC().Format("2006-01-02"), url.PathEscape(pkg))
	totalData, err := n.httpGet(ctx, totalURL)
	if err == nil {
		var totalResp struct {
			Downloads int `json:"downloads"`
		}
		if json.Unmarshal(totalData, &totalResp) == nil {
			snapshot.TotalDownloads = totalResp.Downloads
		}
	}

	return snapshot, nil
}

func (n *NPMPoller) httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cairn/1.0 (signal poller)")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm: status %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
