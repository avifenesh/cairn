package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOptional_NoEnv(t *testing.T) {
	c := LoadOptional()
	if c == nil {
		t.Fatal("LoadOptional returned nil")
	}
	if c.Port == 0 {
		t.Error("expected non-zero default port")
	}
	if c.DatabasePath == "" {
		t.Error("expected default database path")
	}
}

func TestLoad_RequiresAPIKey(t *testing.T) {
	// Unset all API key vars to ensure Load fails.
	for _, k := range []string{"LLM_API_KEY", "GLM_API_KEY", "ZHIPU_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(k, "")
	}
	_, err := Load()
	if err == nil {
		t.Error("expected error when no API key set")
	}
}

func TestLoad_GLMProvider(t *testing.T) {
	t.Setenv("GLM_API_KEY", "test-key")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LLMProvider != "glm" {
		t.Errorf("provider = %q, want glm", c.LLMProvider)
	}
	if c.LLMModel != "glm-5-turbo" {
		t.Errorf("model = %q, want glm-5-turbo", c.LLMModel)
	}
}

func TestLoad_OpenAIProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("GLM_API_KEY", "")
	t.Setenv("ZHIPU_API_KEY", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LLMProvider != "openai" {
		t.Errorf("provider = %q, want openai", c.LLMProvider)
	}
	if c.LLMModel != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", c.LLMModel)
	}
}

func TestLoad_ZhipuNormalized(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test")
	t.Setenv("LLM_PROVIDER", "zhipu")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LLMProvider != "glm" {
		t.Errorf("provider = %q, want glm (zhipu normalized)", c.LLMProvider)
	}
}

func TestMaxRoundsForMode(t *testing.T) {
	c := &Config{TalkMaxRounds: 10, WorkMaxRounds: 20, CodingMaxRounds: 100}

	tests := []struct {
		mode string
		want int
	}{
		{"talk", 10},
		{"work", 20},
		{"coding", 100},
		{"unknown", 10}, // defaults to talk
	}
	for _, tt := range tests {
		if got := c.MaxRoundsForMode(tt.mode); got != tt.want {
			t.Errorf("MaxRoundsForMode(%q) = %d, want %d", tt.mode, got, tt.want)
		}
	}
}

func TestApplyPatch(t *testing.T) {
	c := &Config{
		CompactionTriggerTokens: 80000,
		BudgetDailyCap:          5.0,
		QuietHoursStart:         -1,
		NotifMinPriority:        "low",
	}

	trigger := 150000
	cap := 10.0
	qh := 22
	prio := "high"

	c.ApplyPatch(PatchableConfig{
		CompactionTriggerTokens: &trigger,
		BudgetDailyCap:          &cap,
		QuietHoursStart:         &qh,
		NotifMinPriority:        &prio,
	})

	if c.CompactionTriggerTokens != 150000 {
		t.Errorf("CompactionTriggerTokens = %d, want 150000", c.CompactionTriggerTokens)
	}
	if c.BudgetDailyCap != 10.0 {
		t.Errorf("BudgetDailyCap = %f, want 10.0", c.BudgetDailyCap)
	}
	if c.QuietHoursStart != 22 {
		t.Errorf("QuietHoursStart = %d, want 22", c.QuietHoursStart)
	}
	if c.NotifMinPriority != "high" {
		t.Errorf("NotifMinPriority = %q, want high", c.NotifMinPriority)
	}
}

func TestApplyPatch_InvalidPriority(t *testing.T) {
	c := &Config{NotifMinPriority: "low"}
	bad := "invalid"
	c.ApplyPatch(PatchableConfig{NotifMinPriority: &bad})
	if c.NotifMinPriority != "low" {
		t.Errorf("expected invalid priority to be rejected, got %q", c.NotifMinPriority)
	}
}

func TestGetPatchable_RoundTrip(t *testing.T) {
	c := &Config{
		CompactionTriggerTokens: 100000,
		BudgetDailyCap:          3.5,
		GHOwner:                 "testuser",
		NotifMinPriority:        "medium",
	}

	p := c.GetPatchable()
	if p.CompactionTriggerTokens == nil || *p.CompactionTriggerTokens != 100000 {
		t.Error("CompactionTriggerTokens not round-tripped")
	}
	if p.BudgetDailyCap == nil || *p.BudgetDailyCap != 3.5 {
		t.Error("BudgetDailyCap not round-tripped")
	}
	if p.GHOwner == nil || *p.GHOwner != "testuser" {
		t.Error("GHOwner not round-tripped")
	}
}

func TestSaveAndLoadOverrides(t *testing.T) {
	dir := t.TempDir()
	c := &Config{
		CompactionTriggerTokens: 200000,
		BudgetDailyCap:          7.5,
		NotifMinPriority:        "medium",
	}

	if err := c.SaveOverrides(dir); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("config.json not created: %v", err)
	}

	// Load into a fresh config and verify values applied.
	c2 := &Config{
		CompactionTriggerTokens: 80000,
		BudgetDailyCap:          1.0,
		NotifMinPriority:        "low",
	}
	c2.LoadOverrides(dir)

	if c2.CompactionTriggerTokens != 200000 {
		t.Errorf("after LoadOverrides: CompactionTriggerTokens = %d, want 200000", c2.CompactionTriggerTokens)
	}
	if c2.BudgetDailyCap != 7.5 {
		t.Errorf("after LoadOverrides: BudgetDailyCap = %f, want 7.5", c2.BudgetDailyCap)
	}
}

func TestLoadOverrides_NoFile(t *testing.T) {
	c := &Config{CompactionTriggerTokens: 80000}
	c.LoadOverrides(t.TempDir()) // should not panic or change anything
	if c.CompactionTriggerTokens != 80000 {
		t.Error("LoadOverrides changed config when no file exists")
	}
}

func TestSplitTrimmed(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a, b, c", 3},
		{"single", 1},
		{" , , ", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := splitTrimmed(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitTrimmed(%q) = %d items, want %d", tt.input, len(got), tt.want)
		}
	}
}
