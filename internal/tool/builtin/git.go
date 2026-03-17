package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

const gitTimeout = 60 // seconds

// protectedBranches are branches that cannot be pushed to directly.
var protectedBranches = []string{"main", "master"}

// gitRunParams are the parameters for pub.gitRun.
type gitRunParams struct {
	Args    []string `json:"args" desc:"Git command arguments (e.g., [\"status\", \"--short\"])"`
	WorkDir string   `json:"workDir" desc:"Working directory (relative to work directory, default: work directory root)"`
}

var gitRun = tool.Define("pub.gitRun",
	"Run a git command in the worktree. Rejects push to main/master unless explicitly allowed.",
	[]tool.Mode{tool.ModeCoding},
	func(ctx *tool.ToolContext, p gitRunParams) (*tool.ToolResult, error) {
		if len(p.Args) == 0 {
			return &tool.ToolResult{Error: "no git arguments provided"}, nil
		}

		// Reject push to protected branches.
		if isProtectedPush(p.Args) {
			return &tool.ToolResult{
				Error: fmt.Sprintf("push to protected branch (%s) is not allowed",
					strings.Join(protectedBranches, ", ")),
			}, nil
		}

		// Determine working directory.
		if ctx.WorkDir == "" {
			return &tool.ToolResult{Error: "work directory not set - cannot run git without a working directory"}, nil
		}
		workDir := ctx.WorkDir
		if p.WorkDir != "" {
			resolved, err := safePath(ctx.WorkDir, p.WorkDir)
			if err != nil {
				return nil, err
			}
			workDir = resolved
		}

		execCtx := ctx.Cancel
		if execCtx == nil {
			execCtx = context.Background()
		}
		execCtx, cancel := context.WithTimeout(execCtx, gitTimeout*time.Second)
		defer cancel()

		cmd := exec.CommandContext(execCtx, "git", p.Args...)
		cmd.Dir = workDir

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n--- stderr ---\n"
			}
			output += stderr.String()
		}

		result := &tool.ToolResult{
			Output: output,
			Metadata: map[string]any{
				"exitCode": cmd.ProcessState.ExitCode(),
				"workDir":  workDir,
				"command":  "git " + strings.Join(p.Args, " "),
			},
		}

		if err != nil {
			result.Error = fmt.Sprintf("git command failed: %v", err)
		}

		return result, nil
	},
)

// isProtectedPush returns true if the git args represent a push to a protected branch.
func isProtectedPush(args []string) bool {
	if len(args) < 1 || args[0] != "push" {
		return false
	}

	// Check every argument after "push" for protected branch names.
	// Also check refspecs like "main:main" or "HEAD:main".
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue // skip flags
		}
		for _, branch := range protectedBranches {
			if arg == branch {
				return true
			}
			// Check refspec patterns like "HEAD:main" or "main:main".
			if strings.HasSuffix(arg, ":"+branch) {
				return true
			}
		}
	}
	return false
}
