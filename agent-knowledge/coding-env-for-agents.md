# Learning Guide: Coding Environments for AI Agents

**Generated**: 2026-03-19
**Sources**: 42 resources analyzed
**Depth**: deep

---

## Prerequisites

- Familiarity with git fundamentals (branches, commits, diffs)
- Basic understanding of Docker containers and Linux namespaces
- Conceptual knowledge of LLM tool use / function calling
- Comfort reading Go, Python, or TypeScript examples

---

## TL;DR

- **Isolate every agent run in its own git worktree** (or Docker container) — agents writing to the same working tree cause silent overwrites and merge conflicts.
- **Sandbox code execution with layers**: Docker resource limits + seccomp profiles + nsjail/firejail for inner untrusted processes.
- **Route to the right model for the job**: fast/cheap models for read-only exploration (Haiku, Flash), capable models for writing and debugging (Sonnet, Opus).
- **Set an explicit tool-round ceiling** (e.g. 100 for coding, 10 for chat) and emit structured logs per round so failures are reproducible.
- **Clean up deterministically**: worktrees and containers that had no commits/writes should auto-delete; ones with work should prompt.

---

## Core Concepts

### 1. Git Worktree Isolation

Git worktrees allow multiple checked-out branches from a single repository to co-exist on disk simultaneously. Each worktree has its own `HEAD`, index, and working files while sharing the object database and remote tracking branches.

**Why this matters for agents:** without isolation, two concurrent agents working in the same working tree will silently overwrite each other's in-progress edits. Worktrees give each agent a private filesystem view with zero data duplication — all history objects are shared.

**Anatomy of a worktree:**
```
main-repo/
└── .git/
    └── worktrees/
        ├── agent-feature-a/    # private: HEAD, index, config.worktree
        └── agent-bugfix-b/     # private: HEAD, index
                                # shared: refs/heads/*, objects/

/tmp/agent-feature-a/           # agent A's working directory
/tmp/agent-bugfix-b/            # agent B's working directory
```

**Creating a worktree for an agent:**
```bash
# New branch from main, placed in .claude/worktrees/<name>
git worktree add -b worktree-feature-auth .claude/worktrees/feature-auth

# Detached HEAD for ephemeral experiments (no branch created)
git worktree add --detach /tmp/agent-scratch-$(uuidgen)

# List all live worktrees
git worktree list

# Remove when done (only works if no modifications)
git worktree remove .claude/worktrees/feature-auth

# Force-remove with uncommitted changes
git worktree remove --force .claude/worktrees/feature-auth
```

**Add to `.gitignore`** to prevent worktree content from showing in `git status`:
```
.claude/worktrees/
```

**Cleanup policy:** automatic if the agent made no commits; prompt the user if there are staged or committed changes.

#### Plandex's Protected Worktree Model

Plandex takes a different approach: it maintains a **pending diff sandbox** that accumulates AI-proposed edits without touching the user's working tree at all. Changes stay in a review buffer until the developer runs `apply`. The internal version control tracks every prompt/response/pending-diff tuple with `plandex log` / `plandex rewind`. This gives users a two-stage safety net: the AI sandbox, then the git working tree.

#### Claude Code's `--worktree` Flag

Claude Code (v2+) exposes this directly via the CLI:
```bash
# Agent gets its own isolated worktree
claude --worktree feature-auth

# Subagents can be given worktree isolation via frontmatter
# isolation: worktree  →  auto-cleanup if no changes
```

Worktrees live at `<repo>/.claude/worktrees/<name>`, branch off the default remote branch as `worktree-<name>`, and are cleaned up on exit based on whether the agent made changes.

#### Uzi Pattern

Uzi (and similar orchestrators) spawn N Claude Code processes, each in its own git worktree. The orchestrator manages the worktree lifecycle, merges completed branches back to main, and garbage-collects stale worktrees. This gives each concurrent agent full isolation at the cost of N × repository working-tree disk space.

---

### 2. Sandboxed Code Execution

Raw AI-generated code execution is a security boundary. Several layers are used in practice, typically nested.

#### Layer 1: Docker Containers (Process + Filesystem Isolation)

Docker provides the primary isolation shell used by SWE-agent, OpenHands, AutoGen, and most production coding agents.

**SWE-agent pattern:**
```bash
docker run -i --rm \
  --name sweagent-$(uuidgen) \
  sweagent/swe-agent:latest \
  /bin/bash -l
```
Each task gets an ephemeral container (`--rm`). The container is removed on exit. Environment variables (`CURRENT_FILE`, `CURRENT_LINE`, etc.) maintain agent state across tool calls within a single container session.

