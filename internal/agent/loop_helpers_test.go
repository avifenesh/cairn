package agent

import (
	"encoding/json"
	"testing"
)

func TestIsRepoAllowed_EmptyList(t *testing.T) {
	l := &Loop{config: LoopConfig{CodingAllowedRepos: nil}}
	if l.isRepoAllowed("/any/path") {
		t.Error("expected false for empty allowlist")
	}
}

func TestIsRepoAllowed_ExactMatch(t *testing.T) {
	l := &Loop{config: LoopConfig{CodingAllowedRepos: []string{"/home/ubuntu/cairn", "/home/ubuntu/pub"}}}
	if !l.isRepoAllowed("/home/ubuntu/cairn") {
		t.Error("expected allowed for exact match")
	}
	if !l.isRepoAllowed("/home/ubuntu/pub") {
		t.Error("expected allowed for second repo")
	}
}

func TestIsRepoAllowed_NotInList(t *testing.T) {
	l := &Loop{config: LoopConfig{CodingAllowedRepos: []string{"/home/ubuntu/cairn"}}}
	if l.isRepoAllowed("/home/ubuntu/evil-repo") {
		t.Error("expected denied for repo not in list")
	}
}

func TestIsRepoAllowed_NormalizesPath(t *testing.T) {
	l := &Loop{config: LoopConfig{CodingAllowedRepos: []string{"/home/ubuntu/cairn"}}}
	// Trailing slash should still match after normalization.
	if !l.isRepoAllowed("/home/ubuntu/cairn/") {
		t.Error("expected allowed after path normalization")
	}
}

func TestExtractRepoFromInput_ValidJSON(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"repo": "/home/ubuntu/cairn", "instruction": "fix bug"})
	repo := extractRepoFromInput(input)
	if repo != "/home/ubuntu/cairn" {
		t.Errorf("expected /home/ubuntu/cairn, got %q", repo)
	}
}

func TestExtractRepoFromInput_NoRepo(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"instruction": "fix bug"})
	repo := extractRepoFromInput(input)
	if repo != "" {
		t.Errorf("expected empty, got %q", repo)
	}
}

func TestExtractRepoFromInput_EmptyInput(t *testing.T) {
	repo := extractRepoFromInput(nil)
	if repo != "" {
		t.Errorf("expected empty for nil input, got %q", repo)
	}
}

func TestExtractRepoFromInput_InvalidJSON(t *testing.T) {
	repo := extractRepoFromInput(json.RawMessage("not json"))
	if repo != "" {
		t.Errorf("expected empty for invalid JSON, got %q", repo)
	}
}
