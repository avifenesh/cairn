package cron

import (
	"testing"
	"time"
)

func TestValidate_Valid(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 9 * * 1-5",
		"*/30 * * * *",
		"0 0 1 * *",
		"0 9,17 * * *",
		"0 */4 * * *",
	}
	for _, expr := range valid {
		if err := Validate(expr); err != nil {
			t.Errorf("expected valid: %q, got error: %v", expr, err)
		}
	}
}

func TestValidate_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"not a cron",
		"* * *",         // too few fields
		"60 * * * *",    // minute out of range
		"* 25 * * *",    // hour out of range
		"0 0 0 0 0 0 0", // too many fields
	}
	for _, expr := range invalid {
		if err := Validate(expr); err == nil {
			t.Errorf("expected invalid: %q, got nil error", expr)
		}
	}
}

func TestNextRun(t *testing.T) {
	// "0 9 * * *" = daily at 9am UTC
	after := time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)
	next, err := NextRun("0 9 * * *", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextRun_AfterMatch(t *testing.T) {
	// If after is exactly at 9am, next should be tomorrow 9am.
	after := time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC)
	next, err := NextRun("0 9 * * *", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestIsDue(t *testing.T) {
	// Job fires every minute. After 10:00:00, now is 10:01:30 → should be due.
	after := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 19, 10, 1, 30, 0, time.UTC)
	due, err := IsDue("* * * * *", after, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !due {
		t.Error("expected due")
	}
}

func TestIsDue_NotYet(t *testing.T) {
	// Job fires at 9am. After 8:00, now is 8:30 → not due yet.
	after := time.Date(2026, 3, 19, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 19, 8, 30, 0, 0, time.UTC)
	due, err := IsDue("0 9 * * *", after, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if due {
		t.Error("expected not due")
	}
}

func TestNextRun_InvalidExpr(t *testing.T) {
	_, err := NextRun("invalid", time.Now())
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}
