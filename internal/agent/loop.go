package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avifenesh/cairn/internal/agenttype"
	cairncron "github.com/avifenesh/cairn/internal/cron"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/rules"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
	"github.com/avifenesh/cairn/internal/tool/builtin"
)

// Loop is the always-on agent loop. It ticks periodically, checks for pending
// tasks, decides on proactive actions, and drives the reflection cycle.
type Loop struct {
	agent        Agent
	tasks        *task.Engine
	events       *signal.EventStore
	memories     *memory.Service
	soul         *memory.Soul
	tools        *tool.Registry
	provider     llm.Provider
	bus          *eventbus.Bus
	journaler    *Journaler
	extractor    *memory.Extractor
	reflector    *ReflectionEngine
	logger       *slog.Logger
	config       LoopConfig
	toolMemories tool.MemoryService
	toolEvents   tool.EventService
	toolDigest   tool.DigestService
	toolJournal  tool.JournalService
	toolTasks    tool.TaskService
	toolStatus   tool.StatusService
	toolSkills   tool.SkillService
	toolNotifier tool.NotifyService
	toolCrons    tool.CronService
	toolRules    tool.RulesService
	toolConfig   tool.ConfigService
	toolIdentity tool.IdentityService

	contextBuilder *memory.ContextBuilder // token-budgeted context (nil = fallback)
	plugins        *plugin.Manager        // lifecycle hooks (nil = no plugins)

	cronStore       *cairncron.Store         // nil = cron disabled
	activityStore   *ActivityStore           // nil = activity tracking disabled
	sessions        *SessionStore            // nil = loop events not persisted (lost on crash)
	checkpoints     *CheckpointStore         // nil = no session checkpointing
	db              *sql.DB                  // for state checkpoint
	orchestrator    *Orchestrator            // management layer (replaces idleTick)
	subagentRunner  tool.SubagentService     // nil = subagent spawning disabled
	worktreeManager *task.WorktreeManager    // nil = no worktree isolation
	notifier        tool.NotifyService       // nil = notifications disabled
	skillSuggestor  *SkillSuggestor          // nil = skill suggestions disabled
	marketplace     *skill.MarketplaceClient // nil = marketplace disabled

	userProfile   *memory.UserProfile  // nil = no user profile enrichment
	agentsFile    *memory.AgentsFile   // nil = no agents file enrichment
	curatedMemory *memory.MarkdownFile // nil = no curated memory enrichment
	agentTypes    *agenttype.Service   // nil = no agent type catalog in prompt

	rulesEngine *rules.Engine // nil = rules engine disabled

	cancel  context.CancelFunc
	stopped atomic.Bool
	wg      sync.WaitGroup

	tickCount     atomic.Int64
	lastReflect   time.Time
	lastRulePrune time.Time

	// Last orchestrator decision — recorded in activity store for UI visibility.
	lastIdleDecision *IdleDecision
}

// LoopConfig configures the always-on agent loop.
type LoopConfig struct {
	TickInterval           time.Duration // Default: 60s
	ReflectionInterval     time.Duration // Default: 30min
	Model                  string
	IdleEnabled            bool
	TalkMaxRounds          int         // Default: 40
	WorkMaxRounds          int         // Default: 80
	CodingMaxRounds        int         // Default: 400
	CodingEnabled          bool        // Whether coding tasks can be submitted from idle loop
	CodingAllowedRepos     []string    // Repo paths where coding is allowed (empty = cwd only)
	MaxConcurrentSubagents int         // Max concurrent subagents (default 5)
	MaxSpawnDepth          int         // Max subagent nesting depth (default 3)
	BriefingModel          string      // Cheap model for context summarization (default: fallback model)
	EnvContext             *EnvContext // Ground-truth environment context for orchestrator
}