**OpenHands runtime model:**
- Backend spawns a Docker container with a custom image containing an `ActionExecutor`
- `ActionExecutor` manages: bash shell, browser, Jupyter server (via plugins), and additional plugins
- Backend communicates via RESTful API calls into the container
- Bidirectional: backend sends actions → container executes → observations return → agent decides next action
- **Image tagging strategy**: source tag (hash of code, avoids rebuild) → lock tag (dependency hash) → versioned tag (fallback)
- Supports bind mounts, Docker named volumes, and optional copy-on-write overlay (`SANDBOX_VOLUME_OVERLAYS`)

**Resource constraints for agent sandboxing:**
```bash
docker run \
  --cpus=2.0 \
  --memory=2g \
  --memory-swap=2g \          # no swap beyond RAM
  --network none \            # or a restricted bridge
  --cap-drop=ALL \
  --cap-add=NET_BIND_SERVICE \
  --security-opt no-new-privileges \
  --security-opt seccomp=/etc/docker/agent-seccomp.json \
  --mount type=bind,source=$(pwd),target=/workspace \
  --workdir /workspace \
  my-agent-image:latest
```

#### Layer 2: Seccomp Profiles (Syscall Filtering)

The Docker default seccomp profile blocks ~44 of 300+ syscalls. For coding agents, key restrictions include:

- Namespace operations (`clone`, `unshare`) — prevents container escapes
- `ptrace`, `process_vm_readv/writev` — prevents host inspection
- Kernel module loading — prevents kernel-level persistence
- Time-of-day manipulation — prevents timing attacks

Best practice: use the default Docker seccomp profile. Only loosen it with documented justification.

#### Layer 3: nsjail (Lightweight Process Jailing)

nsjail sits inside a Docker container and jails individual code execution commands. It uses all 8 Linux namespace types (UTS, MOUNT, PID, IPC, NET, USER, CGROUPS, TIME) plus:
- Seccomp-BPF filtering (via Kafel policy language)
- Resource limits: `--time_limit`, `--rlimit_as`, `--rlimit_cpu`
- Filesystem: chroot + pivot_root + read-only mounts

Example nsjail invocation for untrusted code:
```bash
nsjail \
  --mode o \
  --time_limit 30 \
  --rlimit_as 512 \
  --rlimit_cpu 10 \
  --rlimit_nofile 64 \
  --user nobody \
  --group nogroup \
  --chroot /jail \
  -- /usr/bin/python3 /jail/user_code.py
```

Compared to Docker: nsjail is significantly lighter (no image management, no daemon), ideal for per-command isolation. Docker is better for full runtime environments.

#### Layer 4: Firejail (Desktop Process Sandboxing)

Firejail is appropriate for sandboxing individual tool calls made from within an agent (e.g., a browser fetch, a build tool). It uses Linux user namespaces and integrates transparently with existing commands:
```bash
firejail --noprofile --noroot --net=none python3 -c "$(AGENT_CODE)"
```

#### E2B: Cloud-Native Sandboxes

E2B provides cloud-hosted Linux VMs as sandboxes (JavaScript/Python SDK):
```typescript
const sandbox = await Sandbox.create();
await sandbox.runCode('x = 1');
const result = await sandbox.runCode('x += 1; x');
// sandbox is isolated in cloud, full Linux VM
// state persists across runCode() calls within one sandbox
```

E2B's lifecycle: create → run code (state persists) → pause (snapshot for later resume) → resume → destroy. Supports filesystem access, process management, Git operations, and MCP server integration. Self-hostable on GCP or AWS.

#### OpenAI Codex CLI Sandboxing Tiers

Codex CLI (Rust-based) uses a tiered approval model:

| Mode | Sandbox | Approval Required |
|------|---------|-------------------|
| Suggest | Read-only | All writes and commands |
| Auto Edit | Read + patches | All shell commands |
| Full Auto | macOS Seatbelt / Docker | None (network disabled) |

macOS Full Auto uses `sandbox-exec` (Apple Seatbelt) with a read-only jail, writable only in `$PWD`, `$TMPDIR`, `~/.codex`. Linux Full Auto uses Docker with iptables blocking all outbound traffic except the LLM API endpoint.

---

### 3. Model Routing

Different tasks have very different cost/capability requirements. Production coding agents route tasks to different models rather than using one model for everything.

#### The Three Tiers

| Tier | Task Types | Typical Models | Token Cost |
|------|-----------|---------------|------------|
| Fast / Cheap | Codebase search, file listing, syntax checks, quick Q&A | Claude Haiku, Gemini Flash, GPT-4o mini | Low |
| Balanced | Implementation, refactoring, writing tests, code review | Claude Sonnet, GPT-4o | Medium |
| Capable | Complex debugging, architecture decisions, multi-file reasoning, security analysis | Claude Opus, o3, Gemini Ultra | High |

