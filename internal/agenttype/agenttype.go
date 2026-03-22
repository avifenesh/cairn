// Package agenttype discovers and manages AGENT.md files that define
// sub-agent type configurations. Mirrors the skill package pattern.
package agenttype

import "github.com/avifenesh/cairn/internal/tool"

// AgentType represents a parsed AGENT.md file that defines a sub-agent
// type's capabilities, constraints, and system prompt.
type AgentType struct {
	Name         string         // lowercase identifier, matches directory name
	Description  string         // human-readable description
	Mode         tool.Mode      // talk, work, or coding
	AllowedTools []string       // allowlist: only these tools (nil = all tools in mode)
	DeniedTools  []string       // denylist: exclude these tools (applied after AllowedTools; nil = no denials)
	Skills       []string       // skills to pre-load into session (nil = none)
	MaxRounds    int            // max reasoning rounds
	Model        string         // LLM model override (empty = default)
	Worktree     bool           // requires worktree isolation
	Content      string         // AGENT.md body (everything after frontmatter)
	Location     string         // file path on disk
	Metadata     map[string]any // extra frontmatter fields
}