func (c LoopConfig) maxRoundsForMode(mode tool.Mode) int {
	switch mode {
	case tool.ModeTalk:
		if c.TalkMaxRounds > 0 {
			return c.TalkMaxRounds
		}
		return 40
	case tool.ModeWork:
		if c.WorkMaxRounds > 0 {
			return c.WorkMaxRounds
		}
		return 80
	case tool.ModeCoding:
		if c.CodingMaxRounds > 0 {
			return c.CodingMaxRounds
		}
		return 400
	default:
		return 40
	}
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
		agent:           deps.Agent,
		tasks:           deps.Tasks,
		events:          deps.Events,
		memories:        deps.Memories,
		soul:            deps.Soul,
		tools:           deps.Tools,
		provider:        deps.Provider,
		bus:             deps.Bus,
		journaler:       deps.Journaler,
		extractor:       deps.Extractor,
		reflector:       deps.Reflector,
		logger:          logger,
		config:          cfg,
		toolMemories:    deps.ToolMemories,
		toolEvents:      deps.ToolEvents,
		toolDigest:      deps.ToolDigest,
		toolJournal:     deps.ToolJournal,
		toolTasks:       deps.ToolTasks,
		toolStatus:      deps.ToolStatus,
		toolSkills:      deps.ToolSkills,
		toolNotifier:    deps.ToolNotifier,
		toolCrons:       deps.ToolCrons,
		toolRules:       deps.ToolRules,
		toolConfig:      deps.ToolConfig,
		toolIdentity:    deps.ToolIdentity,
		contextBuilder:  deps.ContextBuilder,
		plugins:         deps.Plugins,
		cronStore:       deps.CronStore,
		activityStore:   deps.ActivityStore,
		sessions:        deps.Sessions,
		checkpoints:     deps.Checkpoints,
		db:              deps.DB,
		subagentRunner:  deps.SubagentRunner,
		worktreeManager: deps.WorktreeManager,
		notifier:        deps.Notifier,
		marketplace:     deps.Marketplace,
		rulesEngine:     deps.RulesEngine,
		userProfile:     deps.UserProfile,
		agentsFile:      deps.AgentsFile,
		curatedMemory:   deps.CuratedMemory,
		agentTypes:      deps.AgentTypes,
		skillSuggestor:  NewSkillSuggestor(logger),
		orchestrator: NewOrchestrator(OrchestratorDeps{
			Memories:               deps.Memories,
			Tasks:                  deps.Tasks,
			Events:                 deps.Events,
			Soul:                   deps.Soul,
			Approvals:              deps.Approvals,
			SubagentRunner:         deps.SubagentRunner,
			Bus:                    deps.Bus,
			Provider:               deps.Provider,
			Model:                  cfg.Model,
			BriefingModel:          cfg.BriefingModel,
			ActivityStore:          deps.ActivityStore,
			Reflector:              deps.Reflector,
			SkillSuggestor:         NewSkillSuggestor(logger),
			Marketplace:            deps.Marketplace,
			ToolSkills:             deps.ToolSkills,
			Journaler:              deps.Journaler,
			AgentTypes:             deps.AgentTypes,
			Logger:                 logger,
			CodingEnabled:          cfg.CodingEnabled,
			EnvContext:             cfg.EnvContext,
			MaxConcurrentSubagents: cfg.MaxConcurrentSubagents,
		}),
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
	Extractor *memory.Extractor
	Reflector *ReflectionEngine
	Logger    *slog.Logger

	// Tool service adapters for agent tools.
	ToolMemories tool.MemoryService
	ToolEvents   tool.EventService
	ToolDigest   tool.DigestService
	ToolJournal  tool.JournalService
	ToolTasks    tool.TaskService
	ToolStatus   tool.StatusService
	ToolSkills   tool.SkillService
	ToolNotifier tool.NotifyService
	ToolCrons    tool.CronService
	ToolRules    tool.RulesService
	ToolConfig   tool.ConfigService
	ToolIdentity tool.IdentityService

	ContextBuilder *memory.ContextBuilder // optional: token-budgeted context
	Plugins        *plugin.Manager        // optional: lifecycle hooks

	CronStore       *cairncron.Store         // optional: enables cron job checking in tick
	ActivityStore   *ActivityStore           // optional: enables activity recording
	Sessions        *SessionStore            // optional: persists loop task events (enables resume)
	Checkpoints     *CheckpointStore         // optional: session checkpoint for crash recovery
	DB              *sql.DB                  // optional: enables state checkpoint
	SubagentRunner  tool.SubagentService     // optional: enables subagent spawning from tools
	WorktreeManager *task.WorktreeManager    // optional: worktree isolation for coding tasks
	Notifier        tool.NotifyService       // optional: routes notifications to channels
	Marketplace     *skill.MarketplaceClient // optional: ClawHub marketplace for suggestions
	Approvals       *task.ApprovalStore      // optional: human-in-the-loop approvals
	RulesEngine     *rules.Engine            // optional: automation rules engine (for pruning)

	UserProfile   *memory.UserProfile  // optional: USER.md enrichment
	AgentsFile    *memory.AgentsFile   // optional: AGENTS.md enrichment
	CuratedMemory *memory.MarkdownFile // optional: curated long-term memory
	AgentTypes    *agenttype.Service   // optional: AGENT.md type definitions
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

// SkillSuggestor returns the skill suggestion engine (for server API access).
func (l *Loop) SkillSuggestor() *SkillSuggestor { return l.skillSuggestor }

// SetNotifier sets the notification service (called after channels are configured).
func (l *Loop) SetNotifier(n tool.NotifyService) {
	l.notifier = n
	l.toolNotifier = n // also wire to tool context
	if l.orchestrator != nil {
		l.orchestrator.notifier = n
	}
}

// buildInvocationContext creates a complete InvocationContext with all available
// deps. Single source of truth — prevents field divergence across code paths.
func (l *Loop) buildInvocationContext(ctx context.Context, sessionID, userMessage string, mode tool.Mode, session *Session) *InvocationContext {
	return &InvocationContext{
		Context:         ctx,
		SessionID:       sessionID,
		UserMessage:     userMessage,
		Mode:            mode,
		Session:         session,
		Tools:           l.tools,
		LLM:             l.provider,
		Memory:          l.memories,
		Soul:            l.soul,
		UserProfile:     l.userProfile,
		AgentsFile:      l.agentsFile,
		CuratedMemory:   l.curatedMemory,
		AgentTypes:      l.agentTypes,
		Bus:             l.bus,
		ContextBuilder:  l.contextBuilder,
		Plugins:         l.plugins,
		ActivityStore:   l.activityStore,
		Subagents:       l.subagentRunner,
		ToolMemories:    l.toolMemories,
		ToolEvents:      l.toolEvents,
		ToolDigest:      l.toolDigest,
		ToolJournal:     l.toolJournal,
		ToolTasks:       l.toolTasks,
		ToolStatus:      l.toolStatus,
		ToolSkills:      l.toolSkills,
		ToolNotifier:    l.toolNotifier,
		ToolCrons:       l.toolCrons,
		ToolRules:       l.toolRules,
		ToolConfig:      l.toolConfig,
		ToolIdentity:    l.toolIdentity,
		Config:          &AgentConfig{Model: l.config.Model, MaxRounds: l.config.maxRoundsForMode(mode)},
		CheckpointStore: l.checkpoints,
		Origin:          "task",
	}
}

// SetInitialState restores tick count and reflection time from crash recovery.
func (l *Loop) SetInitialState(state LoopState) {
	l.tickCount.Store(state.TickCount)
	l.lastReflect = state.LastReflection
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

	// 1. Check for due cron jobs and submit them as tasks (before claiming).
	cronSubmitted := l.checkDueCrons(ctx)

	// 1b. Prune old rule execution logs daily.
	if l.rulesEngine != nil && time.Since(l.lastRulePrune) > 24*time.Hour {
		if n, err := l.rulesEngine.PruneExecutions(ctx, 30*24*time.Hour); err != nil {
			l.logger.Warn("rules: prune failed", "error", err)
		} else if n > 0 {
			l.logger.Info("rules: pruned old executions", "count", n)
		}
		l.lastRulePrune = time.Now()
	}

	// 2. Check for pending tasks and execute the highest priority one.
	executed, taskSummary, taskDetails := l.executePendingTask(ctx)

	// 3. If no task was executed and no cron submitted, run orchestrator.
	// Observations are gathered inside Evaluate (after throttle check) to avoid
	// unnecessary DB queries on every tick.
	if !executed && !cronSubmitted && l.config.IdleEnabled {
		gatherFn := func(gCtx context.Context) *Observations {
			obs := l.gatherObservations(ctx)
			l.gatherMemories(ctx, obs)
			return obs
		}
		if decision := l.orchestrator.Evaluate(ctx, gatherFn, l.tickCount.Load()); decision != nil {
			l.lastIdleDecision = orchestratorDecisionToIdle(decision)
		}
	}

	// 4. Run reflection if interval elapsed.
	if time.Since(l.lastReflect) >= l.config.ReflectionInterval && l.reflector != nil {
		l.runReflection(ctx)
		l.lastReflect = time.Now()
	}

	// 4. Checkpoint state for crash recovery.
	CheckpointState(ctx, l.db, l.tickCount.Load(), l.lastReflect)

	// 5. Publish heartbeat.
	dur := time.Since(start).Milliseconds()
	if l.bus != nil {
		eventbus.Publish(l.bus, AgentHeartbeat{
			EventMeta:  eventbus.NewMeta("agent"),
			TickNumber: l.tickCount.Load(),
			TaskRun:    executed,
			DurationMs: dur,
		})
	}

	// 6. Record tick activity (skip empty idle ticks — they're noise).
	if l.activityStore != nil {
		var entry *ActivityEntry
		if executed {
			entry = &ActivityEntry{Type: "task", Summary: taskSummary, Details: taskDetails, DurationMs: dur}
		} else if cronSubmitted {
			entry = &ActivityEntry{Type: "cron", Summary: "Submitted cron job(s)", DurationMs: dur}
		} else if l.lastIdleDecision != nil {
			d := l.lastIdleDecision
			// Short summary for the header (what was done).
			summary := "Idle: " + d.Action
			if d.Action == "notify" && d.Message != "" {
				// First line of notification message as summary hint.
				firstLine := d.Message
				if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
					firstLine = firstLine[:idx]
				}
				if len(firstLine) > 80 {
					firstLine = firstLine[:77] + "..."
				}
				summary = "Notified: " + firstLine
			}
			// Full details with reason, action, and message for the expandable body.
			var detailBuf strings.Builder
			fmt.Fprintf(&detailBuf, "Action: %s\n", d.Action)
			if d.Reason != "" {
				fmt.Fprintf(&detailBuf, "Reason: %s\n", d.Reason)
			}
			if d.Message != "" {
				fmt.Fprintf(&detailBuf, "Message: %s\n", d.Message)
			}
			entry = &ActivityEntry{Type: "idle", Summary: summary, Details: detailBuf.String(), DurationMs: dur}
			l.lastIdleDecision = nil
		}
		// Only record when something meaningful happened.
		if entry != nil {
			if err := l.activityStore.Record(ctx, *entry); err != nil {
				l.logger.Warn("agent loop: failed to record activity", "error", err)
			} else if l.bus != nil {
				eventbus.Publish(l.bus, AgentActivityEvent{
					EventMeta: eventbus.NewMeta("agent"),
					Entry:     *entry,
				})
			}
		}
	}
}

