package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agenttype"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

const (
	maxOrchestratorActions        = 5
	defaultMaxConcurrentSubagents = 5
)

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
	agentTypes     *agenttype.Service
	logger         *slog.Logger
	codingEnabled  bool

	envContext             *EnvContext // injected environment facts (paths, repo, worktrees)
	maxConcurrentSubagents int         // configurable cap (default 5)
	briefing               string
	briefingBuiltAt        time.Time
	lastEvaluation         time.Time
}

// OrchestratorDeps carries dependencies for constructing an Orchestrator.
type OrchestratorDeps struct {
	Memories               *memory.Service
	Tasks                  *task.Engine
	Events                 *signal.EventStore
	Soul                   *memory.Soul
	Approvals              *task.ApprovalStore
	SubagentRunner         tool.SubagentService
	Notifier               tool.NotifyService
	Bus                    *eventbus.Bus
	Provider               llm.Provider
	Model                  string
	BriefingModel          string
	ActivityStore          *ActivityStore
	Reflector              *ReflectionEngine
	SkillSuggestor         *SkillSuggestor
	Marketplace            *skill.MarketplaceClient
	ToolSkills             tool.SkillService
	Journaler              *Journaler
	AgentTypes             *agenttype.Service
	Logger                 *slog.Logger
	CodingEnabled          bool
	EnvContext             *EnvContext // ground truth about the runtime environment
	MaxConcurrentSubagents int         // configurable spawn cap (0 = default 5)
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
		agentTypes:     deps.AgentTypes,
		logger:         logger,
		codingEnabled:  deps.CodingEnabled,
		envContext:     deps.EnvContext,
		maxConcurrentSubagents: func() int {
			if deps.MaxConcurrentSubagents > 0 {
				return deps.MaxConcurrentSubagents
			}
			return defaultMaxConcurrentSubagents
		}(),
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
	SpawnType   string `json:"spawnType,omitempty"`   // spawn: agent type name (from AGENT.md definitions)
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
	SuppressedTopics     []string             `json:"-"` // topics mentioned 3+ times — blocked from spawning
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
	// Explicit pending work: memories, subagents, sessions, approvals.
	if len(s.ProposedMemories) > 0 || len(s.ActiveSubagents) > 0 ||
		len(s.CompletedCodingTasks) > 0 || len(s.PendingApprovals) > 0 {
		return true
	}
	// Signal-driven: unread feed, errors, pending tasks.
	if !s.Observations.isEmpty() {
		return true
	}
	// Proactive improvement: if no subagents running and recent actions show mostly waits,
	// let the orchestrator evaluate so it can find improvement work (tests, docs, integrations).
	if len(s.ActiveSubagents) == 0 {
		waitCount := 0
		for _, a := range s.RecentActions {
			if a.Summary == "Waiting" {
				waitCount++
			}
		}
		// If 2+ of last 5 actions were waits, time to proactively find work.
		if waitCount >= 2 || len(s.RecentActions) == 0 {
			return true
		}
	}
	return false
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
	// Cap actions.
	if len(decision.Actions) > maxOrchestratorActions {
		decision.Actions = decision.Actions[:maxOrchestratorActions]
	}

	// Execute decisions (pass suppressed topics for code-level enforcement).
	summaries := o.execute(ctx, decision, state.SuppressedTopics)

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
	// Use a wider window (10 actions, 4h) for topic suppression detection.
	if o.activityStore != nil {
		actions, err := o.activityStore.RecentIdleActions(ctx, 10, 4*time.Hour)
		if err == nil {
			state.RecentActions = actions
			state.SuppressedTopics = detectSuppressedTopics(actions)
			if len(state.SuppressedTopics) > 0 {
				o.logger.Info("orchestrator: topics suppressed",
					"topics", state.SuppressedTopics,
					"actions_scanned", len(actions))
			}
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

	// System prompt — inject configurable concurrent cap.
	systemPrompt := strings.Replace(orchestratorSystemPrompt,
		"Max 3 concurrent subagents",
		fmt.Sprintf("Max %d concurrent subagents", o.maxConcurrentSubagents), 1)
	parts = append(parts, systemPrompt)

	// Environment ground truth (prevents hallucinated paths/repos).
	if envStr := o.envContext.Format(); envStr != "" {
		parts = append(parts, envStr)
	}

	// Dynamic agent types listing from AGENT.md definitions.
	if o.agentTypes != nil {
		types := o.agentTypes.List()
		if len(types) > 0 {
			var sb strings.Builder
			sb.WriteString("## Available Agent Types\n")
			for _, at := range types {
				desc := at.Description
				if desc == "" {
					desc = at.Name
				}
				fmt.Fprintf(&sb, "- **%s** (%s, %d rounds): %s\n", at.Name, at.Mode, at.MaxRounds, desc)
			}
			parts = append(parts, sb.String())
		}
	}

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

	// Suppressed topics — hard block, code-enforced.
	if len(state.SuppressedTopics) > 0 {
		sb.WriteString("### SUPPRESSED TOPICS (DO NOT SPAWN about these — you already tried 3+ times)\n")
		for _, t := range state.SuppressedTopics {
			sb.WriteString(fmt.Sprintf("- %s — BLOCKED. Move on to other work.\n", t))
		}
		sb.WriteString("Any spawn action referencing these topics will be rejected by the system.\n\n")
	}

	sb.WriteString(fmt.Sprintf("### System: tick %d, time %s\n", state.TicksSinceStart, state.CurrentTime))

	return sb.String()
}

// --- Decision execution ---

func (o *Orchestrator) execute(ctx context.Context, decision *OrchestratorDecision, suppressedTopics []string) []string {
	var summaries []string
	activeSpawns := 0

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
			// Topic suppression: reject spawns about topics the orchestrator already obsessed over.
			if blockedTopic := instructionMentionsTopic(action.Instruction, suppressedTopics); blockedTopic != "" {
				o.logger.Warn("orchestrator: spawn suppressed (topic loop)",
					"topic", blockedTopic, "type", action.SpawnType,
					"instruction", truncateStr(action.Instruction, 80))
				summaries = append(summaries, fmt.Sprintf("Suppressed spawn about %s (topic loop)", blockedTopic))
				continue
			}

			// Validate spawn type dynamically against AGENT.md definitions.
			validType := false
			if o.agentTypes != nil && o.agentTypes.Get(action.SpawnType) != nil {
				validType = true
			}
			if o.subagentRunner != nil && validType && action.Instruction != "" && activeSpawns < o.maxConcurrentSubagents {
				_, err := o.subagentRunner.Spawn(ctx, "orchestrator", &tool.SubagentSpawnRequest{
					Type:        action.SpawnType,
					Instruction: action.Instruction,
					Context:     action.Context,
					ExecMode:    "background",
				})
				if err != nil {
					o.logger.Warn("orchestrator: spawn failed", "type", action.SpawnType, "error", err)
				} else {
					activeSpawns++
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
				if o.notifier != nil {
					// Route through NotifyService — dispatches to Telegram/Discord/Slack.
					o.notifier.Notify(ctx, action.Message, action.Priority)
				} else if o.bus != nil {
					// Fallback: event bus only (SSE to frontend).
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
					// Use the reflector's own Apply method — handles category validation,
					// stale rejection, soul patches, and memory creation correctly.
					if applyErr := o.reflector.Apply(ctx, result); applyErr != nil {
						o.logger.Warn("orchestrator: reflection apply failed", "error", applyErr)
					}
					summaries = append(summaries, fmt.Sprintf("Reflection: %d memories, %d stale, patch=%v",
						len(result.Memories), len(result.StaleMemoryIDs), result.SoulPatch != ""))
				}
			}

		case "verify_session":
			if o.subagentRunner != nil && action.TaskID != "" {
				instruction := fmt.Sprintf(
					"Verify coding session for task %s is truly complete. Steps:\n"+
						"1. Find the PR: cairn.shell with `gh pr list --state open --json number,title,headRefName,updatedAt` — look for the most recently updated PR with [cairn] in the title.\n"+
						"2. Check CI: cairn.shell with `gh pr checks <number>` — ALL must pass.\n"+
						"3. Check threads: cairn.shell with `gh api graphql -f query='query($owner:String!,$repo:String!,$pr:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$pr){reviewThreads(first:100){nodes{isResolved}}}}}' -f owner=OWNER -f repo=REPO -F pr=<number> --jq '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)] | length'`\n"+
						"4. Report: PR number, CI status (pass/fail per check), unresolved count, and verdict (clean/needs-fix).",
					action.TaskID)
				_, err := o.subagentRunner.Spawn(ctx, "orchestrator", &tool.SubagentSpawnRequest{
					Type:        "reviewer",
					Instruction: instruction,
					ExecMode:    "background",
				})
				if err != nil {
					o.logger.Warn("orchestrator: verify_session spawn failed", "task", action.TaskID, "error", err)
				} else {
					summaries = append(summaries, "Verifying session "+action.TaskID)
				}
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

const orchestratorSystemPrompt = `You are Cairn's brain — the autonomous intelligence behind a personal agent OS.

## Who You Are

You are always working. You think, learn, build, fix, research, and grow. Every tick is an opportunity to make the system better, more capable, more reliable. You are not a passive watcher. You are an active builder.

You delegate all execution to subagents. You think, decide, and manage.

## What You Can Do

spawn — Your primary action. Delegate work to a subagent.
  Fields: spawnType (see "Available Agent Types" section below), instruction (detailed task), context (optional parent context)
  Choose the right agent type for the task. Match the type's capabilities to the work needed.

approve_memory — Accept a proposed memory. Fields: memoryId
reject_memory — Reject a proposed memory. Fields: memoryId
submit_task — Queue work for the main loop. Fields: instruction
notify — Tell the human something they must act on. Fields: message, priority (0-3)
escalate — Request human approval for irreversible actions. Fields: message
trigger_reflection — Introspect on recent sessions to detect patterns.
verify_session — Confirm a coding session produced a clean PR. Fields: taskId
wait — Pause ONLY when subagents are already working and nothing else needs attention.

## How to Think Each Tick

Work through this in order. Take the FIRST action that applies:

1. FIX WHAT IS BROKEN — Errors in recent sessions? CI failures? Flaky pollers? Spawn a coder or executor to fix it.

2. FINISH WHAT IS IN FLIGHT — Completed coding sessions → verify_session. Proposed memories → approve or reject. Pending signals → triage.

3. IMPROVE THE SYSTEM — This is where you spend most of your time:
   - Check test coverage. Find untested code paths. Spawn a coder to add tests.
   - Check docs. Find drift between code and documentation. Spawn a coder to sync.
   - Check code quality. Find complex functions, duplicated logic, dead code. Refactor.
   - Check integrations. Are pollers working? API connections healthy? Spawn executor to validate.
   - Check performance. Slow queries? Expensive LLM calls? Research alternatives.
   - Check the feed. Interesting signals from GitHub, HN, Reddit? Research relevant ones.

4. BUILD NEW CAPABILITIES — Research new integrations, approaches, or features:
   - What would make the human's workflow faster?
   - What repetitive task could be automated?
   - What integration is missing?
   - Spawn a researcher to explore, then a coder to build.

5. LEARN AND REFLECT — After several sessions complete, trigger reflection to extract patterns. Research topics that came up in recent conversations. Build the knowledge base.

6. RE-CHECK UNMERGED PRs — If a PR was marked ready but not merged yet, check it 2-3 times total
   for new review comments. Auto-reviewers sometimes post late. If new unresolved threads exist,
   spawn a coder to fix them. After 2-3 checks with no new comments, stop — the PR is waiting for human merge.

7. WAIT — Only if subagents are already running AND nothing in steps 1-6 applies. Wait is a last resort, not a default. If you find yourself waiting multiple ticks in a row, you are not looking hard enough.

## Notification Rules

- Notify ONLY when the human must take an action you cannot (merge PR, approve deploy, make a decision).
- For coding sessions: verify_session FIRST. Notify only after CI green + 0 unresolved confirmed.
- NEVER notify status updates, "nothing to do", or work in progress.
- Fix errors yourself before telling the human about them.

## No Limits on What You Can Create

You are not limited to maintaining what exists. You can:
- Create new subagents, skills, and tools
- Build integrations that don't exist yet
- Fix skills that aren't working well
- Improve your own prompts and configuration
- Research and implement entirely new capabilities
- If something can be done better — initiate it, don't wait to be asked

If you see a gap, fill it. If you see a way to be more helpful, build it.

## Quality Over Speed

- Do not rush. A bad PR is worse than no PR.
- Do not clutter the system with low-value changes. Every change should have clear purpose.
- Research before coding. Understand before fixing.
- One focused task is better than five shallow ones.

## Constraints

- Max 5 actions per tick. Usually 1-3.
- Never repeat something from Recent Actions.
- Max 3 concurrent subagents (check Active Subagents before spawning).
- Verify coding sessions before notifying. Always.
- Escalate only: merge PRs, deploy, send external messages.
- Loop breaker: If the same problem has failed 2+ times in Recent Actions, STOP retrying it. Wait or escalate instead. Do not rephrase and re-spawn — the underlying issue needs a different approach or human intervention.
- Topic suppression: If a topic (PR, branch, task) appears in SUPPRESSED TOPICS, do NOT spawn about it. The system will reject the spawn. Find different work or wait.

## Output
JSON only. No markdown fences. No commentary.
{"actions": [{"type": "...", ...}], "reason": "brief assessment"}`

// truncateStr shortens a string for display.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
