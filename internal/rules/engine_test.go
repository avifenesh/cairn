package rules

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
)

// mockNotifier captures notifications.
type mockNotifier struct {
	mu   sync.Mutex
	msgs []string
}

func (m *mockNotifier) Notify(_ context.Context, msg string, _ int) error {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
	return nil
}

func (m *mockNotifier) Messages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.msgs))
	copy(cp, m.msgs)
	return cp
}

// mockTasks captures task submissions.
type mockTasks struct {
	mu    sync.Mutex
	descs []string
}

func (m *mockTasks) Submit(_ context.Context, desc, _ string, _ int) (string, error) {
	m.mu.Lock()
	m.descs = append(m.descs, desc)
	m.mu.Unlock()
	return "task-123", nil
}

func (m *mockTasks) Descriptions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.descs))
	copy(cp, m.descs)
	return cp
}

func setupEngine(t *testing.T) (*Engine, *mockNotifier, *mockTasks, *eventbus.Bus) {
	t.Helper()
	store := NewStore(testDB(t))
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	notifier := &mockNotifier{}
	tasks := &mockTasks{}

	engine := NewEngine(EngineDeps{
		Store:    store,
		Bus:      bus,
		Notifier: notifier,
		Tasks:    tasks,
	})
	return engine, notifier, tasks, bus
}

func TestEngine_EventTrigger(t *testing.T) {
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	// Create a rule that triggers on EventIngested from github.
	rule := &Rule{
		Name:    "github-notify",
		Enabled: true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
			Filter:    map[string]string{"sourceType": "github"},
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "New: {{.title}}", "priority": "1"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	// Publish a matching event.
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta:  eventbus.NewMeta("github"),
		SourceType: "github",
		Title:      "PR #42 opened",
		URL:        "https://github.com/test/repo/pull/42",
	})

	// Wait for async processing.
	time.Sleep(200 * time.Millisecond)

	msgs := notifier.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(msgs))
	}
	if msgs[0] != "New: PR #42 opened" {
		t.Errorf("expected 'New: PR #42 opened', got %q", msgs[0])
	}

	// Verify execution was logged.
	execs, _ := engine.store.ListExecutions(ctx, rule.ID, 10)
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
	if execs[0].Status != ExecSuccess {
		t.Errorf("expected status 'success', got %q", execs[0].Status)
	}
}

func TestEngine_FilterMismatch(t *testing.T) {
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "github-only",
		Enabled: true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
			Filter:    map[string]string{"sourceType": "github"},
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "test"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	// Publish a non-matching event (reddit, not github).
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta:  eventbus.NewMeta("reddit"),
		SourceType: "reddit",
		Title:      "Reddit post",
	})

	time.Sleep(100 * time.Millisecond)

	if len(notifier.Messages()) != 0 {
		t.Error("expected no notification for non-matching filter")
	}
}

func TestEngine_ConditionEvaluation(t *testing.T) {
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "pr-only",
		Enabled: true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
			Filter:    map[string]string{"sourceType": "github"},
		},
		Condition: `title contains "PR"`,
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "PR detected"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	// Event that matches filter but NOT condition.
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta:  eventbus.NewMeta("github"),
		SourceType: "github",
		Title:      "Issue #10 created",
	})
	time.Sleep(100 * time.Millisecond)
	if len(notifier.Messages()) != 0 {
		t.Error("expected no notification when condition is false")
	}

	// Event that matches both.
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta:  eventbus.NewMeta("github"),
		SourceType: "github",
		Title:      "PR #42 opened",
	})
	time.Sleep(200 * time.Millisecond)
	if len(notifier.Messages()) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.Messages()))
	}
}

func TestEngine_Throttle(t *testing.T) {
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "throttled",
		Enabled: true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "fired"}},
		},
		ThrottleMs: 60000, // 1 minute
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	// First event should fire.
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta: eventbus.NewMeta("test"),
		Title:     "event 1",
	})
	time.Sleep(200 * time.Millisecond)
	if len(notifier.Messages()) != 1 {
		t.Fatalf("expected 1 notification after first event, got %d", len(notifier.Messages()))
	}

	// Second event should be throttled (within 60s).
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta: eventbus.NewMeta("test"),
		Title:     "event 2",
	})
	time.Sleep(200 * time.Millisecond)
	if len(notifier.Messages()) != 1 {
		t.Errorf("expected still 1 notification (throttled), got %d", len(notifier.Messages()))
	}
}

func TestEngine_DisabledRule(t *testing.T) {
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "disabled",
		Enabled: false,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "should not fire"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta: eventbus.NewMeta("test"),
		Title:     "event",
	})
	time.Sleep(100 * time.Millisecond)

	if len(notifier.Messages()) != 0 {
		t.Error("disabled rule should not fire")
	}
}