#### Claude Code Subagent Model Routing

Claude Code implements this via subagents with explicit model fields:
```yaml
---
name: explorer
description: Fast read-only codebase exploration
tools: Read, Grep, Glob
model: haiku          # fast, cheap, read-only
---

---
name: implementer
description: Write code, fix bugs, implement features
tools: Read, Edit, Write, Bash
model: sonnet          # balanced capability
---

---
name: architect
description: Complex architectural analysis, security review
tools: Read, Grep, Glob
model: opus            # deep reasoning, no writes
---
```

**Built-in subagent routing in Claude Code:**
- `Explore` subagent: Haiku, read-only tools — for all search/analysis operations
- `Plan` subagent: inherits model, read-only — for planning before implementation
- `General-purpose` subagent: inherits model, all tools — for complex multi-step work

#### Aider's Model Routing

Aider routes based on model capability:
- Weaker models: full-file replacement (wholefile edit format) — simpler but uses more tokens
- GPT-4o: editblock format — targeted diff-style edits
- GPT-4 Turbo: universal diff format — most precise

Context-sensitive: repo map is disabled by default for weaker models (overwhelming), can be forced on.

#### OpenAI Swarm / Agents SDK Handoffs

OpenAI Swarm uses function-return-based routing: returning an `Agent` object from a tool function transfers execution to that agent:
```python
def route_to_specialist(topic: str) -> Agent:
    if "security" in topic:
        return security_agent
    elif "database" in topic:
        return db_agent
    return general_coding_agent
```

OpenAI Agents SDK uses `handoff()` for explicit agent transfer with input metadata and optional history filtering. The LLM itself decides which specialist to hand off to based on registered handoff descriptions.

#### Cairn / Custom Go Routing

In Go, model routing is typically implemented as a strategy pattern:
```go
type ModelRouter struct {
    cheap    LLMClient  // e.g. glm-5-turbo
    balanced LLMClient  // e.g. glm-5
    capable  LLMClient  // e.g. claude-opus
}

func (r *ModelRouter) Route(task TaskType) LLMClient {
    switch task {
    case TaskExplore, TaskSearch:
        return r.cheap
    case TaskImplement, TaskTest:
        return r.balanced
    case TaskArchitect, TaskDebug:
        return r.capable
    }
    return r.balanced
}
```

---

### 4. Tool Call Round Limits

Every agentic loop needs a ceiling on the number of tool calls per session, both to prevent runaway cost and to surface stuck-agent failures.

#### Why Limits Matter

Without limits:
- A buggy tool can cause infinite retries (e.g., a broken test that never passes)
- Context grows unboundedly, eventually hitting the model's context window
- Cost can spike to hundreds of dollars on a single malformed task

#### Observed Limits in Production Systems

| System | Talk/Chat | Work/Coding | Notes |
|--------|-----------|-------------|-------|
| Claude Code (Pub) | 10 | 100 | Per mode, configurable |
| SWE-agent | — | ~100 | Configurable per YAML config |
| AutoGen | — | 10 (`max_tool_iterations`) | Per AssistantAgent |
| OpenAI Codex CLI | — | no hard limit | Human-in-loop per action |
| Claude Code subagents | — | `maxTurns` field | Per-subagent config |

#### Tuning Tool Round Limits

Factors that push the limit higher:
- Tasks with many files (full-repo refactors need more read rounds)
- Iterative test-fix cycles (each failing test costs 2-4 rounds)
- Complex debugging with hypothesis testing

Factors that push it lower:
- Cost sensitivity
- Latency requirements
- Trust level (untrusted/new tasks get tighter limits)

**Practical starting points:**
```
Exploration / Q&A: 5-15 rounds
Simple single-file edit: 10-20 rounds
Feature implementation: 40-80 rounds
Full-repo refactor: 80-150 rounds
```

#### Structured Limit Handling

When the limit is hit, the agent should:
1. Emit a structured `LIMIT_REACHED` log event with current context
2. Checkpoint any progress made (committed files, partial work)
3. Return to the user with a summary of what was done and what remains
4. NOT silently fail or loop back from the start

```go
type AgentSession struct {
    MaxRounds    int
    CurrentRound int
    CheckpointFn func(ctx *AgentContext) error
}

func (s *AgentSession) IncrementRound() error {
    s.CurrentRound++
    if s.CurrentRound >= s.MaxRounds {
        if err := s.CheckpointFn(s.ctx); err != nil {
            return fmt.Errorf("checkpoint failed at limit: %w", err)
        }
        return ErrRoundLimitReached
    }
    return nil
}
```

#### Tool Blocklisting

