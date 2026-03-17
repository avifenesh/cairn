package eventbus

import (
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
)

const defaultQueueSize = 4096

// Bus is a typed async pub/sub event bus using Go generics.
// Type-safe routing via reflect.Type, synchronous + asynchronous delivery,
// and backpressure on the async queue.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[reflect.Type][]subscriber
	asyncQueue  chan asyncItem
	closed      chan struct{}
	logger      *slog.Logger
	nextID      atomic.Uint64
}

type subscriber struct {
	id      uint64
	handler reflect.Value // func(E)
}

type asyncItem struct {
	eventType reflect.Type
	event     reflect.Value
}

// Option configures a Bus.
type Option func(*busConfig)

type busConfig struct {
	logger    *slog.Logger
	queueSize int
}

// WithLogger sets the logger for the bus.
func WithLogger(l *slog.Logger) Option {
	return func(c *busConfig) {
		c.logger = l
	}
}

// WithQueueSize sets the async queue capacity.
func WithQueueSize(size int) Option {
	return func(c *busConfig) {
		if size > 0 {
			c.queueSize = size
		}
	}
}

// New creates a Bus and starts its async worker goroutine.
func New(opts ...Option) *Bus {
	cfg := &busConfig{
		logger:    slog.Default(),
		queueSize: defaultQueueSize,
	}
	for _, o := range opts {
		o(cfg)
	}

	b := &Bus{
		subscribers: make(map[reflect.Type][]subscriber),
		asyncQueue:  make(chan asyncItem, cfg.queueSize),
		closed:      make(chan struct{}),
		logger:      cfg.logger,
	}

	go b.asyncWorker()
	return b
}

// Subscribe registers a handler for events of type E.
// Returns an unsubscribe function that removes the handler.
func Subscribe[E any](bus *Bus, handler func(E)) func() {
	t := reflect.TypeFor[E]()
	id := bus.nextID.Add(1)

	sub := subscriber{
		id:      id,
		handler: reflect.ValueOf(handler),
	}

	bus.mu.Lock()
	bus.subscribers[t] = append(bus.subscribers[t], sub)
	bus.mu.Unlock()

	// Return unsubscribe closure.
	return func() {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		subs := bus.subscribers[t]
		for i, s := range subs {
			if s.id == id {
				bus.subscribers[t] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}
}

// Publish delivers event synchronously to all subscribers of type E.
// Panics in individual handlers are recovered; other handlers still execute.
func Publish[E any](bus *Bus, event E) {
	t := reflect.TypeFor[E]()
	val := reflect.ValueOf(event)

	bus.mu.RLock()
	// Copy the slice so we can release the lock before calling handlers.
	subs := make([]subscriber, len(bus.subscribers[t]))
	copy(subs, bus.subscribers[t])
	bus.mu.RUnlock()

	for _, s := range subs {
		bus.callHandler(s, t, val)
	}
}

// PublishAsync enqueues event for asynchronous delivery.
// Blocks if the queue is full (backpressure).
func PublishAsync[E any](bus *Bus, event E) {
	t := reflect.TypeFor[E]()
	val := reflect.ValueOf(event)

	select {
	case <-bus.closed:
		bus.logger.Warn("PublishAsync called on closed bus", "eventType", t.String())
		return
	default:
	}

	bus.asyncQueue <- asyncItem{eventType: t, event: val}
}

// PublishStream returns a write-only channel. Events sent to it are
// distributed to all subscribers of type E. Close the channel when done.
// Used for high-throughput event streams (e.g. LLM deltas → SSE broadcast).
func PublishStream[E any](bus *Bus) chan<- E {
	ch := make(chan E, 64)

	go func() {
		for event := range ch {
			Publish(bus, event)
		}
	}()

	return ch
}

// Close shuts down the async worker, draining remaining items.
func (b *Bus) Close() {
	select {
	case <-b.closed:
		return // already closed
	default:
	}

	close(b.asyncQueue)
	<-b.closed // wait for worker to finish draining
}

// asyncWorker processes items from the async queue until the channel is closed,
// then drains any remaining items.
func (b *Bus) asyncWorker() {
	defer close(b.closed)

	for item := range b.asyncQueue {
		b.deliverAsync(item)
	}
}

func (b *Bus) deliverAsync(item asyncItem) {
	b.mu.RLock()
	subs := make([]subscriber, len(b.subscribers[item.eventType]))
	copy(subs, b.subscribers[item.eventType])
	b.mu.RUnlock()

	for _, s := range subs {
		b.callHandler(s, item.eventType, item.event)
	}
}

// callHandler invokes a single subscriber handler, recovering from panics.
func (b *Bus) callHandler(s subscriber, t reflect.Type, val reflect.Value) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("panic in event handler",
				"eventType", t.String(),
				"subscriberID", s.id,
				"panic", r,
			)
		}
	}()

	s.handler.Call([]reflect.Value{val})
}
