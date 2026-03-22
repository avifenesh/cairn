package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// --- Assistant / Sessions ---

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sessions": []any{}})
		return
	}

	limit := 50
	if limitQ := r.URL.Query().Get("limit"); limitQ != "" {
		if n, err := strconv.Atoi(limitQ); err == nil && n > 0 {
			limit = n
		}
	}

	sessions, err := s.sessions.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": marshalSessions(sessions)})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	session, err := s.sessions.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, marshalSession(session))
}

func (s *Server) handleAssistantMessage(w http.ResponseWriter, r *http.Request) {
	if s.agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent not available")
		return
	}
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "task engine not available")
		return
	}

	var req assistantMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	mode := tool.ModeTalk
	if req.Mode != "" {
		mode = tool.Mode(req.Mode)
	}

	// Create or load session.
	ctx := r.Context()
	var session *agent.Session

	if req.SessionID != "" && s.sessions != nil {
		var err error
		session, err = s.sessions.Get(ctx, req.SessionID)
		if err != nil {
			s.logger.Warn("failed to load session, creating new", "id", req.SessionID, "error", err)
			session = nil
		}
	}

	if session == nil {
		session = &agent.Session{
			Mode:  mode,
			State: map[string]any{"workDir": "."},
		}
		if s.sessions != nil {
			if err := s.sessions.Create(ctx, session); err != nil {
				s.logger.Warn("failed to create session", "error", err)
			}
		}
		if session.ID == "" {
			session.ID = "ephemeral"
		}
	}

	// Create a task for this assistant invocation.
	taskInput, _ := json.Marshal(map[string]string{
		"message":   req.Message,
		"sessionId": session.ID,
		"mode":      string(mode),
	})

	t, err := s.tasks.Submit(ctx, &task.SubmitRequest{
		Type:        task.TypeChat,
		Priority:    task.PriorityNormal,
		Mode:        string(mode),
		SessionID:   session.ID,
		Input:       taskInput,
		Description: truncate(req.Message, 100),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("task submit: %v", err))
		return
	}

	// Persist the user message in the session.
	if s.sessions != nil {
		userEvent := &agent.Event{
			SessionID: session.ID,
			Timestamp: time.Now(),
			Author:    "user",
			Parts:     []agent.Part{agent.TextPart{Text: req.Message}},
		}
		s.sessions.AppendEvent(ctx, session.ID, userEvent)
	}

	// Mark running immediately so the agent loop doesn't claim it.
	// Chat tasks are handled by the HTTP handler goroutine, not the loop.
	// If this fails, abort — running the agent without the DB update would
	// let the loop also claim and execute the same task.
	if err := s.tasks.MarkRunning(ctx, t.ID); err != nil {
		s.logger.Error("mark running failed, aborting chat task", "task", t.ID, "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("mark running: %v", err))
		return
	}

	// Run the agent asynchronously.
	go s.runAgent(session, t, req.Message, mode)

	// Return 202 with the task ID.
	writeJSON(w, http.StatusAccepted, map[string]any{
		"taskId":    t.ID,
		"sessionId": session.ID,
	})
}

