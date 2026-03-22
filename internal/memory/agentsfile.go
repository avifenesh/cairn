package memory

// AgentsFile wraps a MarkdownFile for the AGENTS.md operating manual.
// This file contains instructions, style guides, and constraints that
// all agents (main and sub) should follow.
type AgentsFile struct {
	*MarkdownFile
}

// NewAgentsFile creates an AgentsFile bound to the given file path.
func NewAgentsFile(filePath string) *AgentsFile {
	return &AgentsFile{MarkdownFile: NewMarkdownFile(filePath)}
}
