package skill

// Skill represents a parsed SKILL.md file — a prompt-based capability
// that teaches the LLM how to accomplish tasks.
type Skill struct {
	Name         string         // lowercase identifier, matches directory name
	Description  string         // trigger description ("Use when user asks to...")
	Inclusion    Inclusion      // always or on-demand
	AllowedTools []string       // scoped tool access (comma-separated in frontmatter)
	DisableModel bool           // side-effect skills require approval
	Content      string         // SKILL.md body (everything after frontmatter)
	Location     string         // file path on disk
	Metadata     map[string]any // extra frontmatter fields not in the core set
}

// Inclusion controls when a skill is injected into the system prompt.
type Inclusion string

const (
	Always   Inclusion = "always"    // always included in system prompt
	OnDemand Inclusion = "on-demand" // listed by description; LLM decides whether to use
)
