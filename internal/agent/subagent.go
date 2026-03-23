package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agenttype"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// subagentTypeConfig holds the static configuration for a built-in subagent type.
type subagentTypeConfig struct {
	Mode         tool.Mode
	AllowedTools []string // allowlist: nil = all tools in mode
	DeniedTools  []string // denylist: nil = no denials. Takes precedence when AllowedTools is nil.
	Skills       []string // skills to pre-load into session
	MaxRounds    int
	Worktree     bool
	SystemPrompt string
}

const (
	// DefaultMaxSpawnDepth is the default maximum nesting depth for subagents.
	// parent(0) -> child(1) -> grandchild(2). Depth 3 means 3 levels allowed.
	DefaultMaxSpawnDepth = 3
	// AbsoluteMaxSpawnDepth is the hard cap - not configurable beyond this.
	AbsoluteMaxSpawnDepth = 5
)

// SubagentRunner implements tool.SubagentService. It spawns child agents
// with isolated context and returns condensed results to the parent.
type SubagentRunner struct {
	tasks     *task.Engine
	tools     *tool.Registry
	provider  llm.Provider
	bus       *eventbus.Bus
	worktrees *task.WorktreeManager
	logger    *slog.Logger

	// Depth tracking for multi-level spawning.
	maxSpawnDepth int // configurable limit (default DefaultMaxSpawnDepth)
	currentDepth  int // this runner's depth (0 = top-level)

	// Dependencies forwarded to child InvocationContexts.
	memories       *memory.Service
	soul           *memory.Soul
	contextBuilder *memory.ContextBuilder
	plugins        *plugin.Manager
	activityStore  *ActivityStore
	toolMemories   tool.MemoryService
	toolEvents     tool.EventService
	toolDigest     tool.DigestService
	toolJournal    tool.JournalService
	toolTasks      tool.TaskService
	toolStatus     tool.StatusService
	toolSkills     tool.SkillService
	toolNotifier   tool.NotifyService
	toolCrons      tool.CronService
	toolRules      tool.RulesService
	toolConfig     tool.ConfigService
	toolIdentity   tool.IdentityService
	agentsFile     *memory.AgentsFile // operating manual for subagents
	agentTypes     *agenttype.Service // AGENT.md type definitions (nil = fallback)
	envContext     *EnvContext        // runtime environment context (nil = omit)
	model          string             // LLM model to use
}

// SubagentRunnerDeps carries dependencies for constructing a SubagentRunner.
type SubagentRunnerDeps struct {
	Tasks          *task.Engine
	Tools          *tool.Registry
	Provider       llm.Provider
	Bus            *eventbus.Bus
	Worktrees      *task.WorktreeManager
	Logger         *slog.Logger
	Memories       *memory.Service
	Soul           *memory.Soul
	ContextBuilder *memory.ContextBuilder
	Plugins        *plugin.Manager
	ActivityStore  *ActivityStore
	ToolMemories   tool.MemoryService
	ToolEvents     tool.EventService
	ToolDigest     tool.DigestService
	ToolJournal    tool.JournalService
	ToolTasks      tool.TaskService
	ToolStatus     tool.StatusService
	ToolSkills     tool.SkillService
	ToolNotifier   tool.NotifyService
	ToolCrons      tool.CronService
	ToolRules      tool.RulesService
	ToolConfig     tool.ConfigService
	ToolIdentity   tool.IdentityService
	AgentsFile     *memory.AgentsFile // optional: operating manual for subagents
	AgentTypes     *agenttype.Service // optional: AGENT.md type definitions
	EnvContext     *EnvContext        // optional: runtime environment context
	Model          string
	MaxSpawnDepth  int // 0 = DefaultMaxSpawnDepth
	CurrentDepth   int // depth of this runner (0 = top-level)
}

