package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

const maxOrchestratorActions = 5

// Orchestrator is a thin management layer that scans system state and produces
// structured decisions. It delegates ALL work through narrow interfaces:
// spawn subagent, accept/reject memory, submit task, notify.
// It does NOT: write code, edit files, run shell commands, or search the web.
type Orchestrator struct {
	memories  *memory.Service
	tasks     *task.Engine
	events    *signal.EventStore
	soul      *memory.Soul
	approvals *task.ApprovalStore

	subagentRunner tool.SubagentService
	notifier       tool.NotifyService
	bus            *eventbus.Bus

	provider      llm.Provider
	model         string
	briefingModel string

	activityStore  *ActivityStore
	reflector      *ReflectionEngine
	skillSuggestor *SkillSuggestor
	marketplace    *skill.MarketplaceClient
	toolSkills     tool.SkillService
	journaler      *Journaler
	logger         *slog.Logger
	codingEnabled  bool

	briefing        string
	briefingBuiltAt time.Time
	lastEvaluation  time.Time
}

// OrchestratorDeps carries dependencies for constructing an Orchestrator.
type OrchestratorDeps struct {
	Memories       *memory.Service
	Tasks          *task.Engine
	Events         *signal.EventStore
	Soul           *memory.Soul
	Approvals      *task.ApprovalStore
	SubagentRunner tool.SubagentService
	Notifier       tool.NotifyService
	Bus            *eventbus.Bus
	Provider       llm.Provider
	Model          string
	BriefingModel  string
	ActivityStore  *ActivityStore
	Reflector      *ReflectionEngine
	SkillSuggestor *SkillSuggestor
	Marketplace    *skill.MarketplaceClient
	ToolSkills     tool.SkillService
	Journaler      *Journaler
	Logger         *slog.Logger
	CodingEnabled  bool
	CronStore      interface{} // unused, kept for future
}

// NewOrchestrator creates an Orchestrator from the given dependencies.
func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Orchestrator{
		memories:       deps.Memories,
		tasks:          deps.Tasks,
		events:         deps.Events,
		soul:           deps.Soul,
		approvals:      deps.Approvals,
		subagentRunner: deps.SubagentRunner,
		notifier:       deps.Notifier,
		bus:            deps.Bus,
		provider:       deps.Provider,
		model:          deps.Model,
		briefingModel:  deps.BriefingModel,
		activityStore:  deps.ActivityStore,
		reflector:      deps.Reflector,
		skillSuggestor: deps.SkillSuggestor,
		marketplace:    deps.Marketplace,
		toolSkills:     deps.ToolSkills,
		journaler:      deps.Journaler,
		logger:         logger,
		codingEnabled:  deps.CodingEnabled,
	}
}

// --- Decision types ---

// OrchestratorDecision is the structured output from the orchestrator LLM call.
type OrchestratorDecision struct {
	Actions []OrchestratorAction `json:"actions"`
	Reason  string               `json:"reason"`
}

// OrchestratorAction is a single atomic action.
type OrchestratorAction struct {
	Type        string `json:"type"`                  // approve_memory, reject_memory, spawn, submit_task, notify, escalate, trigger_reflection, verify_session, wait
	MemoryID    string `json:"memoryId,omitempty"`    // approve_memory, reject_memory
	SpawnType   string `json:"spawnType,omitempty"`   // spawn: researcher, coder, reviewer, executor
	Instruction string `json:"instruction,omitempty"` // spawn, submit_task
	Context     string `json:"context,omitempty"`     // spawn: parent context
	TaskID      string `json:"taskId,omitempty"`      // verify_session
	Message     string `json:"message,omitempty"`     // notify, escalate
	Priority    int    `json:"priority,omitempty"`    // notify: 0-3
}

// --- Extended state ---

// OrchestratorState extends Observations with management-specific signals.
type OrchestratorState struct {
	Observations
	ProposedMemories     []proposedMemoryInfo `json:"proposedMemories,omitempty"`
	ActiveSubagents      []subagentTaskInfo   `json:"activeSubagents,omitempty"`
	CompletedCodingTasks []codingTaskInfo     `json:"completedCodingTasks,omitempty"`
	PendingApprovals     []approvalInfo       `json:"pendingApprovals,omitempty"`
	RecentActions        []ActivityEntry      `json:"recentActions,omitempty"`
}

type proposedMemoryInfo struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
}

