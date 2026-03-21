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
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// subagentTypeConfig holds the static configuration for a built-in subagent type.
type subagentTypeConfig struct {
	Mode         tool.Mode
	AllowedTools []string // nil = all tools in mode
	MaxRounds    int
	Worktree     bool
	SystemPrompt string
}

// builtinTypes maps subagent type names to their configurations.
var builtinTypes = map[string]subagentTypeConfig{
	"researcher": {
		Mode: tool.ModeTalk,
		AllowedTools: []string{
			"cairn.readFile", "cairn.listFiles", "cairn.searchFiles",
			"cairn.searchMemory", "cairn.webSearch", "cairn.webFetch",
			"cairn.readFeed",
		},
		MaxRounds:    15,
		SystemPrompt: "You are a research agent. Gather information thoroughly, cite sources, and return a comprehensive summary. You have read-only access - you cannot modify files or run commands.",
	},
	"coder": {
		Mode:         tool.ModeCoding,
		MaxRounds:    50,
		Worktree:     true,
		SystemPrompt: "You are a coding agent working in an isolated git worktree. Implement the requested changes, run tests, and commit your work. Focus on correctness and clean code.",
	},
	"reviewer": {
		Mode: tool.ModeWork,
		AllowedTools: []string{
			"cairn.readFile", "cairn.listFiles", "cairn.searchFiles",
			"cairn.shell", "cairn.gitRun",
		},
		MaxRounds:    10,
		SystemPrompt: "You are a code review agent. Analyze the code for quality, security, and correctness. Provide structured feedback organized by priority: critical, warning, suggestion.",
	},
	"executor": {
		Mode: tool.ModeWork,
		AllowedTools: []string{
			"cairn.shell", "cairn.readFile", "cairn.writeFile",
			"cairn.editFile", "cairn.gitRun",
		},
		MaxRounds:    10,
		SystemPrompt: "You are an executor agent. Run the requested commands and report results. Be cautious with destructive operations.",
	},
}

// SubagentRunner implements tool.SubagentService. It spawns child agents
// with isolated context and returns condensed results to the parent.
type SubagentRunner struct {
	tasks     *task.Engine
	tools     *tool.Registry
	provider  llm.Provider
	bus       *eventbus.Bus
	worktrees *task.WorktreeManager
	logger    *slog.Logger

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
	toolConfig     tool.ConfigService
	model          string // LLM model to use
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
	ToolConfig     tool.ConfigService
	Model          string
}

// NewSubagentRunner creates a SubagentRunner from the given dependencies.
func NewSubagentRunner(deps SubagentRunnerDeps) *SubagentRunner {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &SubagentRunner{
		tasks:          deps.Tasks,
		tools:          deps.Tools,
		provider:       deps.Provider,
		bus:            deps.Bus,
		worktrees:      deps.Worktrees,
		logger:         logger,
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
		toolConfig:     deps.ToolConfig,
		model:          deps.Model,
	}
}

// Spawn implements tool.SubagentService.
func (r *SubagentRunner) Spawn(ctx context.Context, parentTaskID string, req *tool.SubagentSpawnRequest) (*tool.SubagentSpawnResult, error) {
	if req.Instruction == "" {
		return nil, fmt.Errorf("instruction is required")
	}

	typeCfg, ok := builtinTypes[req.Type]
	if !ok {
		return nil, fmt.Errorf("unknown subagent type %q (available: researcher, coder, reviewer, executor)", req.Type)
	}

	maxRounds := typeCfg.MaxRounds
	if req.MaxRounds > 0 {
		maxRounds = req.MaxRounds
	}
	if maxRounds > 100 {
		maxRounds = 100 // hard cap
	}

	execMode := req.ExecMode
	if execMode == "" {
		execMode = "foreground"
	}

	switch execMode {
	case "foreground":
		return r.runForeground(ctx, parentTaskID, req, typeCfg, maxRounds)
	case "background":
		return r.runBackground(ctx, parentTaskID, req, typeCfg)
	default:
		return nil, fmt.Errorf("unknown exec mode %q (use foreground or background)", execMode)
	}
}