// NewSubagentRunner creates a SubagentRunner from the given dependencies.
func NewSubagentRunner(deps SubagentRunnerDeps) *SubagentRunner {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxDepth := deps.MaxSpawnDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxSpawnDepth
	}
	if maxDepth > AbsoluteMaxSpawnDepth {
		maxDepth = AbsoluteMaxSpawnDepth
	}
	return &SubagentRunner{
		tasks:          deps.Tasks,
		tools:          deps.Tools,
		provider:       deps.Provider,
		bus:            deps.Bus,
		worktrees:      deps.Worktrees,
		logger:         logger,
		maxSpawnDepth:  maxDepth,
		currentDepth:   deps.CurrentDepth,
		memories:       deps.Memories,
		soul:           deps.Soul,
		contextBuilder: deps.ContextBuilder,
		plugins:        deps.Plugins,
		activityStore:  deps.ActivityStore,
		toolMemories:   deps.ToolMemories,
		toolEvents:     deps.ToolEvents,
		toolDigest:     deps.ToolDigest,
		toolJournal:    deps.ToolJournal,
		toolTasks:      deps.ToolTasks,
		toolStatus:     deps.ToolStatus,
		toolSkills:     deps.ToolSkills,
		toolNotifier:   deps.ToolNotifier,
		toolCrons:      deps.ToolCrons,
		toolRules:      deps.ToolRules,
		toolConfig:     deps.ToolConfig,
		toolIdentity:   deps.ToolIdentity,
		agentsFile:     deps.AgentsFile,
		agentTypes:     deps.AgentTypes,
		envContext:     deps.EnvContext,
		model:          deps.Model,
	}
}

// resolveType looks up a subagent type from AGENT.md definitions (if available),
// converting to the internal subagentTypeConfig.
func (r *SubagentRunner) resolveType(name string) (subagentTypeConfig, error) {
	if r.agentTypes == nil {
		return subagentTypeConfig{}, fmt.Errorf("agent type service not initialized")
	}
	if at := r.agentTypes.Get(name); at != nil {
		cfg := subagentTypeConfig{
			Mode:         at.Mode,
			AllowedTools: at.AllowedTools,
			DeniedTools:  at.DeniedTools,
			Skills:       at.Skills,
			MaxRounds:    at.MaxRounds,
			Worktree:     at.Worktree,
			SystemPrompt: at.Content,
		}
		return cfg, nil
	}
	return subagentTypeConfig{}, fmt.Errorf("not found")
}