func (l *Loop) executePendingTask(ctx context.Context) (executed bool, summary, details string) {
	if l.tasks == nil || l.agent == nil {
		return false, "", ""
	}

	// Try to claim any pending task.
	t, err := l.tasks.Claim(ctx, "")
	if err != nil || t == nil {
		return false, "", ""
	}

	// Build activity summary from task info.
	desc := t.Description
	if desc == "" {
		// Fallback: extract instruction from Input JSON (cron tasks store it there).
		var inputData map[string]string
		if json.Unmarshal(t.Input, &inputData) == nil {
			if inst := inputData["instruction"]; inst != "" {
				desc = inst
			} else if msg := inputData["message"]; msg != "" {
				desc = msg
			}
		}
	}
	if desc == "" {
		desc = string(t.Type)
	}
	if len(desc) > 80 {
		desc = desc[:77] + "..."
	}
	summary = fmt.Sprintf("Task: %s", desc)
	details = fmt.Sprintf("Type: %s\nID: %s", string(t.Type), t.ID)
	if t.Description != "" {
		details += fmt.Sprintf("\nDescription: %s", t.Description)
	}

	l.logger.Info("agent loop: executing task", "task", t.ID, "type", t.Type, "description", t.Description)

	sessionID := "loop-" + t.ID
	defer builtin.CleanupSessionFiles(sessionID)

	// Determine mode based on task type.
	mode := tool.ModeWork
	if t.Type == "coding" {
		mode = tool.ModeCoding
	}

	// Check if session already exists (resume after crash).
	var session *Session
	if l.sessions != nil {
		if existing, err := l.sessions.Get(ctx, sessionID); err == nil && len(existing.Events) > 0 {
			session = existing
			l.logger.Info("agent loop: resuming interrupted session",
				"task", t.ID, "session", sessionID, "events", len(existing.Events))
		}
	}
	if session == nil {
		session = &Session{
			ID:    sessionID,
			Mode:  mode,
			State: map[string]any{"taskId": t.ID},
		}
		if l.sessions != nil {
			if err := l.sessions.Create(ctx, session); err != nil {
				l.logger.Warn("agent loop: session create failed", "session", sessionID, "error", err)
			}
		}
	}

	// Create isolated worktree for coding tasks.
	if mode == tool.ModeCoding && l.worktreeManager != nil {
		inputRepo := extractRepoFromInput(t.Input)

		// If allowlist is configured, verify the repo is permitted.
		if len(l.config.CodingAllowedRepos) > 0 {
			targetRepo := l.worktreeManager.RepoDir()
			if inputRepo != "" {
				targetRepo = inputRepo
			}
			if !l.isRepoAllowed(targetRepo) {
				l.logger.Error("agent loop: repo not in allowed list, failing task",
					"repo", targetRepo, "allowed", l.config.CodingAllowedRepos)
				l.tasks.Fail(ctx, t.ID, fmt.Errorf("repo %q not in CODING_ALLOWED_REPOS", targetRepo))
				return true, summary, details
			}
		}

		wtPath, _, wtErr := l.worktreeManager.Create(t.ID, "HEAD", inputRepo)
		if wtErr != nil {
			l.logger.Error("agent loop: worktree creation failed, failing task", "task", t.ID, "error", wtErr)
			l.tasks.Fail(ctx, t.ID, fmt.Errorf("worktree creation failed: %w", wtErr))
			return true, summary, details
		} else {
			session.State["workDir"] = wtPath
			defer func() {
				if rmErr := l.worktreeManager.Remove(t.ID); rmErr != nil {
					l.logger.Warn("agent loop: worktree cleanup failed", "task", t.ID, "error", rmErr)
				} else {
					l.logger.Info("agent loop: worktree cleaned", "task", t.ID)
				}
			}()
		}
	}

	// Use description as user message; fall back to instruction from Input JSON.
	userMessage := t.Description
	var inputData map[string]string
	json.Unmarshal(t.Input, &inputData)
	if userMessage == "" {
		if inst := inputData["instruction"]; inst != "" {
			userMessage = inst
		}
	}

	// Continuation: if this task references a previous session, prepend its journal summary.
	if cont := inputData["continuation"]; cont != "" && l.journaler != nil && l.journaler.store != nil {
		entries, err := l.journaler.store.Recent(ctx, 24*time.Hour)
		if err == nil {
			for _, e := range entries {
				if strings.Contains(e.SessionID, cont) && e.Summary != "" {
					userMessage = "## Previous Session\n" + e.Summary + "\n\n## Continue\n" + userMessage
					l.logger.Info("agent loop: loaded continuation context", "from", cont, "summaryLen", len(e.Summary))
					break
				}
			}
		}
	}

	invCtx := l.buildInvocationContext(ctx, sessionID, userMessage, mode, session)

	// Heartbeat the lease every 2 minutes while the agent runs.
	// Without this, the 5-minute lease expires during long tasks (coding: 400 rounds)
	// and the reaper re-queues the task, causing duplicate execution.
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				hbCtx, hbCancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := l.tasks.Heartbeat(hbCtx, t.ID); err != nil {
					l.logger.Warn("agent loop: lease heartbeat failed", "task", t.ID, "error", err)
				}
				hbCancel()
			}
		}
	}()
	defer close(heartbeatDone)

	// Run agent, collect assistant response only (skip user events).
	var response strings.Builder
	taskStart := time.Now()
	for ev := range l.agent.Run(invCtx) {
		if ev.Err != nil {
			l.logger.Error("agent loop: task error", "task", t.ID, "error", ev.Err)
			if err := l.tasks.Fail(ctx, t.ID, ev.Err); err != nil {
				l.logger.Warn("agent loop: fail task error", "task", t.ID, "error", err)
			}
			return true, summary, details
		}
		if ev.Event != nil {
			session.Events = append(session.Events, ev.Event)
			// Persist events so they survive crashes (matches chat path in routes.go).
			if l.sessions != nil {
				if err := l.sessions.AppendEvent(ctx, session.ID, ev.Event); err != nil {
					l.logger.Warn("agent loop: event persist failed", "session", session.ID, "error", err)
				}
			}
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
		return true, summary, details
	}
	// Use background context for cleanup - must complete even if loop ctx canceled.
	cleanupCtx := context.Background()
	if err := l.tasks.Complete(cleanupCtx, t.ID, json.RawMessage(outputJSON)); err != nil {
		l.logger.Warn("agent loop: complete task error", "task", t.ID, "error", err)
	}
	l.logger.Info("agent loop: task completed", "task", t.ID, "duration", time.Since(taskStart))

	// Journal the session.
	if l.journaler != nil {
		l.journaler.Record(cleanupCtx, session, time.Since(taskStart))
	}

	// Extract memories from completed session (fire-and-forget).
	// Skip trivial sessions — need at least a few exchanges to extract meaningful facts.
	const minEventsForExtraction = 4
	if l.extractor != nil && len(session.Events) >= minEventsForExtraction {
		go func() {
			ectx, ecancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer ecancel()
			l.extractor.Extract(ectx, buildTranscript(session))
		}()
	}

	return true, summary, details
}

