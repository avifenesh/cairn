package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// Loop is the always-on agent loop. It ticks periodically, checks for pending
// tasks, decides on proactive actions, and drives the reflection cycle.
type Loop struct {
	agent     Agent
	tasks     *task.Engine
	events    *signal.EventStore
	memories  *memory.Service
	soul      *memory.Soul
	tools     *tool.Registry
	provider  llm.Provider
	bus       *eventbus.Bus
	journaler *Journaler
	reflector *ReflectionEngine
	logger    *slog.Logger
	config    LoopConfig

	cancel  context.CancelFunc
	stopped atomic.Bool
	wg      sync.WaitGroup

	tickCount   atomic.Int64
	lastReflect time.Time
}

// LoopConfig configures the always-on agent loop.
type LoopConfig struct {
	TickInterval       time.Duration // Default: 60s
	ReflectionInterval time.Duration // Default: 30min
	Model              string
	IdleEnabled        bool
}

// NewLoop creates an always-on agent loop.
func NewLoop(cfg LoopConfig, deps LoopDeps) *Loop {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 60 * time.Second
	}
	if cfg.ReflectionInterval <= 0 {
		cfg.ReflectionInterval = 30 * time.Minute
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Loop{
		agent:     deps.Agent,
		tasks:     deps.Tasks,
		events:    deps.Events,
		memories:  deps.Memories,
		soul:      deps.Soul,
		tools:     deps.Tools,
		provider:  deps.Provider,
		bus:       deps.Bus,
		journaler: deps.Journaler,
		reflector: deps.Reflector,
		logger:    logger,
		config:    cfg,
	}
}

// LoopDeps carries dependencies for the loop.
type LoopDeps struct {
	Agent     Agent
	Tasks     *task.Engine
	Events    *signal.EventStore
	Memories  *memory.Service
	Soul      *memory.Soul
	Tools     *tool.Registry
	Provider  llm.Provider
	Bus       *eventbus.Bus
	Journaler *Journaler
	Reflector *ReflectionEngine
	Logger    *slog.Logger
}

// Start begins the agent loop in a background goroutine. Safe to call only once.
func (l *Loop) Start() {
	if l.stopped.Load() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.wg.Add(1)
	go l.run(ctx)
	l.logger.Info("agent loop started", "tick", l.config.TickInterval, "reflection", l.config.ReflectionInterval)
}

// Close stops the agent loop and waits for the current tick to finish.
func (l *Loop) Close() {
	if l.stopped.CompareAndSwap(false, true) {
		if l.cancel != nil {
			l.cancel()
		}
	}
	l.wg.Wait()
	l.logger.Info("agent loop stopped", "ticks", l.tickCount.Load())
}

// TickCount returns the number of ticks completed.
func (l *Loop) TickCount() int64 {
	return l.tickCount.Load()
}

func (l *Loop) run(ctx context.Context) {
	defer l.wg.Done()

	// Tick immediately on startup.
	l.tick(ctx)

	ticker := time.NewTicker(l.config.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.tick(ctx)
		}
	}
}

func (l *Loop) tick(ctx context.Context) {
	l.tickCount.Add(1)
	start := time.Now()

	// 1. Check for pending tasks and execute the highest priority one.
	executed := l.executePendingTask(ctx)

	// 2. Run reflection if interval elapsed.
	if time.Since(l.lastReflect) >= l.config.ReflectionInterval && l.reflector != nil {
		l.runReflection(ctx)
		l.lastReflect = time.Now()
	}

	// 3. Publish heartbeat.
	if l.bus != nil {
		eventbus.Publish(l.bus, AgentHeartbeat{
			EventMeta:  eventbus.NewMeta("agent"),
			TickNumber: l.tickCount.Load(),
			TaskRun:    executed,
			DurationMs: time.Since(start).Milliseconds(),
		})
	}
}

func (l *Loop) executePendingTask(ctx context.Context) bool {
	if l.tasks == nil || l.agent == nil {
		return false
	}

	// Try to claim any pending task.
	t, err := l.tasks.Claim(ctx, "")
	if err != nil || t == nil {
		return false
	}

	l.logger.Info("agent loop: executing task", "task", t.ID, "type", t.Type, "description", t.Description)

	sessionID := "loop-" + t.ID
	session := &Session{
		ID:    sessionID,
		Mode:  tool.ModeWork,
		State: map[string]any{"taskId": t.ID},
	}

	invCtx := &InvocationContext{
		Context:     ctx,
		SessionID:   sessionID,
		UserMessage: t.Description,
		Mode:        tool.ModeWork,
		Session:     session,
		Tools:       l.tools,
		LLM:         l.provider,
		Memory:      l.memories,
		Soul:        l.soul,
		Bus:         l.bus,
		Config:      &AgentConfig{Model: l.config.Model, MaxRounds: 10},
	}

	// Run agent, collect assistant response only (skip user events).
	var response strings.Builder
	taskStart := time.Now()
	for ev := range l.agent.Run(invCtx) {
		if ev.Err != nil {
			l.logger.Error("agent loop: task error", "task", t.ID, "error", ev.Err)
			if err := l.tasks.Fail(ctx, t.ID, ev.Err); err != nil {
				l.logger.Warn("agent loop: fail task error", "task", t.ID, "error", err)
			}
			return true
		}
		if ev.Event != nil {
			session.Events = append(session.Events, ev.Event)
			if ev.Event.Author != "user" {
				for _, part := range ev.Event.Parts {
					if tp, ok := part.(TextPart); ok {
						response.WriteString(tp.Text)
					}
				}
			}
		}
	}

	outputJSON, err := json.Marshal(response.String())
	if err != nil {
		l.logger.Error("agent loop: marshal output", "task", t.ID, "error", err)
		if fErr := l.tasks.Fail(ctx, t.ID, err); fErr != nil {
			l.logger.Warn("agent loop: fail task error", "task", t.ID, "error", fErr)
		}
		return true
	}
	if err := l.tasks.Complete(ctx, t.ID, json.RawMessage(outputJSON)); err != nil {
		l.logger.Warn("agent loop: complete task error", "task", t.ID, "error", err)
	}
	l.logger.Info("agent loop: task completed", "task", t.ID, "duration", time.Since(taskStart))

	// Journal the session.
	if l.journaler != nil {
		l.journaler.Record(ctx, session, time.Since(taskStart))
	}

	return true
}

func (l *Loop) runReflection(ctx context.Context) {
	result, err := l.reflector.Reflect(ctx)
	if err != nil {
		l.logger.Warn("agent loop: reflection failed", "error", err)
		return
	}

	if len(result.Memories) == 0 && result.SoulPatch == "" {
		return
	}

	l.logger.Info("agent loop: reflection complete",
		"memories", len(result.Memories),
		"soulPatch", result.SoulPatch != "")

	if err := l.reflector.Apply(ctx, result); err != nil {
		l.logger.Warn("agent loop: reflection apply failed", "error", err)
	}
}

// AgentHeartbeat is emitted every tick via the event bus.
type AgentHeartbeat struct {
	eventbus.EventMeta
	TickNumber int64 `json:"tickNumber"`
	TaskRun    bool  `json:"taskRun"`
	DurationMs int64 `json:"durationMs"`
}
