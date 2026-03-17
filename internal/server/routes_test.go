package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
)

// newTestServerWithDB creates a test server backed by an in-memory SQLite database.
func newTestServerWithDB(t *testing.T, cfg *config.Config) (*Server, *httptest.Server, *db.DB) {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{
			WriteAPIToken: "test-write-token",
		}
	}

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bus := eventbus.New()
	t.Cleanup(bus.Close)

	logger := slog.Default()

	// Create stores.
	sessionStore := agent.NewSessionStore(database)
	memStore := memory.NewStore(database)
	memService := memory.NewService(memStore, memory.NoopEmbedder{}, bus)
	taskStore := task.NewStore(database)
	taskEngine := task.NewEngine(taskStore, bus, nil)
	t.Cleanup(taskEngine.Close)

	srv := New(ServerConfig{
		Sessions: sessionStore,
		Tasks:    taskEngine,
		Memories: memService,
		Bus:      bus,
		Config:   cfg,
		Logger:   logger,
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return srv, ts, database
}

// authRequest creates an HTTP request with the write token set.
func authRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequest(method, url, bytes.NewBufferString(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-Api-Token", "test-write-token")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestListTasks(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("GET /v1/tasks: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if _, ok := body["tasks"]; !ok {
		t.Fatal("expected 'tasks' key in response")
	}
}

func TestListTasks_WithData(t *testing.T) {
	srv, ts, _ := newTestServerWithDB(t, nil)

	// Create a task via the engine.
	ctx := context.Background()
	submitted, err := srv.tasks.Submit(ctx, &task.SubmitRequest{
		Type:        task.TypeChat,
		Priority:    task.PriorityNormal,
		Mode:        "talk",
		Description: "test task",
		Input:       json.RawMessage(`{"message":"hello"}`),
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}

	resp, err := http.Get(ts.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("GET /v1/tasks: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	tasks, ok := body["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		t.Fatal("expected at least one task")
	}

	taskObj, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatal("expected task to be an object")
	}

	if taskObj["id"] != submitted.ID {
		t.Fatalf("expected task id %q, got %v", submitted.ID, taskObj["id"])
	}

	if taskObj["type"] != "chat" {
		t.Fatalf("expected type 'chat', got %v", taskObj["type"])
	}

	if taskObj["status"] != "queued" {
		t.Fatalf("expected status 'queued', got %v", taskObj["status"])
	}
}

func TestListMemories(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/memories")
	if err != nil {
		t.Fatalf("GET /v1/memories: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if _, ok := body["memories"]; !ok {
		t.Fatal("expected 'memories' key in response")
	}
}

func TestCreateMemory(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	reqBody := `{"content":"Go is a great language","category":"fact","scope":"global"}`
	req := authRequest(t, "POST", ts.URL+"/v1/memories", reqBody)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/memories: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected 201, got %d: %v", resp.StatusCode, body)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok:true, got %v", body)
	}

	mem, ok := body["memory"].(map[string]any)
	if !ok {
		t.Fatal("expected 'memory' object in response")
	}

	if mem["content"] != "Go is a great language" {
		t.Fatalf("expected content match, got %v", mem["content"])
	}

	// Verify it appears in the list.
	listResp, listErr := http.Get(ts.URL + "/v1/memories")
	if listErr != nil {
		t.Fatalf("GET /v1/memories: %v", listErr)
	}
	defer listResp.Body.Close()

	var listBody map[string]any
	json.NewDecoder(listResp.Body).Decode(&listBody)

	mems, ok := listBody["memories"].([]any)
	if !ok || len(mems) == 0 {
		t.Fatal("expected at least one memory in list")
	}
}

func TestListSessions(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/assistant/sessions")
	if err != nil {
		t.Fatalf("GET /v1/assistant/sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if _, ok := body["sessions"]; !ok {
		t.Fatal("expected 'sessions' key in response")
	}
}

func TestListSessions_WithData(t *testing.T) {
	srv, ts, _ := newTestServerWithDB(t, nil)

	// Create a session directly.
	ctx := context.Background()
	session := &agent.Session{
		Title: "Test Session",
		Mode:  "talk",
		State: map[string]any{},
	}
	if err := srv.sessions.Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	resp, err := http.Get(ts.URL + "/v1/assistant/sessions")
	if err != nil {
		t.Fatalf("GET /v1/assistant/sessions: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	sessions, ok := body["sessions"].([]any)
	if !ok || len(sessions) == 0 {
		t.Fatal("expected at least one session")
	}
}

func TestAssistantMessage_NoAgent(t *testing.T) {
	// Server without agent configured -> 503.
	_, ts, _ := newTestServerWithDB(t, nil)

	reqBody := `{"message":"hello","mode":"talk"}`
	req := authRequest(t, "POST", ts.URL+"/v1/assistant/message", reqBody)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/assistant/message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no agent), got %d", resp.StatusCode)
	}
}

func TestAssistantMessage_EmptyMessage(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	reqBody := `{"message":"","mode":"talk"}`
	req := authRequest(t, "POST", ts.URL+"/v1/assistant/message", reqBody)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/assistant/message: %v", err)
	}
	defer resp.Body.Close()

	// Either 400 (empty message) or 503 (no agent) is acceptable.
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 400 or 503, got %d", resp.StatusCode)
	}
}

func TestGetSoul_NotConfigured(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/soul")
	if err != nil {
		t.Fatalf("GET /v1/soul: %v", err)
	}
	defer resp.Body.Close()

	// No soul configured -> 404.
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestListSkills(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/skills")
	if err != nil {
		t.Fatalf("GET /v1/skills: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	if _, ok := body["skills"]; !ok {
		t.Fatal("expected 'skills' key in response")
	}
}

func TestStatus(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/status")
	if err != nil {
		t.Fatalf("GET /v1/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok:true in status, got %v", body)
	}
}

func TestCosts(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/costs")
	if err != nil {
		t.Fatalf("GET /v1/costs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSearchMemories_MissingQuery(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	resp, err := http.Get(ts.URL + "/v1/memories/search")
	if err != nil {
		t.Fatalf("GET /v1/memories/search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing q, got %d", resp.StatusCode)
	}
}

func TestCancelTask_MissingID(t *testing.T) {
	_, ts, _ := newTestServerWithDB(t, nil)

	req := authRequest(t, "POST", ts.URL+"/v1/tasks//cancel", "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	defer resp.Body.Close()

	// Should fail with an error (bad request or internal error).
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected error for empty task id")
	}
}
