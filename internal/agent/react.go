package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/tool"
)

// ReActAgent implements the ReAct (Reason + Act) loop.
// Each round: call LLM → if tool calls, execute them → feed results back → repeat.
// Stops when LLM produces text without tool calls, or max rounds reached.
type ReActAgent struct {
	name   string
	modes  map[tool.Mode]*ModeConfig
	logger *slog.Logger
}

// NewReActAgent creates a ReAct agent with default mode configurations.
func NewReActAgent(name string, logger *slog.Logger) *ReActAgent {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReActAgent{
		name:   name,
		modes:  DefaultModes(),
		logger: logger,
	}
}

func (a *ReActAgent) Name() string        { return a.name }
func (a *ReActAgent) Description() string { return "ReAct agent with tool execution loop" }

// Run executes the ReAct loop, streaming events on the returned channel.
// The channel is closed when the agent finishes (either by producing a final
// response or exhausting max rounds).
func (a *ReActAgent) Run(invCtx *InvocationContext) <-chan RunEvent {
	ch := make(chan RunEvent, 32)

	go func() {
		defer close(ch)
		a.run(invCtx, ch)
	}()

	return ch
}

func (a *ReActAgent) run(invCtx *InvocationContext, ch chan<- RunEvent) {
	mode := invCtx.Mode
	modeConfig, ok := a.modes[mode]
	if !ok {
		modeConfig = a.modes[tool.ModeTalk] // fallback
	}

	maxRounds := modeConfig.MaxRounds
	if invCtx.Config != nil && invCtx.Config.MaxRounds > 0 {
		maxRounds = invCtx.Config.MaxRounds
	}

	model := ""
	if invCtx.Config != nil {
		model = invCtx.Config.Model
	}

	// Build conversation history.
	messages := invCtx.Session.History()

	// Compact session if over token threshold.
	if invCtx.CompactionConfig.TriggerTokens > 0 {
		tokens := EstimateMessageTokens(messages)
		if tokens > invCtx.CompactionConfig.TriggerTokens {
			compacted, compErr := CompactMessages(invCtx.Context, messages, invCtx.LLM, invCtx.CompactionConfig)
			if compErr != nil {
				a.logger.Warn("compaction failed, using full history", "tokens", tokens, "error", compErr)
			} else {
				a.logger.Info("session compacted",
					"before", len(messages), "after", len(compacted),
					"tokensBefore", tokens, "tokensAfter", EstimateMessageTokens(compacted),
				)
				messages = compacted
			}
		}
	}

	// Add user message.
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.TextBlock{Text: invCtx.UserMessage}},
	})

	// Emit user event.
	userEvent := &Event{
		ID:        newID(),
		SessionID: invCtx.SessionID,
		Timestamp: time.Now(),
		Author:    "user",
		Round:     0,
		Parts:     []Part{TextPart{Text: invCtx.UserMessage}},
	}
	emit(invCtx.Context, ch, RunEvent{Event: userEvent})

	// Build system prompt (includes skill catalog + active skills).
	systemPrompt := BuildSystemPrompt(invCtx, modeConfig, invCtx.ContextBuilder, invCtx.JournalEntries)

	// Get available tools for this mode, filtered by active skill restrictions.
	var allowedTools []string
	if invCtx.Session != nil {
		allowedTools = invCtx.Session.AllowedToolsFromSkills()
	}
	toolDefs := invCtx.Tools.ForLLMFiltered(mode, allowedTools)

	// Track whether a skill was activated this round (triggers prompt + tool rebuild).
	skillActivated := false

	// Publish stream started event.
	if invCtx.Bus != nil {
		eventbus.Publish(invCtx.Bus, eventbus.StreamStarted{
			EventMeta: eventbus.NewMeta("agent"),
			Model:     model,
		})
	}
	publishSessionEvent(invCtx, "state_change", map[string]any{"state": "running", "model": model})

	// Plugin: BeforeAgentRun.
	agentStart := time.Now()
	inv := &plugin.Invocation{
		SessionID:   invCtx.SessionID,
		UserMessage: invCtx.UserMessage,
		Mode:        mode,
		Model:       model,
		StartedAt:   agentStart,
	}
	if invCtx.Plugins != nil {
		var hookErr error
		invCtx.Context, hookErr = invCtx.Plugins.RunBeforeAgentRun(invCtx.Context, inv)
		if hookErr != nil {
			emit(invCtx.Context, ch, RunEvent{Err: fmt.Errorf("plugin: %w", hookErr)})
			return
		}
	}

	var totalToolCalls int
	for round := 0; round < maxRounds; round++ {
		// Check context cancellation.
		if invCtx.Context.Err() != nil {
			emit(invCtx.Context, ch, RunEvent{Err: invCtx.Context.Err()})
			return
		}

		// Check for steering messages between rounds.
		if invCtx.SteeringCh != nil {
			select {
			case msg := <-invCtx.SteeringCh:
				if msg.Priority == "stop" {
					publishSessionEvent(invCtx, "state_change", map[string]any{"state": "stopped", "reason": "user_stop"})
					emit(invCtx.Context, ch, RunEvent{Event: &Event{
						ID: newID(), SessionID: invCtx.SessionID, Timestamp: time.Now(),
						Author: a.name, Parts: []Part{TextPart{Text: "[session stopped by user]"}},
					}})
					return
				}
				// Inject steering message as user turn.
				publishSessionEvent(invCtx, "user_steer", map[string]any{"content": msg.Content})
				messages = append(messages, llm.Message{
					Role:    llm.RoleUser,
					Content: []llm.ContentBlock{llm.TextBlock{Text: "[User steering]: " + msg.Content}},
				})
				a.logger.Info("steering message injected", "content", msg.Content)
			default:
				// No steering message, continue normally.
			}
		}

		a.logger.Debug("agent round", "round", round, "mode", mode, "messages", len(messages))

		// 1. Call LLM.
		req := &llm.Request{
			Model:           model,
			Messages:        messages,
			System:          systemPrompt,
			Tools:           toolDefs,
			EnableWebSearch: hasWebSearchTool(toolDefs),
		}

		// Plugin: BeforeLLMCall.
		llmCall := &plugin.LLMCall{Model: model, Round: round}
		if invCtx.Plugins != nil {
			var hookErr error
			invCtx.Context, hookErr = invCtx.Plugins.RunBeforeLLMCall(invCtx.Context, llmCall)
			if hookErr != nil {
				emit(invCtx.Context, ch, RunEvent{Err: fmt.Errorf("plugin: %w", hookErr)})
				if invCtx.Plugins != nil {
					invCtx.Plugins.RunOnAgentError(invCtx.Context, inv, hookErr)
				}
				return
			}
		}

		llmCh, err := invCtx.LLM.Stream(invCtx.Context, req)
		if err != nil {
			if invCtx.Plugins != nil {
				invCtx.Context = invCtx.Plugins.RunOnLLMError(invCtx.Context, llmCall, err)
				invCtx.Plugins.RunOnAgentError(invCtx.Context, inv, err)
			}
			emit(invCtx.Context, ch, RunEvent{Err: fmt.Errorf("llm stream: %w", err)})
			return
		}

		// 2. Collect LLM response.
		var roundText strings.Builder
		var roundReasoning strings.Builder
		var toolCalls []llm.ToolCallDelta
		var inputTokens, outputTokens int

		for llmEvent := range llmCh {
			switch e := llmEvent.(type) {
			case llm.TextDelta:
				roundText.WriteString(e.Text)
				// Stream text deltas to the caller.
				emit(invCtx.Context, ch, RunEvent{Event: &Event{
					ID:        newID(),
					SessionID: invCtx.SessionID,
					Timestamp: time.Now(),
					Author:    a.name,
					Round:     round,
					Parts:     []Part{TextPart{Text: e.Text}},
				}})
				publishSessionEvent(invCtx, "text_delta", map[string]any{"text": e.Text, "round": round})

			case llm.ReasoningDelta:
				roundReasoning.WriteString(e.Text)
				publishSessionEvent(invCtx, "thinking", map[string]any{"text": e.Text, "round": round})

			case llm.ToolCallDelta:
				toolCalls = append(toolCalls, e)

			case llm.MessageEnd:
				inputTokens = e.InputTokens
				outputTokens = e.OutputTokens

			case llm.StreamError:
				emit(invCtx.Context, ch, RunEvent{Err: e.Err})
				return
			}
		}

		// Emit accumulated reasoning for this round.
		if r := roundReasoning.String(); r != "" {
			emit(invCtx.Context, ch, RunEvent{Event: &Event{
				ID:        newID(),
				SessionID: invCtx.SessionID,
				Timestamp: time.Now(),
				Author:    a.name,
				Round:     round,
				Parts:     []Part{ReasoningPart{Text: r}},
			}})
		}

		// Plugin: AfterLLMCall.
		if invCtx.Plugins != nil {
			invCtx.Context = invCtx.Plugins.RunAfterLLMCall(invCtx.Context, llmCall, &plugin.TokenUsage{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				Model:        model,
			})
		}

		a.logger.Debug("round complete",
			"round", round,
			"text_len", roundText.Len(),
			"tool_calls", len(toolCalls),
			"tokens_in", inputTokens,
			"tokens_out", outputTokens,
		)

		// 3. If no tool calls, we're done.
		if len(toolCalls) == 0 {
			publishSessionEvent(invCtx, "state_change", map[string]any{"state": "completed"})
			if invCtx.Bus != nil {
				eventbus.Publish(invCtx.Bus, eventbus.StreamEnded{
					EventMeta:    eventbus.NewMeta("agent"),
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
					FinishReason: "stop",
				})
			}
			// Clear checkpoint on successful completion.
			if invCtx.CheckpointStore != nil {
				if err := invCtx.CheckpointStore.Delete(invCtx.Context, invCtx.SessionID); err != nil {
					a.logger.Warn("checkpoint delete failed", "session", invCtx.SessionID, "error", err)
				}
			}
			// Plugin: AfterAgentRun.
			if invCtx.Plugins != nil {
				invCtx.Plugins.RunAfterAgentRun(invCtx.Context, inv, &plugin.RunResult{
					Rounds:     round + 1,
					ToolCalls:  totalToolCalls,
					DurationMs: time.Since(agentStart).Milliseconds(),
				})
			}
			return
		}

		// 4. Build assistant message with tool calls.
		var assistantBlocks []llm.ContentBlock
		if text := roundText.String(); text != "" {
			assistantBlocks = append(assistantBlocks, llm.TextBlock{Text: text})
		}
		for _, tc := range toolCalls {
			assistantBlocks = append(assistantBlocks, llm.ToolUseBlock{
				ID:    tc.ID,
				Name:  tc.Name,
				Input: tc.Input,
			})
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: assistantBlocks,
		})

		// 5. Execute tools.
		toolCtx := &tool.ToolContext{
			SessionID: invCtx.SessionID,
			TaskID:    taskIDFromSession(invCtx),
			AgentMode: mode,
			WorkDir:   workDir(invCtx),
			Bus:       invCtx.Bus,
			Cancel:    invCtx.Context,
			Memories:  invCtx.ToolMemories,
			Events:    invCtx.ToolEvents,
			Digest:    invCtx.ToolDigest,
			Journal:   invCtx.ToolJournal,
			Tasks:     invCtx.ToolTasks,
			Status:    invCtx.ToolStatus,
			Skills:    invCtx.ToolSkills,
			Notifier:  invCtx.ToolNotifier,
			Crons:     invCtx.ToolCrons,
			Config:    invCtx.ToolConfig,
			Subagents: invCtx.Subagents,
			ActivateSkill: func(name, content string, allowedTools []string) {
				if invCtx.Session != nil {
					invCtx.Session.ActiveSkills = append(invCtx.Session.ActiveSkills, ActiveSkill{
						Name:         name,
						Content:      content,
						AllowedTools: allowedTools,
					})
					skillActivated = true
				}
			},
		}

		roundToolCalls := 0
		for _, tc := range toolCalls {
			// Emit tool pending.
			publishSessionEvent(invCtx, "tool_call", map[string]any{
				"toolId": tc.ID, "toolName": tc.Name, "input": tc.Input, "round": round,
			})
			emit(invCtx.Context, ch, RunEvent{Event: &Event{
				ID:        newID(),
				SessionID: invCtx.SessionID,
				Timestamp: time.Now(),
				Author:    a.name,
				Round:     round,
				Parts: []Part{ToolPart{
					ToolName: tc.Name,
					CallID:   tc.ID,
					Status:   ToolRunning,
					Input:    tc.Input,
				}},
			}})

			// Plugin: BeforeToolCall.
			toolCallInfo := &plugin.ToolCall{Name: tc.Name, Input: tc.Input}
			if invCtx.Plugins != nil {
				var hookErr error
				invCtx.Context, hookErr = invCtx.Plugins.RunBeforeToolCall(invCtx.Context, toolCallInfo)
				if hookErr != nil {
					// Plugin blocked the tool call.
					emit(invCtx.Context, ch, RunEvent{Event: &Event{
						ID: newID(), SessionID: invCtx.SessionID, Timestamp: time.Now(),
						Author: a.name, Round: round,
						Parts: []Part{ToolPart{ToolName: tc.Name, CallID: tc.ID, Status: ToolFailed, Error: hookErr.Error()}},
					}})
					messages = append(messages, llm.Message{
						Role:    llm.RoleTool,
						Content: []llm.ContentBlock{llm.ToolResultBlock{ToolUseID: tc.ID, Content: hookErr.Error(), IsError: true}},
					})
					continue
				}
			}

			// Execute.
			start := time.Now()
			result, execErr := invCtx.Tools.Execute(toolCtx, tc.Name, tc.Input)
			duration := time.Since(start)
			totalToolCalls++

			var output, errStr string
			status := ToolCompleted
			if execErr != nil {
				status = ToolFailed
				errStr = execErr.Error()
				if invCtx.Plugins != nil {
					invCtx.Context = invCtx.Plugins.RunOnToolError(invCtx.Context, toolCallInfo, execErr)
				}
			} else if result.Error != "" {
				status = ToolFailed
				errStr = result.Error
				if invCtx.Plugins != nil {
					invCtx.Context = invCtx.Plugins.RunOnToolError(invCtx.Context, toolCallInfo, fmt.Errorf("%s", result.Error))
				}
			} else {
				output = result.Output
				if invCtx.Plugins != nil {
					invCtx.Context = invCtx.Plugins.RunAfterToolCall(invCtx.Context, toolCallInfo, &plugin.ToolResult{
						Output:   output,
						Duration: duration,
					})
				}
			}

			// Record tool call stats for the activity dashboard.
			// Use a short-lived background context so stats are recorded even if the
			// agent context was canceled, but can't block the loop if DB is slow.
			if invCtx.ActivityStore != nil {
				recordCtx, recordCancel := context.WithTimeout(context.Background(), time.Second)
				if recordErr := invCtx.ActivityStore.RecordToolCall(recordCtx, tc.Name, duration.Milliseconds(), errStr); recordErr != nil {
					a.logger.Warn("failed to record tool call", "tool", tc.Name, "error", recordErr)
				}
				recordCancel()
			}

			// Emit tool result with truncated output for observability.
			resultPayload := map[string]any{
				"toolId": tc.ID, "toolName": tc.Name, "isError": status == ToolFailed,
				"durationMs": duration.Milliseconds(), "round": round,
			}
			if errStr != "" {
				resultPayload["error"] = errStr
			}
			if output != "" {
				// Rune-safe truncation for the SSE stream.
				out := output
				if len(out) > 2000 {
					out = string([]rune(out)[:500]) + "\n... (truncated)"
				}
				resultPayload["output"] = out
			}
			publishSessionEvent(invCtx, "tool_result", resultPayload)

			// Emit file_change for file-modifying tools.
			if status != ToolFailed {
				emitFileChangeIfNeeded(invCtx, tc.Name, tc.Input, output)
			}
			roundToolCalls++
			emit(invCtx.Context, ch, RunEvent{Event: &Event{
				ID:        newID(),
				SessionID: invCtx.SessionID,
				Timestamp: time.Now(),
				Author:    a.name,
				Round:     round,
				Parts: []Part{ToolPart{
					ToolName: tc.Name,
					CallID:   tc.ID,
					Status:   status,
					Input:    tc.Input,
					Output:   output,
					Error:    errStr,
					Duration: duration,
				}},
			}})

			// Add tool result to messages for next LLM call (truncate large outputs).
			content := TruncateToolOutput(output, invCtx.CompactionConfig.MaxToolOutput)
			isError := false
			if status == ToolFailed {
				content = errStr
				isError = true
			}
			messages = append(messages, llm.Message{
				Role: llm.RoleTool,
				Content: []llm.ContentBlock{
					llm.ToolResultBlock{
						ToolUseID: tc.ID,
						Content:   content,
						IsError:   isError,
					},
				},
			})

			a.logger.Info("tool executed",
				"tool", tc.Name,
				"status", status,
				"duration", duration,
			)
		}

		// 5b. Emit round_complete session event.
		publishSessionEvent(invCtx, "round_complete", map[string]any{
			"round": round, "toolCalls": roundToolCalls,
			"inputTokens": inputTokens, "outputTokens": outputTokens,
		})

		// 5c. Checkpoint session state for crash recovery.
		if invCtx.CheckpointStore != nil {
			taskID, _ := invCtx.Session.State["taskId"].(string)
			if err := invCtx.CheckpointStore.Save(invCtx.Context, &SessionCheckpoint{
				SessionID:   invCtx.SessionID,
				TaskID:      taskID,
				Round:       round,
				Mode:        mode,
				MaxRounds:   maxRounds,
				UserMessage: invCtx.UserMessage,
				Origin:      invCtx.Origin,
				State:       invCtx.Session.State,
			}); err != nil {
				a.logger.Warn("checkpoint save failed", "session", invCtx.SessionID, "round", round, "error", err)
			}
		}

		// 6. If a skill was activated, rebuild prompt and tool defs for next round.
		if skillActivated {
			systemPrompt = BuildSystemPrompt(invCtx, modeConfig, invCtx.ContextBuilder, invCtx.JournalEntries)
			allowedTools = invCtx.Session.AllowedToolsFromSkills()
			toolDefs = invCtx.Tools.ForLLMFiltered(mode, allowedTools)
			skillActivated = false
			a.logger.Info("skill activated, rebuilt prompt and tools", "activeSkills", len(invCtx.Session.ActiveSkills))
		}

		// 7. Loop continues — LLM will see tool results.
	}

	// Max rounds exhausted — treat as abnormal termination.
	if invCtx.CheckpointStore != nil {
		if err := invCtx.CheckpointStore.Delete(invCtx.Context, invCtx.SessionID); err != nil {
			a.logger.Warn("checkpoint delete failed", "session", invCtx.SessionID, "error", err)
		}
	}
	publishSessionEvent(invCtx, "state_change", map[string]any{"state": "failed", "reason": "max_rounds"})
	if invCtx.Plugins != nil {
		invCtx.Plugins.RunOnAgentError(invCtx.Context, inv, fmt.Errorf("max rounds exhausted (%d)", maxRounds))
	}
	a.logger.Warn("max rounds exhausted", "maxRounds", maxRounds, "mode", mode)
	emit(invCtx.Context, ch, RunEvent{Event: &Event{
		ID:        newID(),
		SessionID: invCtx.SessionID,
		Timestamp: time.Now(),
		Author:    a.name,
		Parts:     []Part{TextPart{Text: "[max tool rounds reached]"}},
	}})
}