SWE-agent blocks interactive tools that cause the agent to hang: `vim`, `python` (interactive REPL), `git` (some interactive operations). The principle: only allow non-interactive, non-blocking commands. Interactive tools that wait for input will deadlock the agent loop.

---

### 5. Log Management for Coding Agents

#### Structured Logging (the 12-Factor Way)

Agents should write all log output to stdout/stderr as structured JSON, never manage log files themselves. The deployment environment handles routing, rotation, and storage.

**Per-round log entry (Go slog example):**
```go
slog.InfoContext(ctx, "tool_call",
    "session_id", sessionID,
    "round", roundNum,
    "tool", toolName,
    "input_summary", truncate(toolInput, 200),
    "duration_ms", durationMs,
    "result_ok", err == nil,
)
```

**Per-session summary entry:**
```go
slog.InfoContext(ctx, "session_complete",
    "session_id", sessionID,
    "total_rounds", roundNum,
    "files_modified", len(modifiedFiles),
    "tests_passed", testsPassed,
    "commit_sha", commitSHA,
    "model", modelUsed,
    "total_tokens", totalTokens,
    "duration_s", sessionDuration.Seconds(),
)
```

#### Log Schema for Agent Actions

A minimal structured log entry for agent actions:
```json
{
  "ts": "2026-03-19T12:00:00.123Z",
  "level": "INFO",
  "event": "tool_call",
  "session": "abc123",
  "agent": "implementer",
  "round": 7,
  "tool": "edit_file",
  "file": "src/auth/session.go",
  "duration_ms": 145,
  "ok": true
}
```

#### Log Rotation with logrotate

For agents writing to files (sidecar pattern), logrotate config:
```
/var/log/agent/*.log {
    daily
    rotate 14
    size 100M
    compress
    delaycompress
    missingok
    notifempty
    sharedscripts
    postrotate
        systemctl reload pub-agent 2>/dev/null || true
    endscript
}
```

Key directives:
- `size 100M` — rotate before `daily` if size exceeded (agent runs can be bursty)
- `delaycompress` — keep yesterday's log uncompressed for active tail
- `rotate 14` — 2 weeks of history for debugging
- `postrotate` — signal agent to reopen file descriptors

#### Session Journal Pattern

The session journal (used in Pub/Cairn) writes a structured episodic memory entry after each coding session:
```json
{
  "session_id": "abc123",
  "type": "coding_session",
  "summary": "Implemented OAuth2 handshake in auth module",
  "decisions": ["used PKCE flow", "stored tokens in HttpOnly cookies"],
  "errors": ["initial approach with session storage failed — concurrent access"],
  "learnings": ["session storage not safe under concurrent agents"],
  "files_changed": ["src/auth/oauth.go", "src/auth/session.go"],
  "tests_added": 3,
  "commit": "a1b2c3d"
}
```

These entries power RAG retrieval for future sessions and feed reflection engines that detect patterns across sessions.

---

### 6. Coding Session Lifecycle

The canonical lifecycle for a coding agent session:

```
┌─────────────────────────────────────────────────────────┐
│                  CODING SESSION LIFECYCLE                │
│                                                         │
│  1. INIT          Create worktree / container           │
│                   Load CLAUDE.md / AGENTS.md / context  │
│                   Verify environment (deps, tools)      │
│                                                         │
│  2. PLAN          Read-only exploration (cheap model)   │
│  (optional)       Build understanding of codebase       │
│                   Generate action plan                  │
│                                                         │
│  3. EXECUTE       Tool call loop (N rounds max)         │
│                   Read → Reason → Write → Test → Repeat │
│                   Checkpoint progress after each commit │
│                                                         │
│  4. VERIFY        Run tests, type checks, linting       │
│                   Confirm all assertions pass           │
│                   Review diff for unintended changes    │
│                                                         │
│  5. COMMIT        git add -p / git commit               │
│                   Generate descriptive commit message   │
│                   (optional: open PR)                   │
│                                                         │
│  6. CLEANUP       Write session journal entry           │
│                   Remove worktree if no changes         │
│                   Archive container                     │
│                   Report summary to user                │
└─────────────────────────────────────────────────────────┘
```

#### Phase 1: Initialization

```bash
# Create isolated worktree
git worktree add -b agent/feature-$(date +%s) .claude/worktrees/current

# Verify environment in the worktree
cd .claude/worktrees/current
npm install --silent   # or go mod download, pip install, etc.
npm run typecheck      # fast fail before spending tokens
```

Key invariant: **the agent should fail fast before expensive LLM calls if the environment is broken.**

#### Phase 3: The Tool Call Loop

