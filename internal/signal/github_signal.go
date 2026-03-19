package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GitHubSignalConfig configures the GitHub signal intelligence poller.
type GitHubSignalConfig struct {
	Token           string
	Owner           string       // your GitHub login (for self-filter)
	TrackedRepos    []string     // explicit list; empty = auto-detect
	Orgs            []string     // orgs to auto-detect repos from
	BotFilter       []string     // additional bot logins
	State           *SourceState // for storing snapshots in Extra
	Logger          *slog.Logger
	MetricsInterval time.Duration // how often to poll metrics (default 4h)
}

// GitHubSignalPoller watches GitHub for external engagement, growth metrics,
// new stargazers, followers, and repos. Filters out bots and self-activity.
type GitHubSignalPoller struct {
	token           string
	owner           string
	repos           []string
	orgs            []string
	bots            map[string]bool
	state           *SourceState
	client          *http.Client
	logger          *slog.Logger
	lastMetrics     time.Time
	metricsInterval time.Duration
}

// NewGitHubSignalPoller creates a signal intelligence poller.
func NewGitHubSignalPoller(cfg GitHubSignalConfig) *GitHubSignalPoller {
	bots := make(map[string]bool, len(cfg.BotFilter))
	for _, b := range cfg.BotFilter {
		bots[strings.ToLower(strings.TrimSpace(b))] = true
	}
	interval := cfg.MetricsInterval
	if interval <= 0 {
		interval = 4 * time.Hour
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &GitHubSignalPoller{
		token:           cfg.Token,
		owner:           strings.ToLower(cfg.Owner),
		repos:           cfg.TrackedRepos,
		orgs:            cfg.Orgs,
		bots:            bots,
		state:           cfg.State,
		client:          &http.Client{Timeout: 30 * time.Second},
		logger:          logger,
		metricsInterval: interval,
	}
}

func (g *GitHubSignalPoller) Source() string { return SourceGitHubSignal }

func (g *GitHubSignalPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	if g.token == "" {
		return nil, fmt.Errorf("github_signal: no token configured")
	}

	// Auto-detect repos on first poll.
	if len(g.repos) == 0 {
		repos, err := g.discoverRepos(ctx)
		if err != nil {
			return nil, fmt.Errorf("github_signal: discover repos: %w", err)
		}
		g.repos = repos
		g.logger.Info("github_signal: discovered repos", "count", len(repos))
	}

	var events []*RawEvent

	// Engagement: external issues, PRs, comments per repo.
	for _, repo := range g.repos {
		if ctx.Err() != nil {
			break
		}
		eng, err := g.pollEngagement(ctx, repo, since)
		if err != nil {
			g.logger.Warn("github_signal: engagement poll failed", "repo", repo, "error", err)
			continue
		}
		events = append(events, eng...)
	}

	// Metrics, stargazers, followers, new repos (throttled).
	if time.Since(g.lastMetrics) >= g.metricsInterval {
		g.lastMetrics = time.Now()

		met, err := g.pollMetrics(ctx)
		if err != nil {
			g.logger.Warn("github_signal: metrics poll failed", "error", err)
		} else {
			events = append(events, met...)
		}

		stars, err := g.pollStargazers(ctx)
		if err != nil {
			g.logger.Warn("github_signal: stargazers poll failed", "error", err)
		} else {
			events = append(events, stars...)
		}

		fol, err := g.pollFollowers(ctx)
		if err != nil {
			g.logger.Warn("github_signal: followers poll failed", "error", err)
		} else {
			events = append(events, fol...)
		}

		nr, err := g.pollNewRepos(ctx)
		if err != nil {
			g.logger.Warn("github_signal: new repos poll failed", "error", err)
		} else {
			events = append(events, nr...)
		}
	}

	return events, nil
}

// --- Engagement ---

func (g *GitHubSignalPoller) pollEngagement(ctx context.Context, repo string, since time.Time) ([]*RawEvent, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	var events []*RawEvent

	// Issues by external users.
	issues, err := g.ghGet(ctx, fmt.Sprintf("/repos/%s/issues?state=open&sort=created&since=%s&per_page=30", repo, sinceStr))
	if err == nil {
		var items []struct {
			Number    int       `json:"number"`
			Title     string    `json:"title"`
			HTMLURL   string    `json:"html_url"`
			User      ghUser    `json:"user"`
			CreatedAt string    `json:"created_at"`
			PR        *struct{} `json:"pull_request"` // non-nil means it's a PR
		}
		if json.Unmarshal(issues, &items) == nil {
			for _, it := range items {
				if it.PR != nil {
					continue // skip PRs listed in issues endpoint
				}
				if g.isSelfOrBot(it.User.Login) {
					continue
				}
				events = append(events, &RawEvent{
					Source:     SourceGitHubSignal,
					SourceID:   fmt.Sprintf("signal:issue:%s:%d", repo, it.Number),
					Kind:       KindIssue,
					Title:      fmt.Sprintf("[issue] %s #%d: %s", repo, it.Number, it.Title),
					URL:        it.HTMLURL,
					Actor:      it.User.Login,
					Repo:       repo,
					GroupKey:   repo,
					Metadata:   map[string]any{"type": "external_issue"},
					OccurredAt: parseGHTime(it.CreatedAt),
				})
			}
		}
	}
	g.rateSleep(ctx)

	// PRs by external users.
	prs, err := g.ghGet(ctx, fmt.Sprintf("/repos/%s/pulls?state=open&sort=created&direction=desc&per_page=30", repo))
	if err == nil {
		var items []struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			HTMLURL   string `json:"html_url"`
			User      ghUser `json:"user"`
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(prs, &items) == nil {
			for _, it := range items {
				if parseGHTime(it.CreatedAt).Before(since) {
					continue
				}
				if g.isSelfOrBot(it.User.Login) {
					continue
				}
				events = append(events, &RawEvent{
					Source:     SourceGitHubSignal,
					SourceID:   fmt.Sprintf("signal:pr:%s:%d", repo, it.Number),
					Kind:       KindPR,
					Title:      fmt.Sprintf("[PR] %s #%d: %s", repo, it.Number, it.Title),
					URL:        it.HTMLURL,
					Actor:      it.User.Login,
					Repo:       repo,
					GroupKey:   repo,
					Metadata:   map[string]any{"type": "external_pr"},
					OccurredAt: parseGHTime(it.CreatedAt),
				})
			}
		}
	}
	g.rateSleep(ctx)

	// Comments by external users.
	comments, err := g.ghGet(ctx, fmt.Sprintf("/repos/%s/issues/comments?sort=created&since=%s&per_page=50", repo, sinceStr))
	if err == nil {
		var items []struct {
			ID        int    `json:"id"`
			Body      string `json:"body"`
			HTMLURL   string `json:"html_url"`
			User      ghUser `json:"user"`
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(comments, &items) == nil {
			for _, it := range items {
				if g.isSelfOrBot(it.User.Login) {
					continue
				}
				body := it.Body
				if len(body) > 200 {
					body = body[:200] + "..."
				}
				events = append(events, &RawEvent{
					Source:     SourceGitHubSignal,
					SourceID:   fmt.Sprintf("signal:comment:%d", it.ID),
					Kind:       KindComment,
					Title:      fmt.Sprintf("[comment] %s by %s", repo, it.User.Login),
					Body:       body,
					URL:        it.HTMLURL,
					Actor:      it.User.Login,
					Repo:       repo,
					GroupKey:   repo,
					Metadata:   map[string]any{"type": "external_comment"},
					OccurredAt: parseGHTime(it.CreatedAt),
				})
			}
		}
	}
	g.rateSleep(ctx)

	return events, nil
}

// --- Metrics ---

type repoMetricsSnapshot struct {
	Stars    int `json:"stars"`
	Forks    int `json:"forks"`
	Watchers int `json:"watchers"`
}

func (g *GitHubSignalPoller) pollMetrics(ctx context.Context) ([]*RawEvent, error) {
	extra, err := g.state.GetExtra(ctx, SourceGitHubSignal)
	if err != nil {
		extra = map[string]any{}
	}

	var events []*RawEvent
	for _, repo := range g.repos {
		if ctx.Err() != nil {
			break
		}
		data, err := g.ghGet(ctx, fmt.Sprintf("/repos/%s", repo))
		if err != nil {
			g.rateSleep(ctx)
			continue
		}
		var info struct {
			Stars    int `json:"stargazers_count"`
			Forks    int `json:"forks_count"`
			Watchers int `json:"subscribers_count"`
		}
		if json.Unmarshal(data, &info) != nil {
			g.rateSleep(ctx)
			continue
		}

		current := repoMetricsSnapshot{Stars: info.Stars, Forks: info.Forks, Watchers: info.Watchers}

		// Load previous snapshot.
		key := "metrics:" + repo
		var prev repoMetricsSnapshot
		if raw, ok := extra[key]; ok {
			if b, err := json.Marshal(raw); err == nil {
				json.Unmarshal(b, &prev)
			}
		}

		// Compute deltas.
		dStars := current.Stars - prev.Stars
		dForks := current.Forks - prev.Forks
		dWatch := current.Watchers - prev.Watchers

		if prev.Stars > 0 && (dStars > 0 || dForks > 0 || dWatch > 0) {
			var parts []string
			if dStars > 0 {
				parts = append(parts, fmt.Sprintf("+%d stars", dStars))
			}
			if dForks > 0 {
				parts = append(parts, fmt.Sprintf("+%d forks", dForks))
			}
			if dWatch > 0 {
				parts = append(parts, fmt.Sprintf("+%d watchers", dWatch))
			}
			date := time.Now().UTC().Format("2006-01-02")
			events = append(events, &RawEvent{
				Source:     SourceGitHubSignal,
				SourceID:   fmt.Sprintf("signal:metrics:%s:%s", repo, date),
				Kind:       KindMetrics,
				Title:      fmt.Sprintf("%s: %s", repo, strings.Join(parts, ", ")),
				Repo:       repo,
				GroupKey:   repo,
				Metadata:   map[string]any{"stars": current.Stars, "forks": current.Forks, "watchers": current.Watchers, "dStars": dStars, "dForks": dForks},
				OccurredAt: time.Now().UTC(),
			})
		}

		// Save snapshot.
		extra[key] = current
		g.rateSleep(ctx)
	}

	if err := g.state.SetExtra(ctx, SourceGitHubSignal, extra); err != nil {
		g.logger.Warn("github_signal: failed to save metrics snapshots", "error", err)
	}
	return events, nil
}

// --- Stargazers ---

func (g *GitHubSignalPoller) pollStargazers(ctx context.Context) ([]*RawEvent, error) {
	extra, err := g.state.GetExtra(ctx, SourceGitHubSignal)
	if err != nil {
		extra = map[string]any{}
	}

	var events []*RawEvent
	for _, repo := range g.repos {
		if ctx.Err() != nil {
			break
		}
		data, err := g.ghGetWithAccept(ctx, fmt.Sprintf("/repos/%s/stargazers?per_page=100", repo), "application/vnd.github.star+json")
		if err != nil {
			g.rateSleep(ctx)
			continue
		}
		var stargazers []struct {
			User      ghUser `json:"user"`
			StarredAt string `json:"starred_at"`
		}
		if json.Unmarshal(data, &stargazers) != nil {
			g.rateSleep(ctx)
			continue
		}

		// Load previous stargazer list.
		key := "stargazers:" + repo
		prevSet := loadStringSet(extra, key)
		currentSet := make(map[string]bool, len(stargazers))
		for _, s := range stargazers {
			currentSet[s.User.Login] = true
		}

		// Find new stargazers.
		for _, s := range stargazers {
			if !prevSet[s.User.Login] {
				events = append(events, &RawEvent{
					Source:     SourceGitHubSignal,
					SourceID:   fmt.Sprintf("signal:star:%s:%s", repo, s.User.Login),
					Kind:       KindStar,
					Title:      fmt.Sprintf("%s starred %s", s.User.Login, repo),
					Actor:      s.User.Login,
					Repo:       repo,
					GroupKey:   repo,
					Metadata:   map[string]any{"type": "new_star"},
					OccurredAt: parseGHTime(s.StarredAt),
				})
			}
		}

		// Save current list.
		extra[key] = setToSlice(currentSet)
		g.rateSleep(ctx)
	}

	if err := g.state.SetExtra(ctx, SourceGitHubSignal, extra); err != nil {
		g.logger.Warn("github_signal: failed to save stargazers", "error", err)
	}
	return events, nil
}

// --- Followers ---

func (g *GitHubSignalPoller) pollFollowers(ctx context.Context) ([]*RawEvent, error) {
	if g.owner == "" {
		return nil, nil
	}
	data, err := g.ghGet(ctx, fmt.Sprintf("/users/%s/followers?per_page=100", g.owner))
	if err != nil {
		return nil, err
	}
	var followers []ghUser
	if err := json.Unmarshal(data, &followers); err != nil {
		return nil, err
	}

	extra, err := g.state.GetExtra(ctx, SourceGitHubSignal)
	if err != nil {
		extra = map[string]any{}
	}

	prevSet := loadStringSet(extra, "followers")
	currentSet := make(map[string]bool, len(followers))
	for _, f := range followers {
		currentSet[f.Login] = true
	}

	var events []*RawEvent
	for _, f := range followers {
		if !prevSet[f.Login] {
			events = append(events, &RawEvent{
				Source:     SourceGitHubSignal,
				SourceID:   fmt.Sprintf("signal:follow:%s", f.Login),
				Kind:       KindFollow,
				Title:      fmt.Sprintf("%s followed you", f.Login),
				Actor:      f.Login,
				URL:        fmt.Sprintf("https://github.com/%s", f.Login),
				Metadata:   map[string]any{"type": "new_follower"},
				OccurredAt: time.Now().UTC(),
			})
		}
	}

	extra["followers"] = setToSlice(currentSet)
	if err := g.state.SetExtra(ctx, SourceGitHubSignal, extra); err != nil {
		g.logger.Warn("github_signal: failed to save followers", "error", err)
	}
	return events, nil
}

// --- New Repos ---

func (g *GitHubSignalPoller) pollNewRepos(ctx context.Context) ([]*RawEvent, error) {
	repos, err := g.discoverRepos(ctx)
	if err != nil {
		return nil, err
	}

	extra, err := g.state.GetExtra(ctx, SourceGitHubSignal)
	if err != nil {
		extra = map[string]any{}
	}

	prevSet := loadStringSet(extra, "repos")
	currentSet := make(map[string]bool, len(repos))
	for _, r := range repos {
		currentSet[r] = true
	}

	var events []*RawEvent
	for _, r := range repos {
		if !prevSet[r] && len(prevSet) > 0 {
			events = append(events, &RawEvent{
				Source:     SourceGitHubSignal,
				SourceID:   fmt.Sprintf("signal:newrepo:%s", r),
				Kind:       KindNewRepo,
				Title:      fmt.Sprintf("New repo created: %s", r),
				URL:        fmt.Sprintf("https://github.com/%s", r),
				Repo:       r,
				Metadata:   map[string]any{"type": "new_repo"},
				OccurredAt: time.Now().UTC(),
			})
		}
	}

	extra["repos"] = setToSlice(currentSet)
	g.repos = repos
	if err := g.state.SetExtra(ctx, SourceGitHubSignal, extra); err != nil {
		g.logger.Warn("github_signal: failed to save repos", "error", err)
	}
	return events, nil
}

// --- Repo Discovery ---

func (g *GitHubSignalPoller) discoverRepos(ctx context.Context) ([]string, error) {
	var repos []string
	seen := map[string]bool{}

	// User repos.
	data, err := g.ghGet(ctx, "/user/repos?per_page=100&type=owner&sort=updated")
	if err == nil {
		var items []struct {
			FullName string `json:"full_name"`
			Fork     bool   `json:"fork"`
		}
		if json.Unmarshal(data, &items) == nil {
			for _, it := range items {
				if !it.Fork && !seen[it.FullName] {
					repos = append(repos, it.FullName)
					seen[it.FullName] = true
				}
			}
		}
	}
	g.rateSleep(ctx)

	// Org repos.
	for _, org := range g.orgs {
		data, err := g.ghGet(ctx, fmt.Sprintf("/orgs/%s/repos?per_page=100&sort=updated", org))
		if err == nil {
			var items []struct {
				FullName string `json:"full_name"`
				Fork     bool   `json:"fork"`
			}
			if json.Unmarshal(data, &items) == nil {
				for _, it := range items {
					if !it.Fork && !seen[it.FullName] {
						repos = append(repos, it.FullName)
						seen[it.FullName] = true
					}
				}
			}
		}
		g.rateSleep(ctx)
	}

	return repos, nil
}

// --- HTTP helpers ---

type ghUser struct {
	Login string `json:"login"`
}

func (g *GitHubSignalPoller) ghGet(ctx context.Context, path string) ([]byte, error) {
	return g.ghGetWithAccept(ctx, path, "application/vnd.github+json")
}

func (g *GitHubSignalPoller) ghGetWithAccept(ctx context.Context, path string, accept string) ([]byte, error) {
	url := "https://api.github.com" + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check rate limit.
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil && n < 200 {
			g.logger.Warn("github_signal: rate limit low", "remaining", n)
		}
	}

	if resp.StatusCode == http.StatusNotModified {
		return []byte("[]"), nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github_signal: %s returned %d", path, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (g *GitHubSignalPoller) isSelfOrBot(login string) bool {
	if strings.EqualFold(login, g.owner) {
		return true
	}
	return IsBot(login, g.bots)
}

func (g *GitHubSignalPoller) rateSleep(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
}

func parseGHTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

// --- State helpers ---

func loadStringSet(extra map[string]any, key string) map[string]bool {
	set := map[string]bool{}
	raw, ok := extra[key]
	if !ok {
		return set
	}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				set[s] = true
			}
		}
	case []string:
		for _, s := range v {
			set[s] = true
		}
	}
	return set
}

func setToSlice(set map[string]bool) []string {
	s := make([]string, 0, len(set))
	for k := range set {
		s = append(s, k)
	}
	return s
}