// runAgent executes the agent in a background goroutine, publishing events
// to the bus for SSE broadcast.
func (s *Server) runAgent(session *agent.Session, t *task.Task, message string, mode tool.Mode) {
	ctx := context.Background()

	// Mark task as running.
	eventbus.Publish(s.bus, eventbus.TaskRunning{
		EventMeta: eventbus.NewMeta("server"),
		TaskID:    t.ID,
	})

	// Build journal entries for context (last 48h).
	var journalEntries []memory.JournalDigestEntry
	if s.journalStore != nil {
		if entries, err := s.journalStore.Recent(ctx, 48*time.Hour); err != nil {
			s.logger.Warn("journal entries failed, proceeding without", "error", err)
		} else {
			for _, e := range entries {
				journalEntries = append(journalEntries, memory.JournalDigestEntry{
					Summary:   e.Summary,
					Mode:      e.Mode,
					CreatedAt: e.CreatedAt,
					Learnings: e.Learnings,
					Errors:    e.Errors,
				})
			}
		}
	}

	// Create steering channel for this session (enables POST /v1/sessions/{id}/steer).
	steerCh := make(chan agent.SteeringMessage, 4)
	s.RegisterSteeringChannel(session.ID, steerCh)
	defer s.UnregisterSteeringChannel(session.ID)

	invCtx := &agent.InvocationContext{
		Context:        ctx,
		SessionID:      session.ID,
		UserMessage:    message,
		Mode:           mode,
		Session:        session,
		Tools:          s.tools,
		LLM:            s.llm,
		Memory:         s.memories,
		Soul:           s.soul,
		UserProfile:    s.userProfile,
		AgentsFile:     s.agentsFile,
		CuratedMemory:  s.curatedMemory,
		AgentTypes:     s.agentTypes,
		Bus:            s.bus,
		ContextBuilder: s.contextBuilder,
		JournalEntries: journalEntries,
		Plugins:        s.plugins,
		ActivityStore:  s.activityStore,
		SteeringCh:     steerCh,
		Subagents:      s.subagentRunner,
		ToolMemories:   s.toolMemories,
		ToolEvents:     s.toolEvents,
		ToolDigest:     s.toolDigest,
		ToolJournal:    s.toolJournal,
		ToolTasks:      s.toolTasks,
		ToolStatus:     s.toolStatus,
		ToolSkills:     s.toolSkills,
		ToolNotifier:   s.toolNotifier,
		ToolCrons:      s.toolCrons,
		ToolRules:      s.toolRules,
		ToolConfig:     s.toolConfig,
		Config: &agent.AgentConfig{
			Model:     s.config.LLMModel,
			MaxRounds: s.config.MaxRoundsForMode(string(mode)),
		},
		CheckpointStore: s.checkpointStore,
		Origin:          "chat",
	}

	var fullText strings.Builder

	for ev := range s.agent.Run(invCtx) {
		if ev.Err != nil {
			slog.Error("agent run error", "task", t.ID, "error", ev.Err)
			s.tasks.Fail(ctx, t.ID, ev.Err)
			return
		}
		if ev.Event == nil {
			continue
		}

		for _, part := range ev.Event.Parts {
			switch p := part.(type) {
			case agent.TextPart:
				if ev.Event.Author != "user" {
					fullText.WriteString(p.Text)
					// Publish text delta for SSE.
					eventbus.Publish(s.bus, eventbus.TextDelta{
						EventMeta: eventbus.NewMeta("agent"),
						TaskID:    t.ID,
						Text:      p.Text,
					})
				}
			case agent.ReasoningPart:
				eventbus.Publish(s.bus, eventbus.ReasoningDelta{
					EventMeta: eventbus.NewMeta("agent"),
					TaskID:    t.ID,
					Text:      p.Text,
					Round:     ev.Event.Round,
				})
			case agent.ToolPart:
				eventbus.Publish(s.bus, eventbus.ToolCallEvent{
					EventMeta: eventbus.NewMeta("agent"),
					TaskID:    t.ID,
					ToolName:  p.ToolName,
					Phase:     string(p.Status),
				})
			}
		}

		// Persist events to session store.
		if s.sessions != nil {
			s.sessions.AppendEvent(ctx, session.ID, ev.Event)
		}
	}

	// Complete the task.
	output, _ := json.Marshal(map[string]string{"text": fullText.String()})
	if err := s.tasks.Complete(ctx, t.ID, output); err != nil {
		slog.Error("failed to complete task", "task", t.ID, "error", err)
	}

	// Generate session title if empty (async, best-effort).
	if s.sessions != nil && s.llm != nil && session.Title == "" {
		go s.generateSessionTitle(session.ID, message, fullText.String())
	}
}

// generateSessionTitle uses a cheap LLM call to create a short title.
func (s *Server) generateSessionTitle(sessionID, userMsg, assistantMsg string) {
	s.logger.Info("generating session title", "session", sessionID, "userMsg", truncate(userMsg, 50))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &llm.Request{
		Model:           "glm-4.5-air",
		System:          "You generate very short conversation titles (3-6 words). Reply with ONLY the title text, nothing else.",
		MaxTokens:       20,
		DisableThinking: true,
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentBlock{llm.TextBlock{Text: fmt.Sprintf("User: %s\nAssistant: %s", truncate(userMsg, 150), truncate(assistantMsg, 150))}}},
		},
	}

	ch, err := s.llm.Stream(ctx, req)
	if err != nil {
		s.logger.Warn("title generation failed", "session", sessionID, "error", err)
		return
	}

	var title strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			title.WriteString(e.Text)
		case llm.StreamError:
			s.logger.Warn("title generation stream error", "session", sessionID, "error", e.Err)
		}
	}
	s.logger.Info("title generation completed", "session", sessionID, "title", title.String())

	t := strings.TrimSpace(title.String())
	if t == "" {
		return
	}
	// Cap at 60 chars
	if len(t) > 60 {
		t = t[:60]
	}

	if err := s.sessions.UpdateTitle(ctx, sessionID, t); err != nil {
		s.logger.Warn("failed to update session title", "session", sessionID, "error", err)
	}
}
