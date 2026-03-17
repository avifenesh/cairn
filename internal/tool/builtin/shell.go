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

// shellParams are the parameters for pub.shell.
type shellParams struct {
	Command string `json:"command" desc:"Shell command to execute"`
	WorkDir string `json:"workDir" desc:"Working directory (relative to work directory, default: work directory root)"`
	Timeout int    `json:"timeout" desc:"Timeout in seconds (default: 30, max: 300)"`
}

var shell = tool.Define("pub.shell",
	"Execute a shell command and return stdout+stderr.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p shellParams) (*tool.ToolResult, error) {
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = defaultShellTimeout
		}
		if timeout > maxShellTimeout {
			timeout = maxShellTimeout
		}

		// Determine working directory.
		workDir := ctx.WorkDir
		if p.WorkDir != "" {
			resolved, err := safePath(ctx.WorkDir, p.WorkDir)
			if err != nil {
				return nil, err
			}
			workDir = resolved
		}

		// Create context with timeout.
		execCtx := ctx.Cancel
		if execCtx == nil {
			execCtx = context.Background()
		}
		execCtx, cancel := context.WithTimeout(execCtx, time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(execCtx, "sh", "-c", p.Command)
		cmd.Dir = workDir

		// Run in its own process group so we can kill all child processes on timeout.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		// On context cancellation, kill the entire process group.
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				// Kill the process group (negative PID).
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			return nil
		}
		cmd.WaitDelay = 2 * time.Second

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

		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}

		result := &tool.ToolResult{
			Output: output,
			Metadata: map[string]any{
				"exitCode": exitCode,
				"workDir":  workDir,
			},
		}

		if err != nil {
			if execCtx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Sprintf("command timed out after %ds", timeout)
			} else {
				result.Error = fmt.Sprintf("command failed: %v", err)
			}
		}

		return result, nil
	},
)
