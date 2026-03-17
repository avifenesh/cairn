package llm

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

// RetryConfig controls retry behavior.
type RetryConfig struct {
	MaxRetries      int           // default: 3
	BaseBackoff     time.Duration // default: 1s
	MaxBackoff      time.Duration // default: 30s
	JitterFraction  float64       // default: 0.2
	RetryableStatus []int         // HTTP status codes to retry on (429, 500, 502, 503)
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		BaseBackoff:     time.Second,
		MaxBackoff:      30 * time.Second,
		JitterFraction:  0.2,
		RetryableStatus: []int{429, 500, 502, 503},
	}
}

// RetryOption configures the retry provider.
type RetryOption func(*retryProvider)

// WithFallback sets a fallback provider to try after exhausting retries on the primary.
func WithFallback(p Provider) RetryOption {
	return func(rp *retryProvider) {
		rp.fallback = p
	}
}

// WithLogger sets a logger for the retry provider.
func WithLogger(l *slog.Logger) RetryOption {
	return func(rp *retryProvider) {
		rp.logger = l
	}
}

// retryProvider wraps a primary provider with retry + optional fallback.
type retryProvider struct {
	primary  Provider
	fallback Provider
	config   RetryConfig
	logger   *slog.Logger
}

// WithRetry wraps a provider with retry logic and optional fallback.
func WithRetry(primary Provider, config RetryConfig, opts ...RetryOption) Provider {
	rp := &retryProvider{
		primary: primary,
		config:  config,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(rp)
	}
	return rp
}

func (rp *retryProvider) ID() string {
	return rp.primary.ID() + "+retry"
}

func (rp *retryProvider) Models() []ModelInfo {
	models := rp.primary.Models()
	if rp.fallback != nil {
		models = append(models, rp.fallback.Models()...)
	}
	return models
}

// Stream attempts streaming from the primary provider with retries,
// falling back to the secondary provider if all retries are exhausted.
func (rp *retryProvider) Stream(ctx context.Context, req *Request) (<-chan Event, error) {
	ch := make(chan Event, 32)

	go func() {
		defer close(ch)

		var lastErr error

		// Try primary provider with retries.
		for attempt := 0; attempt <= rp.config.MaxRetries; attempt++ {
			if attempt > 0 {
				backoff := rp.calcBackoff(attempt - 1)
				rp.logger.Info("llm: retrying",
					"provider", rp.primary.ID(),
					"attempt", attempt,
					"backoff", backoff,
				)

				select {
				case <-ctx.Done():
					sendEvent(ctx, ch, StreamError{Err: ctx.Err(), Retryable: false})
					return
				case <-time.After(backoff):
				}
			}

			events, err := rp.primary.Stream(ctx, req)
			if err != nil {
				lastErr = err
				rp.logger.Warn("llm: stream initiation failed",
					"provider", rp.primary.ID(),
					"attempt", attempt,
					"error", err,
				)
				continue
			}

			// Forward events. If we hit a retryable StreamError, break and retry.
			retryable := rp.forwardEvents(ctx, ch, events)
			if !retryable {
				return // success or non-retryable error
			}
			lastErr = fmt.Errorf("retryable stream error on attempt %d", attempt)
		}

		// Exhausted retries on primary. Try fallback if available.
		if rp.fallback != nil {
			rp.logger.Info("llm: falling back",
				"primary", rp.primary.ID(),
				"fallback", rp.fallback.ID(),
			)

			events, err := rp.fallback.Stream(ctx, req)
			if err != nil {
				sendEvent(ctx, ch, StreamError{
					Err:       fmt.Errorf("fallback failed: %w (primary last error: %v)", err, lastErr),
					Retryable: false,
				})
				return
			}

			rp.forwardEvents(ctx, ch, events)
			return
		}

		// No fallback — emit final error.
		sendEvent(ctx, ch, StreamError{
			Err:       fmt.Errorf("all retries exhausted: %w", lastErr),
			Retryable: false,
		})
	}()

	return ch, nil
}

// forwardEvents forwards events from a provider channel to the output channel.
// Returns true if a retryable StreamError was encountered (caller should retry),
// false on normal completion or non-retryable error.
func (rp *retryProvider) forwardEvents(ctx context.Context, out chan<- Event, in <-chan Event) bool {
	for ev := range in {
		if se, ok := ev.(StreamError); ok && se.Retryable {
			rp.logger.Warn("llm: retryable stream error",
				"provider", rp.primary.ID(),
				"error", se.Err,
			)
			// Drain remaining events from the channel.
			for range in {
			}
			return true
		}

		select {
		case out <- ev:
		case <-ctx.Done():
			return false
		}
	}
	return false
}

// calcBackoff computes exponential backoff with jitter.
// backoff = min(base * 2^attempt, max) * (1 + jitter * rand)
func (rp *retryProvider) calcBackoff(attempt int) time.Duration {
	base := float64(rp.config.BaseBackoff)
	backoff := base * math.Pow(2, float64(attempt))
	maxBackoff := float64(rp.config.MaxBackoff)
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	jitter := rp.config.JitterFraction * rand.Float64()
	backoff = backoff * (1 + jitter)

	return time.Duration(backoff)
}
