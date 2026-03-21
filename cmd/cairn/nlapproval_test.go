package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	cairnchannel "github.com/avifenesh/cairn/internal/channel"
	cairndb "github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
)

// --- Parser tests (pure, no DB) ---

func TestParseApprovalIntent(t *testing.T) {
	tests := []struct {
		input      string
		wantNil    bool
		wantAction ApprovalAction
		wantTarget ApprovalTarget
		wantAll    bool
		wantID     string
	}{
		// Approve with target.
		{"approve the memory", false, ActionApprove, TargetMemory, false, ""},
		{"accept that memory", false, ActionApprove, TargetMemory, false, ""},
		{"approve the soul patch", false, ActionApprove, TargetSoulPatch, false, ""},
		{"apply it", false, ActionApprove, TargetUnknown, false, ""},
		{"looks good", false, ActionApprove, TargetUnknown, false, ""},
		{"lgtm", false, ActionApprove, TargetUnknown, false, ""},
		{"ship it", false, ActionApprove, TargetUnknown, false, ""},
		{"go ahead", false, ActionApprove, TargetUnknown, false, ""},

		// Bare approve signals.
		{"yes", false, ActionApprove, TargetUnknown, false, ""},
		{"yep", false, ActionApprove, TargetUnknown, false, ""},
		{"yeah", false, ActionApprove, TargetUnknown, false, ""},
		{"sure", false, ActionApprove, TargetUnknown, false, ""},
		{"ok", false, ActionApprove, TargetUnknown, false, ""},
		{"okay", false, ActionApprove, TargetUnknown, false, ""},
		{"confirm", false, ActionApprove, TargetUnknown, false, ""},

		// Deny with target.
		{"deny the memory", false, ActionDeny, TargetMemory, false, ""},
		{"reject that memory", false, ActionDeny, TargetMemory, false, ""},
		{"deny the soul patch", false, ActionDeny, TargetSoulPatch, false, ""},
		{"cancel the approval", false, ActionDeny, TargetApproval, false, ""},

		// Bare deny signals.
		{"no", false, ActionDeny, TargetUnknown, false, ""},
		{"nope", false, ActionDeny, TargetUnknown, false, ""},
		{"nah", false, ActionDeny, TargetUnknown, false, ""},
		{"pass", false, ActionDeny, TargetUnknown, false, ""},
		{"skip", false, ActionDeny, TargetUnknown, false, ""},
		{"reject it", false, ActionDeny, TargetUnknown, false, ""},
		{"discard", false, ActionDeny, TargetUnknown, false, ""},
		{"drop it", false, ActionDeny, TargetUnknown, false, ""},

		// Show.
		{"what's pending", false, ActionShow, TargetUnknown, false, ""},
		{"show pending", false, ActionShow, TargetUnknown, false, ""},
		{"list pending", false, ActionShow, TargetUnknown, false, ""},

		// All pattern.
		{"approve all memories", false, ActionApprove, TargetMemory, true, ""},
		{"approve all proposed memories", false, ActionApprove, TargetMemory, true, ""},
		{"accept everything", false, ActionApprove, TargetUnknown, true, ""},
		{"reject all", false, ActionDeny, TargetUnknown, true, ""},

		// With IDs.
		{"approve mem_a1b2c3d4e5f6", false, ActionApprove, TargetMemory, false, "mem_a1b2c3d4e5f6"},
		{"deny apr_deadbeef1234", false, ActionDeny, TargetApproval, false, "apr_deadbeef1234"},
		{"approve a1b2c3d4e5f6a7b8", false, ActionApprove, TargetUnknown, false, "a1b2c3d4e5f6a7b8"},

		// Not approval intents — should return nil.
		{"what is the weather", true, 0, 0, false, ""},
		{"tell me about Go", true, 0, 0, false, ""},
		{"how do goroutines work", true, 0, 0, false, ""},
		{"search for svelte docs", true, 0, 0, false, ""},
		{"create a task", true, 0, 0, false, ""},
		{"", true, 0, 0, false, ""},

		// Edge: should NOT match "no" inside "notable" or "know".
		{"that's notable work", true, 0, 0, false, ""},
		{"I know that", true, 0, 0, false, ""},

		// Edge: "pass" inside longer sentences should not match.
		{"pass the butter", true, 0, 0, false, ""},
		{"password reset", true, 0, 0, false, ""},
		// But bare "pass" should match.
		{"pass", false, ActionDeny, TargetUnknown, false, ""},

		// Case insensitive.
		{"APPROVE the Memory", false, ActionApprove, TargetMemory, false, ""},
		{"Yes", false, ActionApprove, TargetUnknown, false, ""},
		{"NO", false, ActionDeny, TargetUnknown, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseApprovalIntent(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseApprovalIntent(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseApprovalIntent(%q) = nil, want non-nil", tt.input)
			}
			if got.Action != tt.wantAction {
				t.Errorf("Action = %d, want %d", got.Action, tt.wantAction)
			}
			if got.Target != tt.wantTarget {
				t.Errorf("Target = %d, want %d", got.Target, tt.wantTarget)
			}
			if got.All != tt.wantAll {
				t.Errorf("All = %v, want %v", got.All, tt.wantAll)
			}
			if got.TargetID != tt.wantID {
				t.Errorf("TargetID = %q, want %q", got.TargetID, tt.wantID)
			}
		})
	}
}

