package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/tool"
)

// newTestSoul creates a Soul with the given content for testing.
func newTestSoul(t *testing.T, content string) *memory.Soul {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test SOUL.md: %v", err)
	}
	s := memory.NewSoul(path)
	if err := s.Load(); err != nil {
		t.Fatalf("failed to load test SOUL.md: %v", err)
	}
	return s
}

func TestBuildSystemPrompt_SubagentKeepsIdentity(t *testing.T) {
	tests := []struct {
		name         string
		subagentHint string
		soulContent  string
		wantIdentity bool
		wantHint     bool
		wantSoul     bool
	}{
		{
			name:         "main agent has identity but no hint",
			subagentHint: "",
			soulContent:  "# Soul\nRepo owner is avifenesh",
			wantIdentity: true,
			wantHint:     false,
			wantSoul:     true,
		},
		{
			name:         "subagent has both identity and hint",
			subagentHint: "You are a research agent. Gather information.",
			soulContent:  "# Soul\nRepo owner is avifenesh",
			wantIdentity: true,
			wantHint:     true,
			wantSoul:     true,
		},
		{
			name:         "subagent hint does not replace identity",
			subagentHint: "You are a coding agent working in an isolated git worktree.",
			soulContent:  "# Soul\nCanonical repo owner: avifenesh",
			wantIdentity: true,
			wantHint:     true,
			wantSoul:     true,
		},
		{
			name:         "subagent without soul still gets identity",
			subagentHint: "You are a research agent.",
			soulContent:  "",
			wantIdentity: true,
			wantHint:     true,
			wantSoul:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var soul *memory.Soul
			if tt.soulContent != "" {
				soul = newTestSoul(t, tt.soulContent)
			}

			invCtx := &InvocationContext{
				Mode:    tool.ModeTalk,
				Soul:    soul,
				Config:  &AgentConfig{SubagentSystemHint: tt.subagentHint},
				Memory:  nil,
				Session: &Session{},
			}

			modeConfig := &ModeConfig{
				Mode:      tool.ModeTalk,
				MaxRounds: 40,
				Prompt:    "You are in talk mode.",
			}

			prompt := BuildSystemPrompt(invCtx, modeConfig, nil, nil)

			if tt.wantIdentity {
				if !containsStr(prompt, "You are Cairn, a personal agent operating system.") {
					t.Errorf("expected identity line in prompt, got:\n%s", prompt)
				}
			}

			if tt.wantHint && tt.subagentHint != "" {
				if !containsStr(prompt, tt.subagentHint) {
					t.Errorf("expected subagent hint in prompt, got:\n%s", prompt)
				}
			}

			if tt.wantSoul {
				if !containsStr(prompt, "avifenesh") {
					t.Errorf("expected soul content (avifenesh) in prompt — subagent hint should not suppress soul injection, got:\n%s", prompt)
				}
			}
		})
	}
}

func TestBuildSystemPrompt_SubagentHintOrder(t *testing.T) {
	// Identity should appear before the subagent hint.
	soul := newTestSoul(t, "# Soul\nIdentity anchor")

	invCtx := &InvocationContext{
		Mode:    tool.ModeTalk,
		Soul:    soul,
		Config:  &AgentConfig{SubagentSystemHint: "You are a research agent."},
		Memory:  nil,
		Session: &Session{},
	}

	modeConfig := &ModeConfig{
		Mode:      tool.ModeTalk,
		MaxRounds: 40,
		Prompt:    "You are in talk mode.",
	}

	prompt := BuildSystemPrompt(invCtx, modeConfig, nil, nil)

	identityIdx := indexStr(prompt, "You are Cairn, a personal agent operating system.")
	hintIdx := indexStr(prompt, "You are a research agent.")

	if identityIdx == -1 {
		t.Fatal("identity not found in prompt")
	}
	if hintIdx == -1 {
		t.Fatal("hint not found in prompt")
	}
	if identityIdx > hintIdx {
		t.Errorf("identity (idx=%d) should appear before subagent hint (idx=%d)", identityIdx, hintIdx)
	}
}

func containsStr(s, substr string) bool {
	return indexStr(s, substr) >= 0
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
