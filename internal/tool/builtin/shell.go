//go:build unix

package builtin

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		// Shell commands can cd anywhere, so workDir is just the initial cwd.
		// File tools (read/write/edit) use safePath for path containment.
		workDir := ctx.WorkDir
		if p.WorkDir != "" {
			if filepath.IsAbs(p.WorkDir) {
				workDir = filepath.Clean(p.WorkDir)
			} else if workDir != "" {
				workDir = filepath.Clean(filepath.Join(workDir, p.WorkDir))
			} else {
				workDir = filepath.Clean(p.WorkDir)
			}
		}
		// Fall back to $HOME when no workDir is set at all, so the agent
		// can work across repos. "." (process cwd) is kept as-is.
		if workDir == "" {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				workDir = home
			} else {
				workDir = "."
			}
		}

		// Enforce containment for worktree-isolated subagents.
		// File tools use safePath(); shell must match when Confined is set.
		if ctx.Confined && ctx.WorkDir != "" {
			resolved, err := safePath(ctx.WorkDir, workDir)
			if err != nil {
				return &tool.ToolResult{
					Error: fmt.Sprintf("shell containment: %s (confined to %s)", err, ctx.WorkDir),
				}, nil
			}
			workDir = resolved
		}

		// Validate workDir exists before attempting exec. A missing directory
		// causes exec to fail with a cryptic exit -1 (process didn't start).
		// Only fall back for "not found" errors; permission/IO errors are real problems.
		if workDir != "." {
			if info, statErr := os.Stat(workDir); statErr != nil {
				if !os.IsNotExist(statErr) {
					return &tool.ToolResult{
						Error: fmt.Sprintf("workDir %q: %s", workDir, statErr),
					}, nil
				}
				home, _ := os.UserHomeDir()
				if home == "" {
					home = "."
				}
				slog.Warn("shell workDir not found, falling back", "requested", workDir, "fallback", home)
				workDir = home
			} else if !info.IsDir() {
				return &tool.ToolResult{
					Error: fmt.Sprintf("workDir %q is not a directory", workDir),
				}, nil
			}
		}

		// Detect shell and build command.
		si := detectShell()
		command := p.Command
		if si.supportsPipefail {
			command = "set -o pipefail\n" + command
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
			// SIGPIPE (exit 141) is normal for pipe patterns like `cmd | head`.
			// Only treat as success when the command contains a pipe operator.
			if exitCode == 141 && stdout.Len() > 0 && detectPipeOrRedirect(p.Command) == "|" {
				meta["sigpipe"] = true
			} else if execCtx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Sprintf("command timed out after %ds", timeout)
			} else {
				// Include stderr (or err.Error() for exec failures) so the agent can diagnose.
				errMsg := fmt.Sprintf("exit %d", exitCode)
				if se := strings.TrimSpace(stderr.String()); se != "" {
					if len(se) > 500 {
						se = se[:500] + "..."
					}
					errMsg += ": " + se
				} else if exitCode < 0 {
					// Process didn't start — include the Go error for context.
					errMsg += ": " + err.Error()
				}
				result.Error = errMsg
			}
			if result.Error != "" {
				// Log the executable name only — strip inline env assignments
				// (KEY=value) and arguments to avoid leaking secrets.
				cmdLog := p.Command
				if fields := strings.Fields(cmdLog); len(fields) > 0 {
					// Skip leading KEY=value assignments.
					idx := 0
					for idx < len(fields) && strings.Contains(fields[idx], "=") {
						idx++
					}
					if idx < len(fields) {
						cmdLog = fields[idx]
					} else {
						cmdLog = "(env-only command)"
					}
				}
				slog.Warn("shell command failed", "exit", exitCode, "cmd", cmdLog, "workDir", workDir)
			}
		}

		return result, nil
	},
)