```go
func (a *Agent) Run(ctx context.Context, task string) (*SessionResult, error) {
    messages := []Message{{Role: "user", Content: task}}

    for round := 0; round < a.maxRounds; round++ {
        resp, err := a.llm.Complete(ctx, messages, a.tools)
        if err != nil {
            return nil, fmt.Errorf("round %d LLM error: %w", round, err)
        }

        // Log round
        a.logger.InfoContext(ctx, "agent_round",
            "round", round,
            "stop_reason", resp.StopReason,
            "tool_calls", len(resp.ToolCalls),
        )

        // Check if done
        if resp.StopReason == "end_turn" || len(resp.ToolCalls) == 0 {
            break
        }

        // Execute tool calls
        toolResults := a.executeToolCalls(ctx, resp.ToolCalls)
        messages = append(messages, resp.AsMessage(), toolResults.AsMessage())
    }

    return a.buildResult(messages), nil
}
```

#### Phase 4: Verification

Never skip verification. The pattern from SWE-agent:
```bash
# Run linting inline during edit (immediate feedback)
edit file.py <<< 'code'  # SWE-agent runs flake8 after every edit

# Full verification at end
make test
npm run typecheck
go vet ./...
```

#### Phase 5: Commit Strategy

```bash
# Stage only files the agent modified (not everything)
git add -p   # or: git add $(agent_modified_files)

# Structured commit message
git commit -m "feat(auth): implement OAuth2 PKCE flow

- Add /oauth/callback route with state validation
- Store tokens in HttpOnly cookies (no localStorage)
- Add CSRF protection via state parameter

Generated by agent session abc123"
```

#### Phase 6: Cleanup

```bash
# Check if anything was committed
if [ $(git log origin/main..HEAD --oneline | wc -l) -gt 0 ]; then
    echo "Session produced $(git diff --stat origin/main HEAD | tail -1)"
    # Prompt user: keep worktree? open PR?
else
    # Auto-cleanup: nothing committed
    cd /
    git worktree remove --force .claude/worktrees/current
    git branch -D agent/feature-1234567890
fi
```

---

### 7. Go Implementations of Coding Agent Environments

#### Anthropic Go SDK Tool Runner

The `toolrunner` package in `anthropic-sdk-go` provides a generic, type-safe tool execution framework:

```go
// Define a typed tool with JSON schema auto-generation
type EditFileInput struct {
    Path    string `json:"path" jsonschema:"required,description=File path to edit"`
    OldText string `json:"old_text" jsonschema:"required,description=Text to replace"`
    NewText string `json:"new_text" jsonschema:"required,description=Replacement text"`
}

editTool, _ := toolrunner.NewBetaToolFromJSONSchema[EditFileInput](
    "edit_file",
    "Edit a file by replacing exact text",
    func(ctx context.Context, input EditFileInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
        // Execute the edit
        err := editFile(input.Path, input.OldText, input.NewText)
        if err != nil {
            return errorResult(err), nil  // Return error to model, not panic
        }
        return successResult("Edited " + input.Path), nil
    },
)
```

The agentic loop pattern in Go:
```go
func agentLoop(ctx context.Context, client *anthropic.Client, tools []anthropic.BetaTool) {
    var messages []anthropic.BetaMessageParam

    for {
        resp, _ := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
            Model:     anthropic.ModelClaudeOpus4_6,
            MaxTokens: 8192,
            Tools:     tools,
            Messages:  messages,
        })

        messages = append(messages, resp.ToParam())

        if resp.StopReason == "end_turn" {
            break
        }

        // Process tool calls
        var toolResults []anthropic.BetaToolResultBlockParam
        for _, block := range resp.Content {
            if block.Type == "tool_use" {
                result := dispatchTool(ctx, block.Name, block.Input)
                toolResults = append(toolResults, result)
            }
        }

        messages = append(messages, anthropic.BetaUserMessageParam(toolResults...))
    }
}
```

#### Plandex: Go-Based Plan Management

Plandex is written entirely in Go. Its server manages plans where each plan is a named conversation+pending-diff state. Key architectural choices:
- Plan state stored in a database (not the git working tree)
- `plandex apply` atomically applies the accumulated diff to the project
- `plandex rewind` restores previous states using Plandex's internal versioning
- Branch-based safety: `plandex branch` before `plandex rewind` to preserve history

#### Go slog for Agent Action Logging

```go
// Initialize structured logger for agent
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level:     slog.LevelInfo,
    AddSource: false,  // too noisy for production
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        // Redact sensitive values
        if a.Key == "api_key" || a.Key == "token" {
            return slog.String(a.Key, "REDACTED")
        }
        return a
    },
}))

// Per-action logging with consistent schema
logger.InfoContext(ctx, "tool_call",
    slog.Group("tool",
        "name", toolName,
        "round", round,
        "duration_ms", elapsed.Milliseconds(),
    ),
    slog.Group("session",
        "id", sessionID,
        "model", modelName,
        "worktree", worktreePath,
    ),
    "ok", err == nil,
)
```

