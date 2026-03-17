package llm

import (
	"testing"
)

func TestBudget_CanAfford(t *testing.T) {
	b := NewBudget(1.00, 10.00) // $1/day, $10/week

	// GPT-4o: $2.50/1M input. 100k tokens = $0.25
	if !b.CanAfford("gpt-4o", 100_000) {
		t.Error("should be able to afford 100k tokens")
	}

	// 500k tokens = $1.25 — over daily limit
	if b.CanAfford("gpt-4o", 500_000) {
		t.Error("should NOT afford 500k tokens on $1 daily budget")
	}
}

func TestBudget_Record(t *testing.T) {
	b := NewBudget(1.00, 10.00)

	// Record 100k input + 50k output for gpt-4o
	// Cost: 100k/1M * $2.50 + 50k/1M * $10.00 = $0.25 + $0.50 = $0.75
	b.Record("gpt-4o", 100_000, 50_000)

	daily, weekly := b.Spent()
	if daily < 0.74 || daily > 0.76 {
		t.Errorf("expected daily ~$0.75, got $%.4f", daily)
	}
	if weekly < 0.74 || weekly > 0.76 {
		t.Errorf("expected weekly ~$0.75, got $%.4f", weekly)
	}
}

func TestBudget_MidStreamCheck(t *testing.T) {
	b := NewBudget(0.50, 0) // $0.50/day, no weekly limit

	// Already spent $0.40
	b.Record("gpt-4o", 160_000, 0) // 160k * $2.50/1M = $0.40

	// Can we afford 20k more output tokens?
	// 20k * $10/1M = $0.20 → total would be $0.60 > $0.50
	if b.MidStreamCheck("gpt-4o", 20_000) {
		t.Error("should NOT pass mid-stream check — would exceed daily budget")
	}

	// Can we afford 5k more output tokens?
	// 5k * $10/1M = $0.05 → total would be $0.45 < $0.50
	if !b.MidStreamCheck("gpt-4o", 5_000) {
		t.Error("should pass mid-stream check — within budget")
	}
}

func TestBudget_NoLimits(t *testing.T) {
	b := NewBudget(0, 0) // no limits

	if !b.CanAfford("gpt-4o", 10_000_000) {
		t.Error("should always afford with no limits")
	}
	if !b.MidStreamCheck("gpt-4o", 10_000_000) {
		t.Error("should always pass mid-stream with no limits")
	}
}

func TestBudget_UnknownModel(t *testing.T) {
	b := NewBudget(1.00, 10.00)

	// Unknown model = $0 cost (subscription/free)
	if !b.CanAfford("unknown-model", 1_000_000) {
		t.Error("unknown model should be free")
	}

	b.Record("unknown-model", 1_000_000, 1_000_000)
	daily, _ := b.Spent()
	if daily != 0 {
		t.Errorf("expected $0 for unknown model, got $%.4f", daily)
	}
}

func TestBudget_GLMFree(t *testing.T) {
	b := NewBudget(1.00, 10.00)

	// GLM is subscription — always free
	if !b.CanAfford("glm-5-turbo", 10_000_000) {
		t.Error("GLM should always be affordable (subscription)")
	}

	b.Record("glm-5-turbo", 1_000_000, 1_000_000)
	daily, _ := b.Spent()
	if daily != 0 {
		t.Errorf("expected $0 for GLM, got $%.4f", daily)
	}
}

func TestBudget_SetModelCost(t *testing.T) {
	b := NewBudget(1.00, 10.00)

	b.SetModelCost("custom-model", ModelCost{Per1MInput: 5.00, Per1MOutput: 20.00})

	// 100k tokens * $5/1M = $0.50
	if !b.CanAfford("custom-model", 100_000) {
		t.Error("should afford 100k tokens of custom model")
	}

	// 300k tokens * $5/1M = $1.50 — over daily
	if b.CanAfford("custom-model", 300_000) {
		t.Error("should NOT afford 300k tokens of custom model")
	}
}

func TestBudget_String(t *testing.T) {
	b := NewBudget(5.00, 50.00)
	b.Record("gpt-4o", 100_000, 10_000)

	s := b.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}
