package agent

import "strings"

// EnvContext captures environment details for injection into agent prompts.
// Provides compact system context so agents know their runtime environment.
type EnvContext struct {
	OS          string   // e.g. "linux"
	Shell       string   // e.g. "/bin/bash"
	User        string   // e.g. "ubuntu"
	Home        string   // e.g. "/home/ubuntu"
	Go          string   // e.g. "go1.25"
	Node        string   // e.g. "v20.11.0"
	GitUser     string   // e.g. "avifenesh"
	DataDir     string   // e.g. "/home/ubuntu/.cairn/data"
	CodingRepos []string // e.g. ["/home/ubuntu/cairn"]
}

// Format returns a compact markdown representation of the environment.
// Returns empty string if the receiver is nil.
func (e *EnvContext) Format() string {
	if e == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Environment\n")

	if e.OS != "" {
		b.WriteString("- OS: " + e.OS + "\n")
	}
	if e.Shell != "" {
		b.WriteString("- Shell: " + e.Shell + "\n")
	}
	if e.User != "" {
		b.WriteString("- User: " + e.User + "\n")
	}
	if e.Home != "" {
		b.WriteString("- Home: " + e.Home + "\n")
	}
	if e.Go != "" {
		b.WriteString("- Go: " + e.Go + "\n")
	}
	if e.Node != "" {
		b.WriteString("- Node: " + e.Node + "\n")
	}
	if e.GitUser != "" {
		b.WriteString("- Git user: " + e.GitUser + "\n")
	}
	if e.DataDir != "" {
		b.WriteString("- Data dir: " + e.DataDir + "\n")
	}
	if len(e.CodingRepos) > 0 {
		b.WriteString("- Coding repos: " + strings.Join(e.CodingRepos, ", ") + "\n")
	}

	return b.String()
}