Use `slog.LevelVar` for runtime level adjustment without restart:
```go
var logLevel = new(slog.LevelVar)  // Default: Info
// In debug mode: logLevel.Set(slog.LevelDebug)
```

---

## Code Examples

### Basic: Create Isolated Worktree for Agent Session

```bash
#!/bin/bash
# create-agent-worktree.sh

REPO_ROOT=$(git rev-parse --show-toplevel)
SESSION_ID=$(uuidgen | head -c 8)
BRANCH="agent/session-${SESSION_ID}"
WORKTREE_PATH="${REPO_ROOT}/.claude/worktrees/${SESSION_ID}"

# Create worktree + branch
git worktree add -b "${BRANCH}" "${WORKTREE_PATH}"

echo "Created worktree: ${WORKTREE_PATH}"
echo "Branch: ${BRANCH}"
echo "Session: ${SESSION_ID}"

# Initialize environment in worktree
cd "${WORKTREE_PATH}"
npm install --silent 2>/dev/null || go mod download 2>/dev/null || true

# Return path for agent to use
echo "WORKTREE=${WORKTREE_PATH}"
echo "SESSION=${SESSION_ID}"
```

### Intermediate: Agent Tool Loop with Round Limiting (Go)

```go
package agent

import (
    "context"
    "fmt"
    "log/slog"
)

type Config struct {
    MaxRounds   int
    Model       string
    WorktreePath string
    SessionID   string
}

type Runner struct {
    cfg    Config
    llm    LLMClient
    tools  []Tool
    logger *slog.Logger
}

func (r *Runner) Execute(ctx context.Context, task string) error {
    r.logger.InfoContext(ctx, "session_start",
        "session_id", r.cfg.SessionID,
        "task", task[:min(100, len(task))],
        "max_rounds", r.cfg.MaxRounds,
        "worktree", r.cfg.WorktreePath,
    )

    messages := []Message{{Role: "user", Content: task}}

    for round := 0; round < r.cfg.MaxRounds; round++ {
        start := time.Now()
        resp, err := r.llm.Complete(ctx, messages, r.tools)

        r.logger.InfoContext(ctx, "round_complete",
            "round", round,
            "duration_ms", time.Since(start).Milliseconds(),
            "stop_reason", resp.StopReason,
            "tool_calls", len(resp.ToolCalls),
        )

        if err != nil {
            return fmt.Errorf("round %d: %w", round, err)
        }

        if resp.StopReason == "end_turn" {
            r.logger.InfoContext(ctx, "session_complete",
                "rounds_used", round+1,
                "session_id", r.cfg.SessionID,
            )
            return nil
        }

        results, err := r.executeTools(ctx, resp.ToolCalls)
        if err != nil {
            return fmt.Errorf("tool execution round %d: %w", round, err)
        }

        messages = append(messages, resp.AsMessage(), results.AsMessage())
    }

    // Checkpoint on limit hit
    r.logger.WarnContext(ctx, "round_limit_reached",
        "session_id", r.cfg.SessionID,
        "max_rounds", r.cfg.MaxRounds,
    )
    return ErrRoundLimitReached
}
```

### Advanced: Docker Sandbox for Code Execution

```go
package sandbox

import (
    "context"
    "fmt"
    "os/exec"
    "time"
)

type DockerSandbox struct {
    Image       string
    WorkDir     string
    MaxCPU      string  // e.g. "0.5"
    MaxMemory   string  // e.g. "512m"
    NetworkMode string  // "none" for full isolation
    Timeout     time.Duration
}

func (s *DockerSandbox) Run(ctx context.Context, cmd string) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, s.Timeout)
    defer cancel()

    args := []string{
        "run", "--rm",
        "--cpus", s.MaxCPU,
        "--memory", s.MaxMemory,
        "--memory-swap", s.MaxMemory,  // disable swap
        "--network", s.NetworkMode,
        "--cap-drop=ALL",
        "--security-opt", "no-new-privileges",
        "--workdir", "/workspace",
        "--mount", fmt.Sprintf("type=bind,source=%s,target=/workspace", s.WorkDir),
        s.Image,
        "sh", "-c", cmd,
    }

    out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
    if err != nil {
        return string(out), fmt.Errorf("sandbox execution failed: %w", err)
    }
    return string(out), nil
}
```

### Advanced: Claude Code Subagent with Worktree Isolation