type subagentTaskInfo struct {
	TaskID      string `json:"taskId"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type codingTaskInfo struct {
	TaskID      string `json:"taskId"`
	Description string `json:"description"`
	CompletedAt string `json:"completedAt"`
}

type approvalInfo struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func (s *OrchestratorState) hasActionableItems() bool {
	return len(s.ProposedMemories) > 0 ||
		len(s.ActiveSubagents) > 0 ||
		len(s.CompletedCodingTasks) > 0 ||
		len(s.PendingApprovals) > 0 ||
		!s.Observations.isEmpty()
}

// --- Core method ---

// Evaluate runs one orchestrator cycle. Called from Loop.tick() when no task was executed.
// The gatherFn is called only after the throttle check passes, avoiding unnecessary DB queries.
// Returns nil if throttled or nothing to do.
func (o *Orchestrator) Evaluate(ctx context.Context, gatherFn func(context.Context) *Observations, tickCount int64) *OrchestratorDecision {
	if o.provider == nil {
		return nil
	}
	if time.Since(o.lastEvaluation) < minIdleInterval {
		return nil
	}

	// Gather observations only after throttle passes.
	obs := gatherFn(ctx)

	// Extend observations with management state.
	state := o.gatherState(ctx, obs, tickCount)
	if !state.hasActionableItems() {
		o.logger.Debug("orchestrator: no actionable items")
		return nil
	}

	// Rebuild briefing if stale.
	if o.briefing == "" || time.Since(o.briefingBuiltAt) > briefingMaxAge {
		o.rebuildBriefing(ctx, obs)
		o.refreshSkillSuggestions(ctx)
	}

	o.lastEvaluation = time.Now()

	// Call LLM with management prompt.
	decision := o.decide(ctx, state)
	if decision == nil {
		o.logger.Debug("orchestrator: no decision returned")
		return nil
	}
	if len(decision.Actions) == 0 {
		o.logger.Debug("orchestrator: decided to wait", "reason", decision.Reason)
		return decision
	}

	// Cap actions.
	if len(decision.Actions) > maxOrchestratorActions {
		decision.Actions = decision.Actions[:maxOrchestratorActions]
	}

	// Execute decisions.
	summaries := o.execute(ctx, decision)

	o.logger.Info("orchestrator: evaluated",
		"actions", len(decision.Actions),
		"reason", decision.Reason,
		"summaries", strings.Join(summaries, "; "),
	)

	return decision
}

// --- State gathering ---

func (o *Orchestrator) gatherState(ctx context.Context, obs *Observations, tickCount int64) *OrchestratorState {
	state := &OrchestratorState{Observations: *obs}

	// Proposed memories (hard_rules + decisions that need inspection).
	if o.memories != nil {
		proposed, err := o.memories.List(ctx, memory.ListOpts{Status: memory.StatusProposed, Limit: 20})
		if err == nil {
			for _, m := range proposed {
				state.ProposedMemories = append(state.ProposedMemories, proposedMemoryInfo{
					ID:         m.ID,
					Content:    m.Content,
					Category:   string(m.Category),
					Confidence: m.Confidence,
				})
			}
		}
	}

	// Active subagents.
	if o.tasks != nil {
		running, err := o.tasks.List(ctx, task.ListOpts{Type: task.TypeSubagent, Status: task.StatusRunning, Limit: 10})
		if err == nil {
			for _, t := range running {
				state.ActiveSubagents = append(state.ActiveSubagents, subagentTaskInfo{
					TaskID:      t.ID,
					Type:        t.Mode,
					Description: t.Description,
				})
			}
		}

		// Recently completed coding tasks (last 2 hours).
		completed, err := o.tasks.List(ctx, task.ListOpts{Type: task.TypeCoding, Status: task.StatusCompleted, Limit: 5})
		if err == nil {
			cutoff := time.Now().Add(-2 * time.Hour)
			for _, t := range completed {
				if t.CompletedAt.After(cutoff) {
					state.CompletedCodingTasks = append(state.CompletedCodingTasks, codingTaskInfo{
						TaskID:      t.ID,
						Description: t.Description,
						CompletedAt: t.CompletedAt.Format(time.RFC3339),
					})
				}
			}
		}
	}

	// Pending approvals.
	if o.approvals != nil {
		pending, err := o.approvals.ListPending(ctx)
		if err == nil {
			for _, a := range pending {
				state.PendingApprovals = append(state.PendingApprovals, approvalInfo{
					ID:          a.ID,
					Type:        a.Type,
					Description: a.Description,
				})
			}
		}
	}

	// Recent orchestrator actions (to prevent repetition).
	if o.activityStore != nil {
		actions, err := o.activityStore.RecentIdleActions(ctx, 5, 2*time.Hour)
		if err == nil {
			state.RecentActions = actions
		}
	}

	return state
}

// --- LLM decision ---

func (o *Orchestrator) decide(ctx context.Context, state *OrchestratorState) *OrchestratorDecision {
	prompt := o.buildDecisionPrompt(state)

	ch, err := o.provider.Stream(ctx, &llm.Request{
		Model: o.model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens: decisionMaxTokens,
	})
	if err != nil {
		o.logger.Warn("orchestrator: LLM call failed", "error", err)
		return &OrchestratorDecision{Reason: "LLM error"}
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Text)
		case llm.StreamError:
			o.logger.Warn("orchestrator: LLM stream error", "error", e.Err)
			return &OrchestratorDecision{Reason: "LLM stream error"}
		}
	}

	return parseOrchestratorDecision(result.String())
}

func (o *Orchestrator) buildDecisionPrompt(state *OrchestratorState) string {
	var parts []string

	// SOUL context.
	if o.soul != nil {
		if content := o.soul.Content(); content != "" {
			parts = append(parts, "## SOUL\n"+content)
		}
	}

	// System prompt.
	parts = append(parts, orchestratorSystemPrompt)

	// Briefing.
	if o.briefing != "" {
		parts = append(parts, "## Situation Briefing\n"+o.briefing)
	}

	// State snapshot.
	parts = append(parts, o.formatStateSnapshot(state))

	return strings.Join(parts, "\n\n")
}

func (o *Orchestrator) formatStateSnapshot(state *OrchestratorState) string {
	var sb strings.Builder
	sb.WriteString("## Current System State\n\n")

	// Proposed memories.
	if len(state.ProposedMemories) > 0 {
		sb.WriteString(fmt.Sprintf("### Proposed Memories (%d pending)\n", len(state.ProposedMemories)))
		for _, m := range state.ProposedMemories {
			sb.WriteString(fmt.Sprintf("- [id:%s] [%s] %q (confidence: %.1f)\n", m.ID, m.Category, truncateStr(m.Content, 100), m.Confidence))
		}
		sb.WriteString("\n")
	}

	// Active subagents.
	if len(state.ActiveSubagents) > 0 {
		sb.WriteString(fmt.Sprintf("### Active Subagents (%d running)\n", len(state.ActiveSubagents)))
		for _, s := range state.ActiveSubagents {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", s.TaskID, s.Type, truncateStr(s.Description, 80)))
		}
		sb.WriteString("\n")
	}

	// Completed coding tasks.
	if len(state.CompletedCodingTasks) > 0 {
		sb.WriteString(fmt.Sprintf("### Completed Coding Sessions (%d recent)\n", len(state.CompletedCodingTasks)))
		for _, t := range state.CompletedCodingTasks {
			sb.WriteString(fmt.Sprintf("- [%s] %s (completed: %s)\n", t.TaskID, truncateStr(t.Description, 80), t.CompletedAt))
		}
		sb.WriteString("\n")
	}

	// Pending approvals.
	if len(state.PendingApprovals) > 0 {
		sb.WriteString(fmt.Sprintf("### Pending Approvals (%d waiting)\n", len(state.PendingApprovals)))
		for _, a := range state.PendingApprovals {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", a.ID, a.Type, truncateStr(a.Description, 80)))
		}
		sb.WriteString("\n")
	}

	// Feed.
	if state.UnreadFeedCount > 0 {
		sb.WriteString(fmt.Sprintf("### Feed: %d unread", state.UnreadFeedCount))
		if len(state.UnreadBySource) > 0 {
			sources := make([]string, 0, len(state.UnreadBySource))
			for src, count := range state.UnreadBySource {
				sources = append(sources, fmt.Sprintf("%s: %d", src, count))
			}
			sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(sources, ", ")))
		}
		sb.WriteString("\n\n")
	}

	// Errors.
	if len(state.RecentErrors) > 0 {
		sb.WriteString(fmt.Sprintf("### Recent Errors (%d)\n", len(state.RecentErrors)))
		for _, e := range state.RecentErrors {
			sb.WriteString(fmt.Sprintf("- %s\n", truncateStr(e, 100)))
		}
		sb.WriteString("\n")
	}

	// Recent actions (for dedup).
	if len(state.RecentActions) > 0 {
		sb.WriteString("### Recent Orchestrator Actions (DO NOT REPEAT)\n")
		for _, a := range state.RecentActions {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", a.CreatedAt, a.Summary))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("### System: tick %d, time %s\n", state.TicksSinceStart, state.CurrentTime))

	return sb.String()
}

// --- Decision execution ---

func (o *Orchestrator) execute(ctx context.Context, decision *OrchestratorDecision) []string {
	var summaries []string

	for _, action := range decision.Actions {
		switch action.Type {
		case "approve_memory":
			if o.memories != nil && action.MemoryID != "" {
				if err := o.memories.Accept(ctx, action.MemoryID); err != nil {
					o.logger.Warn("orchestrator: approve memory failed", "id", action.MemoryID, "error", err)
				} else {
					summaries = append(summaries, "Approved memory "+action.MemoryID)
				}
			}

		case "reject_memory":
			if o.memories != nil && action.MemoryID != "" {
				if err := o.memories.Reject(ctx, action.MemoryID); err != nil {
					o.logger.Warn("orchestrator: reject memory failed", "id", action.MemoryID, "error", err)
				} else {
					summaries = append(summaries, "Rejected memory "+action.MemoryID)
				}
			}

		case "spawn":
			if o.subagentRunner != nil && action.SpawnType != "" && action.Instruction != "" {
				_, err := o.subagentRunner.Spawn(ctx, "orchestrator", &tool.SubagentSpawnRequest{
					Type:        action.SpawnType,
					Instruction: action.Instruction,
					Context:     action.Context,
					ExecMode:    "background",
				})
				if err != nil {
					o.logger.Warn("orchestrator: spawn failed", "type", action.SpawnType, "error", err)
				} else {
					summaries = append(summaries, fmt.Sprintf("Spawned %s: %s", action.SpawnType, truncateStr(action.Instruction, 60)))
				}
			}

		case "submit_task":
			if o.tasks != nil && action.Instruction != "" {
				input, _ := json.Marshal(map[string]string{"instruction": action.Instruction})
				_, err := o.tasks.Submit(ctx, &task.SubmitRequest{
					Type:        task.TypeGeneral,
					Priority:    task.PriorityLow,
					Description: action.Instruction,
					Input:       input,
				})
				if err != nil {
					o.logger.Warn("orchestrator: submit task failed", "error", err)
				} else {
					summaries = append(summaries, "Submitted task: "+truncateStr(action.Instruction, 60))
				}
			}

		case "notify":
			if action.Message != "" {
				if o.bus != nil {
					eventbus.Publish(o.bus, AgentNotification{
						EventMeta: eventbus.NewMeta("orchestrator"),
						Message:   action.Message,
						Priority:  action.Priority,
					})
				}
				summaries = append(summaries, "Notified: "+truncateStr(action.Message, 60))
			}

		case "escalate":
			if o.approvals != nil && action.Message != "" {
				o.approvals.Create(ctx, &task.Approval{
					Type:        "orchestrator",
					Description: action.Message,
				})
				summaries = append(summaries, "Escalated: "+truncateStr(action.Message, 60))
			}

		case "trigger_reflection":
			if o.reflector != nil {
				result, err := o.reflector.Reflect(ctx)
				if err != nil {
					o.logger.Warn("orchestrator: reflection failed", "error", err)
				} else {
					summaries = append(summaries, fmt.Sprintf("Reflection: %d memories, %d stale", len(result.Memories), len(result.StaleMemoryIDs)))
				}
			}

		case "verify_session":
			if o.subagentRunner != nil && action.TaskID != "" {
				instruction := fmt.Sprintf(
					"Verify coding session completion for task %s. "+
						"Run: gh pr list --state open --json number,title,statusCheckRollup. "+
						"Check CI status and unresolved review threads. "+
						"Report: CI pass/fail, unresolved comment count, and whether follow-up is needed.",
					action.TaskID)
				o.subagentRunner.Spawn(ctx, "orchestrator", &tool.SubagentSpawnRequest{
					Type:        "reviewer",
					Instruction: instruction,
					ExecMode:    "background",
				})
				summaries = append(summaries, "Verifying session "+action.TaskID)
			}

		case "wait":
			summaries = append(summaries, "Waiting")
		}
	}

	return summaries
}

// --- Briefing ---

func (o *Orchestrator) rebuildBriefing(ctx context.Context, obs *Observations) {
	model := o.briefingModel
	if model == "" {
		model = o.model
	}

	prompt := buildBriefingPrompt(obs)

	ch, err := o.provider.Stream(ctx, &llm.Request{
		Model:     model,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}}},
		MaxTokens: briefingMaxTokens,
	})
	if err != nil {
		o.logger.Warn("orchestrator: briefing rebuild failed", "error", err)
		return
	}

	var result strings.Builder
	for ev := range ch {
		if td, ok := ev.(llm.TextDelta); ok {
			result.WriteString(td.Text)
		}
	}

	if result.Len() > 0 {
		o.briefing = result.String()
		o.briefingBuiltAt = time.Now()
		o.logger.Info("orchestrator: briefing rebuilt", "len", result.Len())
	}
}

func (o *Orchestrator) refreshSkillSuggestions(ctx context.Context) {
	if o.skillSuggestor == nil || o.marketplace == nil {
		return
	}
	var journalStore *JournalStore
	if o.journaler != nil {
		journalStore = o.journaler.store
	}
	signals := CollectSignals(ctx, journalStore, o.activityStore, o.toolSkills)
	if len(signals) > 0 {
		o.skillSuggestor.GenerateSuggestions(ctx, signals, o.marketplace, o.toolSkills)
	} else {
		o.skillSuggestor.ClearStale()
	}
}

// --- Parsing ---

func parseOrchestratorDecision(raw string) *OrchestratorDecision {
	raw = strings.TrimSpace(raw)
	raw = stripMarkdownFences(raw)

	var decision OrchestratorDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return &OrchestratorDecision{
			Actions: []OrchestratorAction{{Type: "wait"}},
			Reason:  "Failed to parse decision: " + err.Error(),
		}
	}

	// Filter out unknown action types.
	valid := decision.Actions[:0]
	validTypes := map[string]bool{
		"approve_memory": true, "reject_memory": true, "spawn": true,
		"submit_task": true, "notify": true, "escalate": true,
		"trigger_reflection": true, "verify_session": true, "wait": true,
	}
	for _, a := range decision.Actions {
		if validTypes[a.Type] {
			valid = append(valid, a)
		}
	}
	decision.Actions = valid

	if len(decision.Actions) == 0 {
		decision.Actions = []OrchestratorAction{{Type: "wait"}}
	}

	return &decision
}

// --- Bridge ---

// OrchestratorDecisionToIdle converts to the existing IdleDecision for activity recording.
func orchestratorDecisionToIdle(d *OrchestratorDecision) *IdleDecision {
	if d == nil || len(d.Actions) == 0 {
		return &IdleDecision{Action: "wait", Reason: "no decision"}
	}

	first := d.Actions[0]
	action := "wait"
	switch first.Type {
	case "approve_memory", "reject_memory", "spawn", "submit_task", "verify_session":
		action = "task"
	case "notify", "escalate":
		action = "notify"
	case "trigger_reflection":
		action = "learn"
	}

	return &IdleDecision{
		Action:   action,
		Reason:   d.Reason,
		Message:  first.Message,
		Priority: first.Priority,
	}
}

// --- System prompt ---

const orchestratorSystemPrompt = `You are Cairn's orchestrator — a management layer that makes decisions about autonomous behavior.

## Your Role
You SCAN system state and DECIDE what to do. You delegate ALL work.
You do NOT: write code, edit files, run shell commands, search the web, or talk to users directly.

## Available Actions

### approve_memory
Auto-approve proposed memories. Use for: hard_rules and decisions with clear, reusable content.
Fields: memoryId
Note: facts and preferences are already auto-approved by the extractor. You only see hard_rules and decisions here.

### reject_memory
Reject proposed memories that are too session-specific, wrong, or duplicates.
Fields: memoryId

### spawn
Spawn a subagent for delegated work. Types: researcher, coder, reviewer, executor.
Fields: spawnType, instruction, context (optional)

### submit_task
Submit a task for the main agent loop to execute on the next tick.
Fields: instruction

### notify
Send a notification to the human via configured channels.
Fields: message, priority (0=low, 1=medium, 2=high, 3=critical)
ONLY for things that need human attention.

### escalate
Create a human approval request for an irreversible action.
Fields: message

### trigger_reflection
Run the reflection engine to detect patterns and propose memories.
Use sparingly — after several sessions complete.

### verify_session
Spawn a reviewer to check if a completed coding session needs follow-up.
Fields: taskId

### wait
Do nothing. Often the correct choice.

## Decision Rules
1. Hard rules/decisions: approve if genuinely reusable across sessions. Reject if too specific.
2. Max 5 actions per evaluation. Prefer fewer, targeted actions.
3. Never repeat an action from Recent Actions.
4. "wait" is valid and often correct. Don't act just to act.
5. Only notify for things the human needs NOW. Most things can wait.
6. Escalate only for: merge PRs, deploy, send external messages.

## Output Format
JSON only. No markdown. No commentary outside JSON.
{"actions": [{"type": "...", ...}], "reason": "brief assessment"}`

// truncateStr shortens a string for display.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
