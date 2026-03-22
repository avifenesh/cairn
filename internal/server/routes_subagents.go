package server

import (
	"net/http"

	"github.com/avifenesh/cairn/internal/task"
)

// --- Subagent handlers ---

func (s *Server) handleListSubagents(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusOK, map[string]any{"subagents": []any{}})
		return
	}

	opts := task.ListOpts{
		Type:  task.TypeSubagent,
		Limit: 50,
	}
	if statusQ := r.URL.Query().Get("status"); statusQ != "" {
		opts.Status = task.TaskStatus(statusQ)
	}

	tasks, err := s.tasks.List(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter by parentTaskId if requested (ListOpts doesn't support this natively,
	// so we filter in-memory - subagent lists are small).
	if parentQ := r.URL.Query().Get("parentTaskId"); parentQ != "" {
		var filtered []*task.Task
		for _, t := range tasks {
			if t.ParentID == parentQ {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	writeJSON(w, http.StatusOK, map[string]any{"subagents": marshalTasks(tasks)})
}

func (s *Server) handleGetSubagent(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusNotFound, "task engine not available")
		return
	}

	id := r.PathValue("id")
	t, err := s.tasks.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "subagent not found")
		return
	}
	if t.Type != task.TypeSubagent {
		writeError(w, http.StatusNotFound, "not a subagent task")
		return
	}

	writeJSON(w, http.StatusOK, marshalTask(t))
}

func (s *Server) handleCancelSubagent(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "task engine not available")
		return
	}

	id := r.PathValue("id")
	if err := s.tasks.Cancel(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