// taskIDFromSession extracts the task ID from session state (set by the agent loop).
func taskIDFromSession(ctx *InvocationContext) string {
	if ctx.Session != nil && ctx.Session.State != nil {
		if id, ok := ctx.Session.State["taskId"].(string); ok {
			return id
		}
	}
	return ""
}

// workDir returns the working directory for tool execution.
func workDir(ctx *InvocationContext) string {
	// Check session state for worktree path (set by coding tasks).
	if ctx.Session != nil && ctx.Session.State != nil {
		if wd, ok := ctx.Session.State["workDir"].(string); ok && wd != "" {
			return wd
		}
	}
	// Default to current directory (process cwd, typically the repo root).
	// Shell tool handles its own $HOME fallback for cross-repo access.
	return "."
}

// emit sends a RunEvent to the channel, respecting context cancellation.
func emit(ctx context.Context, ch chan<- RunEvent, ev RunEvent) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

// fileModifyingTools maps tool names to how we extract the file path from their input.
var fileModifyingTools = map[string]string{
	"cairn.writeFile":  "path",
	"cairn.editFile":   "path",
	"cairn.deleteFile": "path",
	"cairn.undoEdit":   "path",
}

// emitFileChangeIfNeeded checks if the tool modifies files and emits a file_change SessionEvent.
func emitFileChangeIfNeeded(invCtx *InvocationContext, toolName string, input json.RawMessage, output string) {
	pathKey, ok := fileModifyingTools[toolName]
	if !ok {
		return
	}
	var inp map[string]any
	if err := json.Unmarshal(input, &inp); err != nil {
		return
	}
	filePath, _ := inp[pathKey].(string)
	if filePath == "" {
		return
	}

	op := "write"
	diff := ""
	wd := workDir(invCtx)
	ctx := invCtx.Context

	// Try tracked file diff first, then untracked (new files).
	if out, err := exec.CommandContext(ctx, "git", "-C", wd, "diff", "--", filePath).Output(); err == nil && len(out) > 0 {
		diff = string(out)
	} else if out, err := exec.CommandContext(ctx, "git", "-C", wd, "diff", "--no-index", "/dev/null", filePath).CombinedOutput(); err != nil && len(out) > 0 {
		// --no-index exits non-zero when files differ, but still produces diff output.
		diff = string(out)
	}

	// Rune-safe truncation.
	if len(diff) > 10000 {
		runes := []rune(diff)
		if len(runes) > 3000 {
			runes = runes[:3000]
		}
		diff = string(runes) + "\n... (truncated)"
	}

	publishSessionEvent(invCtx, "file_change", map[string]any{
		"path": filePath, "operation": op, "diff": diff,
	})
}

// publishSessionEvent emits a SessionEvent on the bus for real-time observability.
func publishSessionEvent(invCtx *InvocationContext, eventType string, payload any) {
	if invCtx.Bus == nil {
		return
	}
	eventbus.Publish(invCtx.Bus, eventbus.SessionEvent{
		EventMeta: eventbus.NewMeta("agent"),
		SessionID: invCtx.SessionID,
		EventType: eventType,
		Payload:   payload,
	})
}

// hasWebSearchTool returns true if cairn.webSearch is among the tool definitions.
func hasWebSearchTool(tools []llm.ToolDef) bool {
	for _, t := range tools {
		if t.Name == "cairn.webSearch" {
			return true
		}
	}
	return false
}