func TestContainsAnyWord(t *testing.T) {
	tests := []struct {
		text  string
		words []string
		want  bool
	}{
		{"yes please", []string{"yes"}, true},
		{"notable work", []string{"no"}, false},
		{"I know that", []string{"no"}, false},
		{"password", []string{"pass"}, false},
		{"pass the butter", []string{"pass"}, true},
		{"ok", []string{"ok"}, true},
		{"ok,", []string{"ok"}, true},       // punctuation stripped
		{"yes!", []string{"yes"}, true},      // punctuation stripped
		{"hello world", []string{"ok"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := containsAnyWord(tt.text, tt.words)
			if got != tt.want {
				t.Errorf("containsAnyWord(%q, %v) = %v, want %v", tt.text, tt.words, got, tt.want)
			}
		})
	}
}

// --- Integration tests with in-memory DB ---

func openTestDB(t *testing.T) *cairndb.DB {
	t.Helper()
	d, err := cairndb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestMemoryService creates a memory service with a noop embedder for testing.
func newTestMemoryService(t *testing.T) *memory.Service {
	t.Helper()
	d := openTestDB(t)
	store := memory.NewStore(d)
	return memory.NewService(store, memory.NoopEmbedder{}, nil)
}

func TestHandleApprovalIntent_SingleMemory(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	mem := &memory.Memory{Content: "Go uses goroutines", Category: "fact", Scope: "global"}
	if err := svc.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}

	// Approve with target=memory, no ID, single proposed → should accept it.
	intent := &ApprovalIntent{Action: ActionApprove, Target: TargetMemory}
	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "accepted")

	// Verify memory is accepted.
	got, err := svc.Get(ctx, mem.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != memory.StatusAccepted {
		t.Errorf("Status = %s, want accepted", got.Status)
	}
}

func TestHandleApprovalIntent_RejectMemory(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	mem := &memory.Memory{Content: "Old preference", Category: "preference", Scope: "global"}
	if err := svc.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}

	intent := &ApprovalIntent{Action: ActionDeny, Target: TargetMemory}
	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "rejected")
}

func TestHandleApprovalIntent_MultipleMemories(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	for _, content := range []string{"Fact A", "Fact B", "Fact C"} {
		if err := svc.Create(ctx, &memory.Memory{Content: content, Category: "fact", Scope: "global"}); err != nil {
			t.Fatal(err)
		}
	}

	// No ID, multiple proposed → should list them.
	intent := &ApprovalIntent{Action: ActionApprove, Target: TargetMemory}
	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "proposed memories")
	assertContains(t, resp.Text, "which one")
}

func TestHandleApprovalIntent_ApproveAll(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	for _, content := range []string{"Fact A", "Fact B", "Fact C"} {
		if err := svc.Create(ctx, &memory.Memory{Content: content, Category: "fact", Scope: "global"}); err != nil {
			t.Fatal(err)
		}
	}

	intent := &ApprovalIntent{Action: ActionApprove, Target: TargetMemory, All: true}
	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "3/3")

	// Verify all accepted.
	mems, _ := svc.List(ctx, memory.ListOpts{Status: memory.StatusProposed})
	if len(mems) != 0 {
		t.Errorf("Still %d proposed memories, want 0", len(mems))
	}
}

