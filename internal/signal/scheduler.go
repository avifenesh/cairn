package signal

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
)

// Scheduler runs pollers at their configured intervals, ingests results,
// and publishes bus events for new items. Pollers execute sequentially
// within each tick to avoid thundering herd on startup.
type Scheduler struct {
	store   *EventStore
	state   *SourceState
	bus     *eventbus.Bus
	logger  *slog.Logger
	pollers []pollerEntry
	done    chan struct{}
	stopped atomic.Bool
	wg      sync.WaitGroup
}

type pollerEntry struct {
	poller   Poller
	interval time.Duration
	backoff  time.Duration // current backoff (reset on success)
}

const (
	defaultInterval = 5 * time.Minute
	maxBackoff      = 30 * time.Minute
	initialBackoff  = 30 * time.Second
)

// NewScheduler creates a scheduler that will poll sources and ingest into the store.
func NewScheduler(store *EventStore, state *SourceState, bus *eventbus.Bus, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		store:  store,
		state:  state,
		bus:    bus,
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Register adds a poller with a custom interval. Use 0 for default (5min).
func (s *Scheduler) Register(p Poller, interval time.Duration) {
	if interval <= 0 {
		interval = defaultInterval
	}
	s.pollers = append(s.pollers, pollerEntry{
		poller:   p,
		interval: interval,
	})
}

// Start begins polling in background goroutines. Each poller gets its own
// goroutine with its own ticker.
func (s *Scheduler) Start() {
	for i := range s.pollers {
		s.wg.Add(1)
		go s.runPoller(i)
	}
	s.logger.Info("signal scheduler started", "pollers", len(s.pollers))
}

// Close stops all pollers and waits for them to finish.
func (s *Scheduler) Close() {
	if s.stopped.CompareAndSwap(false, true) {
		close(s.done)
	}
	s.wg.Wait()
	s.logger.Info("signal scheduler stopped")
}

// PollNow runs a single poll cycle for all registered pollers synchronously.
// Useful for testing and manual triggers.
func (s *Scheduler) PollNow(ctx context.Context) {
	for i := range s.pollers {
		s.pollOnce(ctx, i)
	}
}

func (s *Scheduler) runPoller(idx int) {
	defer s.wg.Done()

	entry := &s.pollers[idx]

	// Poll immediately on startup.
	ctx := context.Background()
	s.pollOnce(ctx, idx)

	ticker := time.NewTicker(entry.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if entry.backoff > 0 {
				// In backoff - use backoff duration instead of normal interval.
				ticker.Reset(entry.backoff)
			} else {
				ticker.Reset(entry.interval)
			}
			s.pollOnce(context.Background(), idx)
		}
	}
}

func (s *Scheduler) pollOnce(ctx context.Context, idx int) {
	entry := &s.pollers[idx]
	source := entry.poller.Source()

	since, err := s.state.GetLastPoll(ctx, source)
	if err != nil {
		s.logger.Warn("signal: failed to get last poll time", "source", source, "error", err)
		since = time.Now().UTC().Add(-24 * time.Hour) // default: last 24h
	}
	if since.IsZero() {
		since = time.Now().UTC().Add(-24 * time.Hour)
	}

	events, err := entry.poller.Poll(ctx, since)
	if err != nil {
		// Apply exponential backoff.
		if entry.backoff == 0 {
			entry.backoff = initialBackoff
		} else {
			entry.backoff *= 2
			if entry.backoff > maxBackoff {
				entry.backoff = maxBackoff
			}
		}
		s.logger.Warn("signal: poll failed", "source", source, "error", err, "backoff", entry.backoff)
		return
	}

	// Reset backoff on success.
	entry.backoff = 0

	if len(events) == 0 {
		s.state.SetLastPoll(ctx, source, time.Now().UTC())
		return
	}

	inserted, err := s.store.Ingest(ctx, events)
	if err != nil {
		s.logger.Error("signal: ingest failed", "source", source, "error", err)
		return
	}

	s.state.SetLastPoll(ctx, source, time.Now().UTC())

	if inserted > 0 {
		s.logger.Info("signal: ingested events", "source", source, "new", inserted, "total", len(events))

		// Publish bus events for each new event so SSE clients get notified.
		if s.bus != nil {
			for _, ev := range events {
				eventbus.Publish(s.bus, eventbus.EventIngested{
					EventMeta:  eventbus.NewMeta(source),
					SourceType: ev.Source,
					Title:      ev.Title,
					URL:        ev.URL,
				})
			}
		}
	}
}