func (l *Loop) runReflection(ctx context.Context) {
	start := time.Now()
	result, err := l.reflector.Reflect(ctx)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		l.logger.Warn("agent loop: reflection failed", "error", err)
		if l.activityStore != nil {
			l.activityStore.Record(ctx, ActivityEntry{
				Type: "reflection", Summary: "Reflection failed", Details: err.Error(), DurationMs: dur,
			})
		}
		return
	}

	if len(result.Memories) == 0 && result.SoulPatch == "" && len(result.StaleMemoryIDs) == 0 {
		l.logger.Debug("agent loop: reflection found no patterns")
		return
	}

	l.logger.Info("agent loop: reflection complete",
		"memories", len(result.Memories),
		"stale", len(result.StaleMemoryIDs),
		"soulPatch", result.SoulPatch != "")

	if err := l.reflector.Apply(ctx, result); err != nil {
		l.logger.Warn("agent loop: reflection apply failed", "error", err)
	}

	// Record activity.
	if l.activityStore != nil {
		summary := fmt.Sprintf("Reflection: %d proposed, %d stale", len(result.Memories), len(result.StaleMemoryIDs))
		details := fmt.Sprintf("Memories proposed: %d\nStale rejected: %d\n", len(result.Memories), len(result.StaleMemoryIDs))
		for _, m := range result.Memories {
			details += fmt.Sprintf("- [%s] %s (%.0f%%)\n", m.Category, m.Content, m.Confidence*100)
		}
		if result.SoulPatch != "" {
			summary += " + SOUL patch"
			details += fmt.Sprintf("\nSOUL.md patch proposed:\n%s\n", result.SoulPatch)
		}
		l.activityStore.Record(ctx, ActivityEntry{
			Type: "reflection", Summary: summary, Details: details, DurationMs: dur,
		})
	}

	// Propose soul patch for human review (surfaced on /soul page).
	if result.SoulPatch != "" && l.soul != nil {
		patch := l.soul.ProposePatch(result.SoulPatch, "reflection")
		if l.bus != nil {
			eventbus.Publish(l.bus, eventbus.SoulPatchProposed{
				EventMeta: eventbus.NewMeta("reflection"),
				PatchID:   patch.ID,
			})
		}
		if l.notifier != nil {
			l.notifier.Notify(ctx, "SOUL.md patch proposed - review it on the Soul page", 1)
		}
	}
}

