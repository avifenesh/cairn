package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NPMPoller tracks new versions of configured npm packages.
type NPMPoller struct {
	packages []string
	client   *http.Client
}

// NPMConfig holds configuration for the npm poller.
type NPMConfig struct {
	Packages []string // Package names to track (e.g. "svelte", "@anthropic-ai/sdk")
}

// NewNPMPoller creates an npm registry poller.
func NewNPMPoller(cfg NPMConfig) *NPMPoller {
	return &NPMPoller{
		packages: cfg.Packages,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (n *NPMPoller) Source() string { return SourceNPM }

func (n *NPMPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	var all []*RawEvent
	for _, pkg := range n.packages {
		ev, err := n.checkPackage(ctx, pkg, since)
		if err != nil {
			continue
		}
		if ev != nil {
			all = append(all, ev)
		}
	}
	return all, nil
}

type npmPackageInfo struct {
	Name    string `json:"name"`
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Time map[string]string `json:"time"` // version -> ISO timestamp
}

func (n *NPMPoller) checkPackage(ctx context.Context, pkg string, since time.Time) (*RawEvent, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", pkg)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cairn/1.0 (signal poller)")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("npm: request %s: %w", pkg, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm: %s status %d", pkg, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("npm: read %s: %w", pkg, err)
	}

	var info npmPackageInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("npm: parse %s: %w", pkg, err)
	}

	latest := info.DistTags.Latest
	if latest == "" {
		return nil, nil
	}

	// Check if this version was published after since.
	publishedStr, ok := info.Time[latest]
	if !ok {
		return nil, nil
	}
	published, err := time.Parse(time.RFC3339, publishedStr)
	if err != nil {
		return nil, fmt.Errorf("npm: parse time for %s@%s: %w", pkg, latest, err)
	}
	if published.Before(since) {
		return nil, nil
	}

	return &RawEvent{
		Source:   SourceNPM,
		SourceID: fmt.Sprintf("pkg:%s@%s", pkg, latest),
		Kind:     KindPackage,
		Title:    fmt.Sprintf("%s %s published", pkg, latest),
		URL:      fmt.Sprintf("https://www.npmjs.com/package/%s/v/%s", pkg, latest),
		GroupKey: "npm",
		Metadata: map[string]any{
			"package": pkg,
			"version": latest,
		},
		OccurredAt: published,
	}, nil
}
