# Piece 7: Signal Plane

> Source polling, webhooks, event ingestion, dedup, digest generation.

## Architecture

```go
type SignalPlane struct {
    pollers   map[string]Poller
    webhooks  *WebhookHandler
    store     EventStore
    bus       *eventbus.Bus
    deduper   *Deduplicator
    scheduler *PollScheduler
}

type Poller interface {
    Source() string
    Poll(ctx context.Context, since time.Time) ([]*RawEvent, error)
}

type RawEvent struct {
    Source      string
    SourceID    string    // dedup key
    Kind        string    // pr, issue, comment, email, post, etc.
    Title       string
    Body        string
    URL         string
    Actor       string
    Repo        string
    Metadata    map[string]any
    OccurredAt  time.Time
}
```

## Pollers (Phase 1)

| Source | API | Interval | Notes |
|--------|-----|----------|-------|
| GitHub | REST + GraphQL | 5min | Org tracking, PR comments, issues, releases |
| Gmail | Google API | 5min | Push via webhook when available |
| Reddit | Reddit API | 5min | Subreddit monitoring |
| HN | Firebase API | 5min | Keyword + score filtering |
| npm | Registry API | 15min | Package version tracking |
| crates.io | API | 15min | Rust package tracking |
| Webhooks | HTTP POST | realtime | GitHub, Stripe, custom |

## Digest System

```go
type DigestRunner struct {
    llm      llm.Client
    store    EventStore
    bus      *eventbus.Bus
    interval time.Duration // default: 3h
}

// Groups unread events by source/entity, priority-ranks, generates summary
func (d *DigestRunner) Generate(ctx context.Context) (*Digest, error)

type Digest struct {
    Summary     string
    Highlights  []string
    Groups      []DigestGroup
    Period      TimeRange
    EventCount  int
}
```

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 7.1 | Event store (SQLite) + dedup | Nothing |
| 7.2 | Poll scheduler (per-source intervals, backoff) | 7.1 |
| 7.3 | GitHub poller | 7.1, 7.2 |
| 7.4 | Gmail poller | 7.1, 7.2 |
| 7.5 | Generic pollers (Reddit, HN, npm, crates) | 7.1, 7.2 |
| 7.6 | Webhook handler (HTTP POST receiver) | 7.1 |
| 7.7 | Digest runner | 7.1, 2 (LLM) |
| 7.8 | Tests | All |