// isRepoAllowed checks if a repo path is in the allowed coding repos list.
// Returns false if the allowlist is empty (caller should skip the check).
// Normalizes the input path before comparison to prevent bypasses.
func (l *Loop) isRepoAllowed(repoPath string) bool {
	if len(l.config.CodingAllowedRepos) == 0 {
		return false
	}
	// Normalize input path to match config (which was normalized on load).
	normalized := repoPath
	if abs, err := filepath.Abs(repoPath); err == nil {
		normalized = filepath.Clean(abs)
	}
	for _, allowed := range l.config.CodingAllowedRepos {
		if normalized == allowed {
			return true
		}
	}
	return false
}

// extractRepoFromInput parses task input JSON for a "repo" field.
func extractRepoFromInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var data map[string]string
	if err := json.Unmarshal(input, &data); err != nil {
		return ""
	}
	return data["repo"]
}

// checkDueCrons finds cron jobs that are due and submits them as tasks.
func (l *Loop) checkDueCrons(ctx context.Context) bool {
	if l.cronStore == nil {
		return false
	}
	dueJobs, err := l.cronStore.GetDueJobs(ctx, time.Now().UTC())
	if err != nil {
		l.logger.Warn("cron: failed to get due jobs", "error", err)
		return false
	}
	submitted := false
	for _, job := range dueJobs {
		// If a specific agent type is bound, spawn it via subagentRunner.
		if job.AgentType != "" && l.subagentRunner != nil {
			spawnReq := &tool.SubagentSpawnRequest{
				Type:        job.AgentType,
				Instruction: job.Instruction,
				ExecMode:    "background",
			}
			result, err := l.subagentRunner.Spawn(ctx, "cron-"+job.ID, spawnReq)
			now := time.Now()
			if err != nil {
				l.logger.Warn("cron: agent type spawn failed", "job", job.Name, "type", job.AgentType, "error", err)
				l.cronStore.RecordExecution(ctx, job.ID, "", "failed", err)
			} else {
				l.cronStore.RecordExecution(ctx, job.ID, result.TaskID, "fired", nil)
				l.logger.Info("cron: agent type spawned", "job", job.Name, "type", job.AgentType, "task", result.TaskID)
			}
			// Always update last_run_at and next_run_at, even on failure,
			// to prevent tight retry loops when spawn errors occur.
			loc := time.UTC
			if job.Timezone != "" && job.Timezone != "UTC" {
				if tl, err := time.LoadLocation(job.Timezone); err == nil {
					loc = tl
				}
			}
			next, _ := cairncron.NextRun(job.Schedule, now.In(loc))
			if uerr := l.cronStore.UpdateAfterRun(ctx, job.ID, now.UTC(), next.UTC()); uerr != nil {
				l.logger.Warn("cron: failed to update last_run_at after spawn", "job", job.Name, "error", uerr)
			}
			if err == nil {
				submitted = true
			}
			continue
		}

		input, _ := json.Marshal(map[string]string{
			"cronJobID":   job.ID,
			"cronJobName": job.Name,
			"instruction": job.Instruction,
		})
		t, err := l.tasks.Submit(ctx, &task.SubmitRequest{
			Type:        "cron",
			Priority:    task.Priority(job.Priority),
			Description: job.Instruction,
			Input:       input,
		})
		now := time.Now()
		loc := time.UTC
		if job.Timezone != "" && job.Timezone != "UTC" {
			if l, err := time.LoadLocation(job.Timezone); err == nil {
				loc = l
			}
		}
		next, _ := cairncron.NextRun(job.Schedule, now.In(loc))
		if err != nil {
			l.logger.Warn("cron: failed to submit task", "job", job.Name, "error", err)
			l.cronStore.RecordExecution(ctx, job.ID, "", "failed", err)
		} else {
			l.cronStore.RecordExecution(ctx, job.ID, t.ID, "fired", nil)
			l.logger.Info("cron: task submitted", "job", job.Name, "task", t.ID, "nextRun", next)
		}
		// Always update last_run_at and next_run_at, even on failure,
		// to prevent tight retry loops when submit errors occur.
		if uerr := l.cronStore.UpdateAfterRun(ctx, job.ID, now.UTC(), next.UTC()); uerr != nil {
			l.logger.Warn("cron: failed to update last_run_at after submit", "job", job.Name, "error", uerr)
		}
		if err == nil {
			submitted = true
		}
	}
	return submitted
}

// AgentHeartbeat is emitted every tick via the event bus.
type AgentHeartbeat struct {
	eventbus.EventMeta
	TickNumber int64 `json:"tickNumber"`
	TaskRun    bool  `json:"taskRun"`
	DurationMs int64 `json:"durationMs"`
}

// AgentActivityEvent is emitted when agent activity is recorded (for SSE broadcast).
type AgentActivityEvent struct {
	eventbus.EventMeta
	Entry ActivityEntry `json:"entry"`
}
