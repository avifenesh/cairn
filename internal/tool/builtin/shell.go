//go:build unix

package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

const (
	defaultShellTimeout = 30  // seconds
	maxShellTimeout     = 300 // seconds
)

// shellParams are the parameters for cairn.shell.
type shellParams struct {
	Command   string `json:"command" desc:"Shell command to execute"`
	WorkDir   string `json:"workDir" desc:"Working directory (relative to work directory, default: work directory root)"`
	Timeout   int    `json:"timeout" desc:"Timeout in seconds (default: 30, max: 300)"`
	MaxOutput *int   `json:"maxOutput" desc:"Max output bytes (default: 102400, 0=unlimited)"`
}

var shell = tool.Define("cairn.shell",
	"Execute a shell command and return stdout+stderr. Dangerous commands (rm -rf /, shutdown, etc.) are blocked. Environment is filtered to prevent secret leakage.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p shellParams) (*tool.ToolResult, error) {
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = defaultShellTimeout
		}
		if timeout > maxShellTimeout {
			timeout = maxShellTimeout
		}

		// Check deny patterns before anything else.
		if reason := checkDenyPatterns(p.Command); reason != "" {
			return &tool.ToolResult{
				Error: fmt.Sprintf("command denied: %s", reason),
			}, nil
		}

		// Determine working directory.
		if ctx.WorkDir == "" {
			return &tool.ToolResult{Error: "work directory not set - cannot execute shell commands without a working directory"}, nil
		}
		workDir := ctx.WorkDir
		if p.WorkDir != "" {
			resolved, err := safePath(ctx.WorkDir, p.WorkDir)
			if err != nil {
				return nil, err
			}
			workDir = resolved
		}

		// Detect shell and build command.
		si := detectShell()
		command := p.Command
		if si.supportsPipefail {
			command = "set -eo pipefail\n" + command
		}

		// Create context with timeout.
		execCtx := ctx.Cancel
		if execCtx == nil {
			execCtx = context.Background()
		}
		execCtx, cancel := context.WithTimeout(execCtx, time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(execCtx, si.path, "-c", command)
		cmd.Dir = workDir
		cmd.Env = filteredEnv()

		// Run in its own process group so we can kill all child processes on timeout.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		// On context cancellation, kill the entire process group.
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			return nil
		}
		cmd.WaitDelay = 2 * time.Second

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		// Build combined output (for the agent).
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n--- stderr ---\n"
			}
			output += stderr.String()
		}

		// Truncate if needed.
		maxOut := DefaultMaxOutputBytes
		if p.MaxOutput != nil {
			maxOut = *p.MaxOutput
		}
		output, wasTruncated := truncateOutput(output, maxOut)

		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}

		meta := map[string]any{
			"exitCode": exitCode,
			"workDir":  workDir,
			"shell":    si.path,
		}
		if stderr.Len() > 0 {
			meta["stderr"] = stderr.String()
		}
		if wasTruncated {
			meta["truncated"] = true
		}
		if op := detectPipeOrRedirect(p.Command); op != "" {
			meta["pipeOrRedirect"] = op
		}

		result := &tool.ToolResult{
			Output:   output,
			Metadata: meta,
		}

		if err != nil {
			if execCtx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Sprintf("command timed out after %ds", timeout)
			} else {
				result.Error = fmt.Sprintf("command failed (exit %d): %v", exitCode, err)
			}
		}

		return result, nil
	},
)
