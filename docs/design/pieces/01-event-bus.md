# Piece 1: Event Bus

> Typed async pub/sub backbone connecting all modules.

## Purpose

Every module communicates through events — signal ingestion, agent decisions, tool results, task state changes, memory updates. The event bus is the nervous system.

## Interface

```go
// Core contract — borrowed from Gollem's typed event bus
type Bus struct { /* internal: sync.RWMutex, subscribers map[reflect.Type][]handler, queue chan event */ }

func Subscribe[E any](bus *Bus, handler func(E)) (unsubscribe func())
func Publish[E any](bus *Bus, event E)        // synchronous delivery
func PublishAsync[E any](bus *Bus, event E)    // queued, non-blocking
func PublishStream[E any](bus *Bus) chan<- E    // returns write-only channel for high-throughput events
```

## Event Categories

| Category | Events | Producer | Consumer |
|----------|--------|----------|----------|
| Signal | `EventIngested`, `EventRead`, `EventArchived` | Signal Plane | Agent Core, Server |
| Agent | `AgentTickStarted`, `AgentDecision`, `AgentError` | Agent Core | Server (SSE), Memory |
| LLM | `StreamStarted`, `TextDelta`, `ToolCall`, `StreamEnded` | LLM Client | Agent Core, Server |
| Task | `TaskCreated`, `TaskRunning`, `TaskCompleted`, `TaskFailed` | Task Engine | Agent Core, Server |
| Memory | `MemoryProposed`, `MemoryAccepted`, `MemoryRejected` | Memory System | Agent Core |
| Tool | `ToolExecuting`, `ToolCompleted`, `ToolError` | Tool System | Agent Core, Server |
| System | `HealthCheck`, `ConfigReloaded`, `ShutdownInitiated` | Server | All |

## Key Design Decisions

1. **Type-safe via generics** — no `interface{}` events. Subscribers register by concrete type.
2. **Async by default** — `PublishAsync` for non-critical events (logging, metrics). `Publish` for critical path (tool results feeding back to agent loop).
3. **Backpressure** — async queue has bounded capacity. If consumer is slow, publisher blocks (not drops).
4. **No persistence** — the bus is in-memory. Persistence is handled by the SQLite store subscribing to events.

## Subphases

| # | Subphase | Description | Depends On |
|---|----------|-------------|------------|
| 1.1 | Core bus implementation | Subscribe, Publish, PublishAsync with generics | Nothing |
| 1.2 | Event type definitions | All event structs across categories | Nothing |
| 1.3 | Stream channels | PublishStream for high-throughput SSE fan-out | 1.1 |
| 1.4 | Bus middleware | Logging, metrics, filtering hooks | 1.1 |
| 1.5 | Tests | Unit tests with race detector | 1.1-1.4 |

## Tasks

### 1.1 Core bus implementation
- [x] Define `Bus` struct with `sync.RWMutex` + subscriber map
- [x] Implement `Subscribe[E]` with reflect.Type routing
- [x] Implement `Publish[E]` synchronous delivery
- [x] Implement `PublishAsync[E]` with buffered channel + worker goroutine
- [x] Implement unsubscribe via returned closure
- [x] Handle panic recovery in subscriber callbacks

### 1.2 Event type definitions
- [x] Define all event structs per category (see table above)
- [x] Ensure every event has `ID string`, `Timestamp time.Time`, `Source string`
- [x] Add JSON serialization tags for SSE emission

### 1.3 Stream channels
- [x] `PublishStream[E]` returns write channel, bus distributes to all `E` subscribers
- [x] Used for LLM streaming deltas → SSE broadcast

### 1.4 Bus middleware
- [x] `WithLogger(logger)` — log all events at debug level
- [ ] `WithMetrics(counter)` — count events by type (deferred - no consumer yet)
- [ ] `WithFilter(predicate)` — drop events matching condition (deferred - no consumer yet)

### 1.5 Tests
- [x] Concurrent publish/subscribe with `-race`
- [x] Unsubscribe during active publishing
- [x] Backpressure behavior when queue full
- [x] Type safety — wrong type subscriber never called