```yaml
# .claude/agents/feature-implementer.md
---
name: feature-implementer
description: >
  Implement new features in an isolated worktree. Use when the user asks to
  build, add, or implement something new. Works in a separate branch to
  avoid affecting the main working tree.
model: sonnet
isolation: worktree
tools: Read, Edit, Write, Bash, Glob, Grep
maxTurns: 80
hooks:
  PostToolUse:
    - matcher: "Write|Edit"
      hooks:
        - type: command
          command: "./scripts/run-typecheck.sh"
  Stop:
    - type: command
      command: "./scripts/session-journal.sh"
---

You are a feature implementation agent. You work in an isolated git worktree.

When invoked:
1. Understand the feature requirements fully before writing any code
2. Check existing patterns with Grep/Read before introducing new ones
3. Implement incrementally: small commits rather than one large change
4. Run tests after each logical unit of work
5. Ensure all existing tests still pass before finishing

Commit convention: `feat(<scope>): <description>`
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|----------------|--------------|
| Two agents editing the same working tree | No worktree isolation | Always create a worktree per agent session; never share working directories |
| Interactive tools deadlock the agent loop | `vim`, REPL `python`, interactive `git` block waiting for stdin | Blocklist interactive tools; only allow non-interactive commands |
| Unbounded tool calls | No round limit set | Set `maxTurns` / `max_rounds` per mode; add checkpoint-on-limit logic |
| Container left running after crash | No cleanup on exception | Use `--rm` on Docker run; defer `git worktree remove` in code |
| All tasks go to the expensive model | No model routing | Route exploration to Haiku/Flash, implementation to Sonnet, deep analysis to Opus |
| Agent commits secrets | Working on a branch with secrets in env | Use `git-secrets` or `gitleaks` in PostToolUse hook; blocklist `.env` from Write tool |
| Test failures swallowed silently | Error handling returns success to model | Return actual error text as tool result; model needs to see failures |
| Log files grow unbounded | Agent writes verbose debug logs in prod | Structured JSON to stdout only; logrotate with `size 100M` limit |
| Stale worktrees consuming disk | No GC of abandoned worktrees | `git worktree prune --expire 1.day.ago` in a daily cron; auto-cleanup on session end |
| Race condition on shared task queue | Multiple agents claiming same task | Use file locking or atomic DB operations for task claiming (Claude Code agent teams use this) |

---

## Best Practices

1. **One worktree per agent session** — never share working trees between concurrent agents. Use `git worktree add -b agent/<session-id> .claude/worktrees/<session-id>`. (Source: git-scm.com, Claude Code docs)

2. **Layer your sandboxing** — Docker for runtime isolation + seccomp for syscall filtering + nsjail for inner untrusted execution. Each layer catches what the previous misses. (Source: nsjail docs, Docker security docs, OpenHands architecture)

3. **Route to the right model** — fast models for search/read, balanced for write/implement, capable for debugging/architecture. This can cut costs 3-5x without capability loss. (Source: Claude Code subagent docs, Aider FAQ, SWE-bench leaderboard)

4. **Emit structured JSON logs per tool call** — include session_id, round, tool name, duration_ms, and ok flag. This is the minimum needed to debug stuck agents. (Source: Go slog docs, 12-factor app)

5. **Always verify before committing** — run the project's own test/typecheck commands inside the worktree before any commit. Fast-fail early is cheaper than cleaning up a bad merge. (Source: SWE-agent patterns, Claude Code common workflows)

6. **Blocklist interactive tools** — any tool that reads from stdin will hang the agent loop indefinitely. Build an explicit allowlist of safe commands or use `--interactive=false` flags. (Source: SWE-agent docs)

7. **Checkpoint progress on round limit** — when `maxTurns` is reached, commit what was done, write a journal entry, and return a summary to the user rather than silently stopping. (Source: Pub architecture patterns)

8. **Clean up deterministically** — auto-remove worktrees with no commits; prompt for ones with work. Add `.claude/worktrees/` to `.gitignore`. Run `git worktree prune` daily. (Source: Claude Code worktree docs)

9. **Use per-worktree `config.worktree`** — enable `extensions.worktreeConfig true` for agent-specific git settings (e.g., different email for agent commits, sparse checkout). (Source: git-scm.com)

10. **Write session journals** — structured episodic memory (summary, decisions, errors, learnings) after each session enables reflection, pattern detection, and better future sessions. (Source: Pub/Cairn session journaling, LLM agent survey)

---

## Comparison: Major Approaches

| Tool | Isolation | Sandboxing | Model Routing | Tool Limits | Log Pattern |
|------|-----------|------------|---------------|-------------|-------------|
| SWE-agent | Docker container per task | Docker + blocklist | Single model | YAML config | Docker logs |
| OpenHands | Docker + ActionExecutor | Docker + RESTful API | Single model | Per session | Structured events |
| Aider | None (working tree) | None | Per-task model selection | None | Terminal output |
| Plandex | Pending diff sandbox | None | None | None | Plan history |
| Claude Code | `--worktree` flag | None (CI: runner) | Per-subagent model | `maxTurns` per agent | Structured |
| Codex CLI | macOS seatbelt / Docker | Iptables + seccomp | Single model | Approval-based | REPL history |
| E2B | Cloud VM sandbox | Full VM isolation | Provider-agnostic | Timeout-based | Cloud logs |
| AutoGen | Docker executor | Docker | Per-agent model | `max_tool_iterations` | Python logging |

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [git-worktree documentation](https://git-scm.com/docs/git-worktree) | Official Docs | Complete worktree command reference; per-worktree config |
| [Claude Code sub-agents](https://code.claude.com/docs/en/sub-agents) | Official Docs | Model routing, worktree isolation, maxTurns, tool scoping |
| [Claude Code agent teams](https://code.claude.com/docs/en/agent-teams) | Official Docs | Parallel agent coordination, task claiming, shared mailbox |
| [Claude Code common workflows](https://code.claude.com/docs/en/common-workflows) | Official Docs | Worktree workflows, session naming, parallel sessions |
| [OpenHands runtime architecture](https://docs.openhands.dev/modules/usage/architecture/runtime) | Official Docs | Docker sandbox, ActionExecutor, image tagging, plugin system |
| [SWE-agent coding challenges](https://swe-agent.com/latest/usage/coding_challenges/) | Official Docs | Docker-per-task pattern, tool blocklist, environment state |
| [nsjail](https://github.com/google/nsjail) | GitHub | Lightweight process jailing with full namespace isolation |
| [E2B sandbox](https://e2b.dev/docs) | Official Docs | Cloud-native sandboxes for AI agent code execution |
| [OpenAI Codex CLI README](https://github.com/openai/codex/blob/main/codex-cli/README.md) | GitHub | Tiered sandboxing (seatbelt/Docker), approval flows |
| [Anthropic Go SDK toolrunner](https://github.com/anthropics/anthropic-sdk-go/tree/main/toolrunner) | GitHub | Generic type-safe tool runner in Go |
| [Go slog package](https://pkg.go.dev/log/slog) | Official Docs | Structured logging for agent actions in Go |
| [Lilian Weng: LLM-powered Agents](https://lilianweng.github.io/posts/2023-06-23-agent/) | Blog | Foundational survey of agent architectures, tool use, memory |
| [OpenAI Agents SDK handoffs](https://openai.github.io/openai-agents-python/handoffs/) | Official Docs | LLM-driven model routing via agent handoffs |
| [OpenAI Swarm](https://github.com/openai/swarm) | GitHub | Lightweight multi-agent handoff patterns |
| [Docker resource constraints](https://docs.docker.com/engine/reference/run/#resource-constraints) | Official Docs | CPU, memory, network limits for sandboxed execution |
| [Docker seccomp profiles](https://docs.docker.com/engine/security/seccomp/) | Official Docs | Syscall filtering for container hardening |
| [12-factor app: Logs](https://12factor.net/logs) | Guide | Stream-based logging methodology for agent deployments |
| [SWE-bench leaderboard](https://www.swebench.com/) | Benchmark | Agent performance by architecture, cost, API call count |
| [Plandex version control](https://docs.plandex.ai/core-concepts/version-control) | Official Docs | Protected diff sandbox, branch safety, rewind mechanics |

---

## Relevancy Analysis for Cairn / Pub

### Cairn (Go backend)

**Immediately applicable:**
- Use `slog.NewJSONHandler` for all agent action logging — structured, zero-cost to parse
- Add `MaxRounds` to the agent loop config with per-mode values (explore: 10, implement: 80)
- Implement worktree creation/cleanup in coding session start/end hooks
- Add nsjail wrapping around any AI-generated shell code execution
- Use the Anthropic Go SDK `toolrunner` pattern for type-safe tool definitions

**Architecture alignment:**
- Cairn's three channels (Telegram, web, API) can route to different model tiers using the strategy pattern
- Voice mode → cheap fast model; coding sessions → balanced/capable model
- Session journal entries (already in spec) map directly to the episodic memory pattern

### Pub (TypeScript/Node.js backend)

**Already implemented:**
- Session journaling via `SessionJournaler` (PR #679)
- Tool round limits per mode (10 for talk/work, 100 for coding)
- git-based isolation for coding sessions

**Gaps to address:**
- Worktree isolation for concurrent coding agents (currently shares working tree)
- Structured per-round logging (currently logs at session level)
- Model routing for subagents (all use GLM-5 today)

---

*This guide was synthesized from 42 sources. See `resources/coding-env-for-agents-sources.json` for full source list.*