func TestEngine_TaskAction(t *testing.T) {
	engine, _, tasks, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "auto-task",
		Enabled: true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "TaskFailed",
		},
		Actions: []Action{
			{Type: ActionTask, Params: map[string]string{
				"description": "Investigate failed task: {{.taskId}}",
				"type":        "general",
			}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	eventbus.Publish(bus, eventbus.TaskFailed{
		EventMeta: eventbus.NewMeta("agent"),
		TaskID:    "task-456",
		Error:     "timeout",
	})
	time.Sleep(200 * time.Millisecond)

	descs := tasks.Descriptions()
	if len(descs) != 1 {
		t.Fatalf("expected 1 task submitted, got %d", len(descs))
	}
	if descs[0] != "Investigate failed task: task-456" {
		t.Errorf("expected task description with substitution, got %q", descs[0])
	}
}

func TestMatchFilter(t *testing.T) {
	data := map[string]any{
		"sourceType": "github",
		"title":      "PR #42",
	}

	if !matchFilter(nil, data) {
		t.Error("nil filter should match")
	}
	if !matchFilter(map[string]string{}, data) {
		t.Error("empty filter should match")
	}
	if !matchFilter(map[string]string{"sourceType": "github"}, data) {
		t.Error("matching filter should match")
	}
	if matchFilter(map[string]string{"sourceType": "reddit"}, data) {
		t.Error("non-matching filter should not match")
	}
	if matchFilter(map[string]string{"missing": "key"}, data) {
		t.Error("missing key should not match")
	}
}

func TestExpandTemplate(t *testing.T) {
	data := map[string]any{
		"title": "PR #42",
		"url":   "https://example.com",
	}

	got := expandTemplate("New: {{.title}} at {{.url}}", data)
	want := "New: PR #42 at https://example.com"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}

	// No substitution.
	got = expandTemplate("plain text", data)
	if got != "plain text" {
		t.Errorf("expected 'plain text', got %q", got)
	}
}

func TestEngine_ConditionNonBool(t *testing.T) {
	// Condition that returns a non-bool (string) should not pass AsBool() compilation.
	// expr.Compile with AsBool() rejects non-bool expressions at compile time,
	// so the rule is skipped during cache refresh. No notification should fire.
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "non-bool-condition",
		Enabled: true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
		},
		Condition: `"hello"`,
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "should not fire"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta:  eventbus.NewMeta("test"),
		SourceType: "test",
		Title:      "test event",
	})
	time.Sleep(200 * time.Millisecond)

	if len(notifier.Messages()) != 0 {
		t.Error("expected no notification for non-bool condition")
	}
}

func TestEngine_NilNotifier(t *testing.T) {
	store := NewStore(testDB(t))
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	engine := NewEngine(EngineDeps{
		Store:    store,
		Bus:      bus,
		Notifier: nil,
		Tasks:    &mockTasks{},
	})
	ctx := context.Background()

	rule := &Rule{
		Name:    "nil-notifier",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "test"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta: eventbus.NewMeta("test"),
		Title:     "test",
	})
	time.Sleep(200 * time.Millisecond)

	execs, _ := engine.store.ListExecutions(ctx, rule.ID, 10)
	if len(execs) == 0 {
		t.Fatal("expected at least 1 execution record")
	}
	if execs[0].Status != ExecError {
		t.Errorf("expected status 'error', got %q", execs[0].Status)
	}
}