// buildSubagentHint combines type system prompt, env context, and AGENTS.md content.
func (r *SubagentRunner) buildSubagentHint(typePrompt string) string {
	var parts []string
	if typePrompt != "" {
		parts = append(parts, typePrompt)
	}
	if r.envContext != nil {
		if envStr := r.envContext.Format(); envStr != "" {
			parts = append(parts, envStr)
		}
	}
	if r.agentsFile != nil {
		if content := r.agentsFile.Content(); content != "" {
			parts = append(parts, "## Operating Manual\n"+content)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// Spawn implements tool.SubagentService.
func (r *SubagentRunner) Spawn(ctx context.Context, parentTaskID string, req *tool.SubagentSpawnRequest) (*tool.SubagentSpawnResult, error) {
	if req.Instruction == "" {
		return nil, fmt.Errorf("instruction is required")
	}

	// Depth check: prevent spawning beyond the configured limit.
	if r.currentDepth >= r.maxSpawnDepth {
		return nil, fmt.Errorf("max subagent nesting depth (%d) reached at depth %d", r.maxSpawnDepth, r.currentDepth)
	}

	typeCfg, resolveErr := r.resolveType(req.Type)
	if resolveErr != nil {
		// Collect available type names for the error message.
		var available []string
		if r.agentTypes != nil {
			for _, at := range r.agentTypes.List() {
				available = append(available, at.Name)
			}
		}
		if len(available) == 0 {
			available = []string{"(none configured)"}
		}
		return nil, fmt.Errorf("unknown subagent type %q (available: %s)", req.Type, strings.Join(available, ", "))
	}

	// Reject coder subagents when worktree isolation is unavailable.
	if typeCfg.Worktree && r.worktrees == nil {
		return nil, fmt.Errorf("coder subagent requires worktree isolation (CODING_ENABLED=true)")
	}

	maxRounds := typeCfg.MaxRounds
	if req.MaxRounds > 0 {
		maxRounds = req.MaxRounds
	}
	if maxRounds > 400 {
		maxRounds = 400 // hard cap — same as main coding mode
	}

	execMode := req.ExecMode
	if execMode == "" {
		execMode = "foreground"
	}

	switch execMode {
	case "foreground":
		return r.runForeground(ctx, parentTaskID, req, typeCfg, maxRounds)
	case "background":
		return r.runBackground(ctx, parentTaskID, req, typeCfg, maxRounds)
	default:
		return nil, fmt.Errorf("unknown exec mode %q (use foreground or background)", execMode)
	}
}

// runForeground creates a child session, runs the agent synchronously, and returns a condensed summary.
func (r *SubagentRunner) runForeground(ctx context.Context, parentTaskID string, req *tool.SubagentSpawnRequest, cfg subagentTypeConfig, maxRounds int) (*tool.SubagentSpawnResult, error) {
	childID := "sub-" + newID()
	return r.executeSubagent(ctx, childID, parentTaskID, req, cfg, maxRounds, "foreground")
}

// runBackground spawns a goroutine that runs the subagent asynchronously and returns immediately.
// Uses a cancellable context so /v1/subagents/{id}/cancel can stop it.
func (r *SubagentRunner) runBackground(ctx context.Context, parentTaskID string, req *tool.SubagentSpawnRequest, cfg subagentTypeConfig, maxRounds int) (*tool.SubagentSpawnResult, error) {
	childID := "sub-" + newID()

	// Publish start event immediately (same ID will be used by executeSubagent).
	eventbus.Publish(r.bus, eventbus.SubagentStarted{
		EventMeta:    eventbus.NewMeta("agent"),
		ParentTaskID: parentTaskID,
		SubagentID:   childID,
		AgentType:    req.Type,
		ExecMode:     "background",
		Instruction:  truncate(req.Instruction, 200),
	})

	// Record in task engine for REST API listing and cancellation.
	var taskID string
	if r.tasks != nil {
		input, _ := json.Marshal(map[string]string{
			"instruction":  req.Instruction,
			"context":      req.Context,
			"subagentType": req.Type,
			"subagentId":   childID,
		})
		t, err := r.tasks.Submit(ctx, &task.SubmitRequest{
			Type:        task.TypeSubagent,
			Priority:    task.PriorityNormal,
			Mode:        string(cfg.Mode),
			ParentID:    parentTaskID,
			Input:       input,
			Description: fmt.Sprintf("[subagent:%s] %s", req.Type, truncate(req.Instruction, 100)),
		})
		if err == nil && t != nil {
			taskID = t.ID
		}
	}

	// Use a cancellable context so the task engine cancel can stop the goroutine.
	bgCtx, bgCancel := context.WithCancel(context.Background())

	// Run in background goroutine using the SAME childID.
	go func() {
		defer bgCancel()
		defer func() {
			if p := recover(); p != nil {
				stack := string(debug.Stack())
				panicErr := fmt.Errorf("subagent panic: %v", p)
				r.logger.Error("background subagent panicked", "id", childID, "panic", p, "stack", stack)
				// Terminal failure — panicking tasks must not be retried.
				if r.tasks != nil && taskID != "" {
					if err := r.tasks.FailTerminal(context.Background(), taskID, panicErr); err != nil {
						r.logger.Error("subagent panic: failed to update task", "id", taskID, "err", err)
					}
				}
				// Publish completion event so the UI doesn't show a hung task.
				eventbus.Publish(r.bus, eventbus.SubagentCompleted{
					EventMeta:  eventbus.NewMeta("agent"),
					SubagentID: childID,
					Status:     "failed",
					Error:      panicErr.Error(),
				})
			}
		}()
		result, err := r.executeSubagent(bgCtx, childID, parentTaskID, req, cfg, maxRounds, "background")
		// Update task engine status on completion.
		if r.tasks != nil && taskID != "" {
			if err != nil || (result != nil && result.Status == "failed") {
				errMsg := "unknown error"
				if err != nil {
					errMsg = err.Error()
				} else if result != nil {
					errMsg = result.Error
				}
				r.tasks.Fail(context.Background(), taskID, fmt.Errorf("%s", errMsg))
			} else if result != nil {
				outputJSON, _ := json.Marshal(result.Summary)
				r.tasks.Complete(context.Background(), taskID, outputJSON)
			}
		}
		if err != nil {
			r.logger.Error("background subagent failed", "id", childID, "error", err)
		} else if result != nil {
			r.logger.Info("background subagent completed", "id", childID, "status", result.Status)
		}
	}()

	_ = bgCancel // suppress unused warning — cancel is deferred in goroutine

	return &tool.SubagentSpawnResult{
		TaskID:    childID,
		SessionID: "",
		Status:    "running",
	}, nil
}

// executeSubagent is the shared implementation for both foreground and background execution.
// childID is pre-generated by the caller to ensure consistent IDs across start/progress/complete events.
func (r *SubagentRunner) executeSubagent(ctx context.Context, childID, parentTaskID string, req *tool.SubagentSpawnRequest, cfg subagentTypeConfig, maxRounds int, execMode string) (*tool.SubagentSpawnResult, error) {
	start := time.Now()

	// Publish start event (only for foreground - background already published).
	if execMode == "foreground" {
		eventbus.Publish(r.bus, eventbus.SubagentStarted{
			EventMeta:    eventbus.NewMeta("agent"),
			ParentTaskID: parentTaskID,
			SubagentID:   childID,
			AgentType:    req.Type,
			ExecMode:     execMode,
			Instruction:  truncate(req.Instruction, 200),
		})
	}

	// Create fresh child session.
	session := &Session{
		ID:   childID,
		Mode: cfg.Mode,
		State: map[string]any{
			"parentTaskId": parentTaskID,
			"subagentType": req.Type,
			"spawnDepth":   r.currentDepth + 1,
		},
	}

	// Pre-load skills defined in the agent type's AGENT.md frontmatter.
	if len(cfg.Skills) > 0 && r.toolSkills != nil {
		for _, skillName := range cfg.Skills {
			sk := r.toolSkills.Get(skillName)
			if sk != nil && sk.Content != "" {
				session.ActiveSkills = append(session.ActiveSkills, ActiveSkill{
					Name:         sk.Name,
					Content:      sk.Content,
					AllowedTools: sk.AllowedTools,
				})
			}
		}
	}

	// Create worktree for coder type (already validated in Spawn that worktrees != nil).
	if cfg.Worktree {
		wtPath, _, err := r.worktrees.Create(childID, "HEAD", req.Repo)
		if err != nil {
			return nil, fmt.Errorf("worktree creation failed: %w", err)
		}
		session.State["workDir"] = wtPath
		session.State["confined"] = true
		defer func() {
			if rmErr := r.worktrees.Remove(childID); rmErr != nil {
				r.logger.Warn("subagent: worktree cleanup failed", "id", childID, "error", rmErr)
			}
		}()
	}

	// Mark read-only when file-write tools are denied.
	// This activates shell policy tier 2 (blocks git commit, sed -i, etc.)
	for _, d := range cfg.DeniedTools {
		if d == "cairn.writeFile" {
			session.State["readOnly"] = true
			break
		}
	}

	// Build scoped tool registry — spawn tool conditionally included based on depth.
	childTools := r.scopeTools(cfg)

	// Build user message with type-specific system prompt prefix.
	userMessage := req.Instruction
	if req.Context != "" {
		userMessage = "## Context from parent\n" + req.Context + "\n\n## Task\n" + req.Instruction
	}

	// Build the subagent system hint combining type prompt, env context, AGENTS.md, and identity.
	systemHint := r.buildSystemHint(ctx, r.buildSubagentHint(cfg.SystemPrompt))

	// Create child runner if depth budget allows further nesting.
	var childSubagents tool.SubagentService
	if r.currentDepth+1 < r.maxSpawnDepth {
		childSubagents = r.childRunner()
	}

	// Build invocation context. Child gets a SubagentRunner if depth allows.
	// AgentsFile is forwarded (operating manual applies to all agents) but NOT UserProfile or CuratedMemory.
	invCtx := &InvocationContext{
		Context:       ctx,
		SessionID:     childID,
		UserMessage:   userMessage,
		Mode:          cfg.Mode,
		Session:       session,
		Tools:         childTools,
		LLM:           r.provider,
		Memory:        r.memories,
		Soul:          r.soul,
		AgentsFile:    r.agentsFile,
		Bus:           r.bus,
		Plugins:       r.plugins,
		ActivityStore: r.activityStore,
		Subagents:     childSubagents,
		ToolMemories:  r.toolMemories,
		ToolEvents:    r.toolEvents,
		ToolDigest:    r.toolDigest,
		ToolJournal:   r.toolJournal,
		ToolTasks:     r.toolTasks,
		ToolStatus:    r.toolStatus,
		ToolSkills:    r.toolSkills,
		ToolNotifier:  r.toolNotifier,
		ToolCrons:     r.toolCrons,
		ToolRules:     r.toolRules,
		ToolConfig:    r.toolConfig,
		ToolIdentity:  r.toolIdentity,
		Config: &AgentConfig{
			Model:              r.model,
			MaxRounds:          maxRounds,
			SubagentSystemHint: systemHint,
		},
	}

	// Run child agent.
	childAgent := NewReActAgent("subagent:"+req.Type, r.logger)
	var response strings.Builder
	var totalToolCalls, rounds int
	// Track unique tool call IDs to avoid double-counting (ReAct emits both Running and Completed).
	seenCallIDs := make(map[string]bool)

	for ev := range childAgent.Run(invCtx) {
		if ev.Err != nil {
			durationMs := time.Since(start).Milliseconds()
			eventbus.Publish(r.bus, eventbus.SubagentCompleted{
				EventMeta:  eventbus.NewMeta("agent"),
				SubagentID: childID,
				Status:     "failed",
				Error:      ev.Err.Error(),
				DurationMs: durationMs,
				ToolCalls:  totalToolCalls,
				Rounds:     rounds,
			})
			return &tool.SubagentSpawnResult{
				TaskID:     childID,
				SessionID:  childID,
				Status:     "failed",
				Error:      ev.Err.Error(),
				Rounds:     rounds,
				ToolCalls:  totalToolCalls,
				DurationMs: durationMs,
			}, nil
		}

		if ev.Event != nil {
			session.Events = append(session.Events, ev.Event)

			// Track rounds and tool calls (deduplicated by CallID).
			if ev.Event.Round > rounds {
				rounds = ev.Event.Round
			}

			toolName := ""
			for _, part := range ev.Event.Parts {
				if tp, ok := part.(ToolPart); ok {
					if tp.CallID != "" && !seenCallIDs[tp.CallID] {
						seenCallIDs[tp.CallID] = true
						totalToolCalls++
						toolName = tp.ToolName
					}
				}
			}

			// Publish progress on new rounds.
			if ev.Event.Round >= rounds && toolName != "" {
				eventbus.Publish(r.bus, eventbus.SubagentProgress{
					EventMeta:  eventbus.NewMeta("agent"),
					SubagentID: childID,
					Round:      rounds,
					MaxRounds:  maxRounds,
					ToolName:   toolName,
				})
			}

			// Collect text output.
			if ev.Event.Author != "user" {
				for _, part := range ev.Event.Parts {
					if tp, ok := part.(TextPart); ok {
						response.WriteString(tp.Text)
					}
				}
			}
		}
	}

	durationMs := time.Since(start).Milliseconds()
	fullOutput := response.String()

	// Condense output for parent model.
	summary := r.condenseSummary(ctx, fullOutput)

	eventbus.Publish(r.bus, eventbus.SubagentCompleted{
		EventMeta:  eventbus.NewMeta("agent"),
		SubagentID: childID,
		Status:     "completed",
		Summary:    truncate(summary, 500),
		DurationMs: durationMs,
		ToolCalls:  totalToolCalls,
		Rounds:     rounds,
	})

	r.logger.Info("subagent completed",
		"id", childID, "type", req.Type, "rounds", rounds,
		"tools", totalToolCalls, "duration_ms", durationMs)

	return &tool.SubagentSpawnResult{
		TaskID:     childID,
		SessionID:  childID,
		Summary:    summary,
		Status:     "completed",
		Rounds:     rounds,
		ToolCalls:  totalToolCalls,
		DurationMs: durationMs,
	}, nil
}

// childRunner creates a SubagentRunner for the next depth level.
// The child inherits all dependencies but has an incremented depth.
func (r *SubagentRunner) childRunner() *SubagentRunner {
	child := *r // shallow copy — all pointer fields are shared (intentional)
	child.currentDepth = r.currentDepth + 1
	return &child
}

// scopeTools creates a child tool.Registry with appropriate tool access.
// Priority: AllowedTools (allowlist) > DeniedTools (denylist) > all tools.
// The spawnSubagent tool is denied when the child would be at the depth limit.
func (r *SubagentRunner) scopeTools(cfg subagentTypeConfig) *tool.Registry {
	child := tool.NewRegistry()
	parent := r.tools.All()

	// Deny spawn tool only when the child is at the depth limit.
	denied := make(map[string]bool)
	if r.currentDepth+1 >= r.maxSpawnDepth {
		denied["cairn.spawnSubagent"] = true
	}
	for _, name := range cfg.DeniedTools {
		denied[name] = true
	}

	if cfg.AllowedTools != nil {
		// Allowlist mode: only include explicitly allowed tools (minus denied).
		allowed := make(map[string]bool, len(cfg.AllowedTools))
		for _, name := range cfg.AllowedTools {
			allowed[name] = true
		}
		for _, t := range parent {
			if allowed[t.Name()] && !denied[t.Name()] {
				child.Register(t)
			}
		}
	} else {
		// Default: all tools in parent minus denied set.
		for _, t := range parent {
			if !denied[t.Name()] {
				child.Register(t)
			}
		}
	}
	return child
}

// condenseSummary compresses the full subagent output into a short summary.
// If the output is already short, returns it as-is.
func (r *SubagentRunner) condenseSummary(ctx context.Context, fullOutput string) string {
	if len(fullOutput) < 800 {
		return fullOutput
	}

	// Use LLM to condense. If this fails, truncate manually.
	condensed, err := r.llmCondense(ctx, fullOutput)
	if err != nil {
		r.logger.Warn("subagent: condense failed, truncating", "error", err)
		if len(fullOutput) > 2000 {
			return fullOutput[:2000] + "\n\n[truncated - full output was " + fmt.Sprintf("%d", len(fullOutput)) + " chars]"
		}
		return fullOutput
	}
	return condensed
}

// llmCondense calls the LLM to summarize the output in under 500 tokens.
func (r *SubagentRunner) llmCondense(ctx context.Context, fullOutput string) (string, error) {
	// Truncate input if extremely long to stay within context.
	input := fullOutput
	if len(input) > 32000 {
		input = input[:16000] + "\n\n...[middle truncated]...\n\n" + input[len(input)-16000:]
	}

	ch, err := r.provider.Stream(ctx, &llm.Request{
		Model: r.model,
		System: "Summarize the following agent output concisely. Keep key findings, " +
			"file paths, and actionable conclusions. Stay under 500 tokens. " +
			"Do not add commentary - just the summary.",
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: input}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("condense LLM call failed: %w", err)
	}
	var result strings.Builder
	for event := range ch {
		if td, ok := event.(llm.TextDelta); ok {
			result.WriteString(td.Text)
		}
	}
	if result.Len() == 0 {
		return "", fmt.Errorf("empty LLM response")
	}
	return result.String(), nil
}

// buildSystemHint injects canonical identity (ghOwner) into a system prompt.
// Shared between executeSubagent and tests via buildSystemHintForTest.
func (r *SubagentRunner) buildSystemHint(ctx context.Context, basePrompt string) string {
	systemHint := basePrompt
	if r.toolConfig != nil {
		sysCfg, err := r.toolConfig.GetConfig(ctx)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("subagent: failed to get config for identity injection", "error", err)
			}
		} else if owner, ok := sysCfg["ghOwner"].(string); ok && owner != "" {
			if isValidGitHubOwner(owner) {
				systemHint += fmt.Sprintf("\n\n## Canonical Identity\n- GitHub repo owner: %s (exact spelling — never guess or fabricate this value)", owner)
			} else if r.logger != nil {
				r.logger.Warn("subagent: skipping identity injection — invalid ghOwner", "owner", truncate(fmt.Sprintf("%q", owner), 256))
			}
		}
	}
	return systemHint
}

// ghOwnerPattern validates GitHub usernames: alphanumeric + hyphens, 1-39 chars, no leading/trailing hyphens.
var ghOwnerPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)

// isValidGitHubOwner checks that owner matches GitHub's username rules.
func isValidGitHubOwner(owner string) bool {
	if len(owner) < 1 || len(owner) > 39 {
		return false
	}
	// GitHub disallows consecutive hyphens in usernames/organizations.
	if strings.Contains(owner, "--") {
		return false
	}
	return ghOwnerPattern.MatchString(owner)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