// runForeground creates a child session, runs the agent synchronously, and returns a condensed summary.
func (r *SubagentRunner) runForeground(ctx context.Context, parentTaskID string, req *tool.SubagentSpawnRequest, cfg subagentTypeConfig, maxRounds int) (*tool.SubagentSpawnResult, error) {
	start := time.Now()
	childID := "sub-" + newID()

	// Publish start event.
	eventbus.Publish(r.bus, eventbus.SubagentStarted{
		EventMeta:    eventbus.NewMeta("agent"),
		ParentTaskID: parentTaskID,
		SubagentID:   childID,
		AgentType:    req.Type,
		ExecMode:     "foreground",
		Instruction:  truncate(req.Instruction, 200),
	})

	// Create fresh child session.
	session := &Session{
		ID:   childID,
		Mode: cfg.Mode,
		State: map[string]any{
			"parentTaskId": parentTaskID,
			"subagentType": req.Type,
		},
	}

	// Optionally create worktree for coder.
	if cfg.Worktree && r.worktrees != nil {
		wtPath, _, err := r.worktrees.Create(childID, "HEAD")
		if err != nil {
			return nil, fmt.Errorf("worktree creation failed: %w", err)
		}
		session.State["workDir"] = wtPath
		defer func() {
			if rmErr := r.worktrees.Remove(childID); rmErr != nil {
				r.logger.Warn("subagent: worktree cleanup failed", "id", childID, "error", rmErr)
			}
		}()
	}

	// Build scoped tool registry - never includes spawnSubagent (two-level enforcement).
	childTools := r.scopeTools(cfg)

	// Build user message.
	userMessage := req.Instruction
	if req.Context != "" {
		userMessage = "## Context from parent\n" + req.Context + "\n\n## Task\n" + req.Instruction
	}

	// Build invocation context (child gets no Subagents field - cannot spawn grandchildren).
	invCtx := &InvocationContext{
		Context:        ctx,
		SessionID:      childID,
		UserMessage:    userMessage,
		Mode:           cfg.Mode,
		Session:        session,
		Tools:          childTools,
		LLM:            r.provider,
		Memory:         r.memories,
		Soul:           r.soul,
		Bus:            r.bus,
		ContextBuilder: r.contextBuilder,
		Plugins:        r.plugins,
		ActivityStore:  r.activityStore,
		Subagents:      nil, // two-level enforcement: child cannot spawn
		ToolMemories:   r.toolMemories,
		ToolEvents:     r.toolEvents,
		ToolDigest:     r.toolDigest,
		ToolJournal:    r.toolJournal,
		ToolTasks:      r.toolTasks,
		ToolStatus:     r.toolStatus,
		ToolSkills:     r.toolSkills,
		ToolNotifier:   r.toolNotifier,
		ToolCrons:      r.toolCrons,
		ToolConfig:     r.toolConfig,
		Config:         &AgentConfig{Model: r.model, MaxRounds: maxRounds},
	}

	// Run child agent.
	childAgent := NewReActAgent("subagent:"+req.Type, r.logger)
	var response strings.Builder
	var totalToolCalls, rounds int

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

			// Track rounds and tool calls.
			if ev.Event.Round > rounds {
				rounds = ev.Event.Round

				// Publish progress every round.
				toolName := ""
				for _, part := range ev.Event.Parts {
					if tp, ok := part.(ToolPart); ok {
						totalToolCalls++
						toolName = tp.ToolName
					}
				}
				eventbus.Publish(r.bus, eventbus.SubagentProgress{
					EventMeta:  eventbus.NewMeta("agent"),
					SubagentID: childID,
					Round:      rounds,
					MaxRounds:  maxRounds,
					ToolName:   toolName,
				})
			} else {
				// Count tool calls in same round.
				for _, part := range ev.Event.Parts {
					if _, ok := part.(ToolPart); ok {
						totalToolCalls++
					}
				}
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

// runBackground submits a task for the Loop to pick up and returns immediately.
func (r *SubagentRunner) runBackground(ctx context.Context, parentTaskID string, req *tool.SubagentSpawnRequest, cfg subagentTypeConfig) (*tool.SubagentSpawnResult, error) {
	if r.tasks == nil {
		return nil, fmt.Errorf("background subagents require the task engine")
	}

	input, _ := json.Marshal(map[string]string{
		"instruction":  req.Instruction,
		"context":      req.Context,
		"subagentType": req.Type,
	})

	t, err := r.tasks.Submit(ctx, &task.SubmitRequest{
		Type:        task.TypeSubagent,
		Priority:    task.PriorityNormal,
		Mode:        string(cfg.Mode),
		ParentID:    parentTaskID,
		Input:       input,
		Description: fmt.Sprintf("[subagent:%s] %s", req.Type, truncate(req.Instruction, 100)),
	})
	if err != nil {
		return nil, fmt.Errorf("submit background subagent: %w", err)
	}

	eventbus.Publish(r.bus, eventbus.SubagentStarted{
		EventMeta:    eventbus.NewMeta("agent"),
		ParentTaskID: parentTaskID,
		SubagentID:   t.ID,
		AgentType:    req.Type,
		ExecMode:     "background",
		Instruction:  truncate(req.Instruction, 200),
	})

	return &tool.SubagentSpawnResult{
		TaskID:    t.ID,
		SessionID: "",
		Status:    "running",
	}, nil
}

// scopeTools creates a child tool.Registry containing only the allowed tools.
// The spawnSubagent tool is always excluded to enforce two-level max.
func (r *SubagentRunner) scopeTools(cfg subagentTypeConfig) *tool.Registry {
	child := tool.NewRegistry()
	parent := r.tools.All()

	if cfg.AllowedTools == nil {
		// All tools in parent except spawnSubagent.
		for _, t := range parent {
			if t.Name() != "cairn.spawnSubagent" {
				child.Register(t)
			}
		}
	} else {
		allowed := make(map[string]bool, len(cfg.AllowedTools))
		for _, name := range cfg.AllowedTools {
			allowed[name] = true
		}
		for _, t := range parent {
			if allowed[t.Name()] && t.Name() != "cairn.spawnSubagent" {
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