func TestHandleApprovalIntent_UnknownTarget_SinglePending(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	mem := &memory.Memory{Content: "Single fact", Category: "fact", Scope: "global"}
	if err := svc.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}

	// Bare "yes" with 1 pending item → should accept it.
	intent := &ApprovalIntent{Action: ActionApprove, Target: TargetUnknown}
	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "accepted")
}

func TestHandleApprovalIntent_UnknownTarget_NothingPending(t *testing.T) {
	ctx := context.Background()
	intent := &ApprovalIntent{Action: ActionApprove, Target: TargetUnknown}

	// No memory service, no soul, no approvals → nothing pending.
	resp, err := handleApprovalIntent(ctx, intent, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "Nothing pending")
}

func TestHandleApprovalIntent_ShowPending(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	if err := svc.Create(ctx, &memory.Memory{Content: "A fact", Category: "fact", Scope: "global"}); err != nil {
		t.Fatal(err)
	}

	intent := &ApprovalIntent{Action: ActionShow}
	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "Pending items")
}

func TestHandleCallbackData_Approval(t *testing.T) {
	d := openTestDB(t)
	approvalStore := task.NewApprovalStore(d.DB)

	ctx := context.Background()
	approval := &task.Approval{
		Type:        "soul_patch",
		Description: "Test approval",
	}
	if err := approvalStore.Create(ctx, approval); err != nil {
		t.Fatal(err)
	}

	resp, err := handleCallbackData(ctx, "approve:"+approval.ID, nil, nil, approvalStore)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "approved")

	// Verify it's approved.
	got, err := approvalStore.Get(ctx, approval.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != task.ApprovalApproved {
		t.Errorf("Status = %s, want approved", got.Status)
	}
}

func TestHandleCallbackData_InvalidFormat(t *testing.T) {
	ctx := context.Background()
	resp, err := handleCallbackData(ctx, "baddata", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "Invalid callback")
}

func TestHandleCallbackData_Memory(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	mem := &memory.Memory{Content: "Test memory", Category: "fact", Scope: "global"}
	if err := svc.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}

	// Use the full memory ID in callback data.
	resp, err := handleCallbackData(ctx, "approve:"+mem.ID, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "accepted")
}

func TestHandleCallbackData_NotFound(t *testing.T) {
	ctx := context.Background()
	resp, err := handleCallbackData(ctx, "approve:nonexistent_id", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "No pending item")
}

// --- Test the full NL flow end-to-end ---

func TestNLApprovalEndToEnd(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	mem := &memory.Memory{Content: "Go conventions", Category: "fact", Scope: "global"}
	if err := svc.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}

	// Simulate: user says "yes" → parser → resolver → memory accepted.
	intent := parseApprovalIntent("yes")
	if intent == nil {
		t.Fatal("parseApprovalIntent(\"yes\") = nil")
	}

	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "accepted")

	got, _ := svc.Get(ctx, mem.ID)
	if got.Status != memory.StatusAccepted {
		t.Errorf("Status = %s, want accepted", got.Status)
	}
}

func TestNLApprovalEndToEnd_DenyWithTarget(t *testing.T) {
	svc := newTestMemoryService(t)

	ctx := context.Background()
	mem := &memory.Memory{Content: "Bad fact", Category: "fact", Scope: "global"}
	if err := svc.Create(ctx, mem); err != nil {
		t.Fatal(err)
	}

	intent := parseApprovalIntent("no, reject that memory")
	if intent == nil {
		t.Fatal("parseApprovalIntent returned nil")
	}
	if intent.Action != ActionDeny || intent.Target != TargetMemory {
		t.Fatalf("got Action=%d Target=%d, want Deny+Memory", intent.Action, intent.Target)
	}

	resp, err := handleApprovalIntent(ctx, intent, svc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, resp.Text, "rejected")
}

// assertContains is a test helper for checking response text.
func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !containsCI(got, want) {
		t.Errorf("response %q does not contain %q", got, want)
	}
}

func containsCI(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// Verify OutgoingMessage type is used (compile check).
var _ *cairnchannel.OutgoingMessage
var _ *sql.DB