func TestEngine_NilTasks(t *testing.T) {
	store := NewStore(testDB(t))
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	engine := NewEngine(EngineDeps{
		Store:    store,
		Bus:      bus,
		Notifier: &mockNotifier{},
		Tasks:    nil,
	})
	ctx := context.Background()

	rule := &Rule{
		Name:    "nil-tasks",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{
			{Type: ActionTask, Params: map[string]string{"description": "do something"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta: eventbus.NewMeta("test"),
		Title:     "test",
	})
	time.Sleep(200 * time.Millisecond)

	execs, _ := engine.store.ListExecutions(ctx, rule.ID, 10)
	if len(execs) == 0 {
		t.Fatal("expected at least 1 execution record")
	}
	if execs[0].Status != ExecError {
		t.Errorf("expected status 'error', got %q", execs[0].Status)
	}
}

func TestEngine_CronRuleIgnoredOnEvent(t *testing.T) {
	engine, notifier, _, bus := setupEngine(t)
	ctx := context.Background()

	rule := &Rule{
		Name:    "cron-rule",
		Enabled: true,
		Trigger: Trigger{
			Type:     TriggerCron,
			Schedule: "0 * * * *",
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "cron fired"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	// Publish an event - cron rules should NOT match event triggers.
	eventbus.Publish(bus, eventbus.EventIngested{
		EventMeta: eventbus.NewMeta("test"),
		Title:     "test",
	})
	time.Sleep(200 * time.Millisecond)

	if len(notifier.Messages()) != 0 {
		t.Error("cron rule should not fire on bus events")
	}
}

func TestEngine_Stats(t *testing.T) {
	store := NewStore(testDB(t))
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	engine := NewEngine(EngineDeps{
		Store:    store,
		Bus:      bus,
		Notifier: &mockNotifier{},
		Tasks:    &mockTasks{},
	})
	ctx := context.Background()

	// Create 2 rules: 1 enabled, 1 disabled.
	r1 := &Rule{
		Name:    "enabled-rule",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "x"}}},
	}
	r2 := &Rule{
		Name:    "disabled-rule",
		Enabled: false,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "y"}}},
	}
	store.Create(ctx, r1)
	store.Create(ctx, r2)

	// Record some executions.
	store.RecordExecution(ctx, &Execution{RuleID: r1.ID, Status: ExecSuccess})
	store.RecordExecution(ctx, &Execution{RuleID: r1.ID, Status: ExecError, Error: "fail1"})
	store.RecordExecution(ctx, &Execution{RuleID: r1.ID, Status: ExecError, Error: "fail2"})

	engine.Start()
	defer engine.Close()

	total, enabled, failures := engine.Stats()
	if total != 2 {
		t.Errorf("total: want 2, got %d", total)
	}
	if enabled != 1 {
		t.Errorf("enabled: want 1, got %d", enabled)
	}
	if failures != 2 {
		t.Errorf("recentFailures: want 2, got %d", failures)
	}
}

func TestEngine_ConcurrentAccess(t *testing.T) {
	engine, _, _, bus := setupEngine(t)
	ctx := context.Background()

	// Create a rule to have something in cache.
	rule := &Rule{
		Name:    "concurrent-test",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "concurrent"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eventbus.Publish(bus, eventbus.EventIngested{
				EventMeta: eventbus.NewMeta("test"),
				Title:     "concurrent event",
			})
			engine.RefreshCache()
		}()
	}
	wg.Wait()
	// If we reach here without panic (and pass -race), the test passes.
}

func TestExpandTemplate_EdgeCases(t *testing.T) {
	// Missing key: placeholder should be preserved.
	got := expandTemplate("Hello {{.missing}}", map[string]any{"title": "PR"})
	if got != "Hello {{.missing}}" {
		t.Errorf("missing key: expected placeholder preserved, got %q", got)
	}

	// Nil value in data: should expand to "<nil>".
	got = expandTemplate("Value: {{.key}}", map[string]any{"key": nil})
	if got != "Value: <nil>" {
		t.Errorf("nil value: expected 'Value: <nil>', got %q", got)
	}

	// Multiple occurrences of same key.
	got = expandTemplate("{{.x}} and {{.x}}", map[string]any{"x": "hello"})
	if got != "hello and hello" {
		t.Errorf("multiple occurrences: expected 'hello and hello', got %q", got)
	}
}

func TestEngine_CronTick(t *testing.T) {
	engine, notifier, _, _ := setupEngine(t)
	ctx := context.Background()

	// Create a cron rule that fires every minute ("* * * * *").
	rule := &Rule{
		Name:    "every-minute",
		Enabled: true,
		Trigger: Trigger{
			Type:     TriggerCron,
			Schedule: "* * * * *",
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "cron fired"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	// Directly invoke cronTick - should fire the rule.
	engine.cronTick()
	time.Sleep(200 * time.Millisecond)

	msgs := notifier.Messages()
	if len(msgs) == 0 {
		t.Fatal("cron rule should have fired on cronTick")
	}
	if msgs[0] != "cron fired" {
		t.Errorf("message = %q, want 'cron fired'", msgs[0])
	}
}

func TestEngine_CronTickNotDue(t *testing.T) {
	engine, notifier, _, _ := setupEngine(t)
	ctx := context.Background()

	// Create a rule that fires at a specific time far in the future.
	rule := &Rule{
		Name:    "future-rule",
		Enabled: true,
		Trigger: Trigger{
			Type:     TriggerCron,
			Schedule: "0 0 1 1 *", // Jan 1 at midnight
		},
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "should not fire"}},
		},
	}
	engine.store.Create(ctx, rule)
	engine.Start()
	defer engine.Close()

	engine.cronTick()
	time.Sleep(200 * time.Millisecond)

	if len(notifier.Messages()) != 0 {
		t.Error("future cron rule should not have fired")
	}
}
