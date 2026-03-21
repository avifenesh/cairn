---
name: coding-session
description: "Autonomous coding workflow: branch, code, test, draft PR with [cairn] prefix. Use when starting a coding task from the idle loop or continuing an incomplete session. Keywords: code, fix, PR, CI, review, test, implement"
inclusion: always
context: "tick"
allowed-tools: "cairn.shell,cairn.readFile,cairn.writeFile,cairn.editFile,cairn.deleteFile,cairn.listFiles,cairn.searchFiles,cairn.gitRun,cairn.notify,cairn.loadSkill,cairn.createCron,cairn.deleteCron"
---

# Autonomous Coding Session

Workflow for coding tasks submitted by the idle loop. You're in an isolated worktree with your own branch.

## Workflow

### 1. Understand the task
- Read the instruction carefully
- If continuing a previous session, review the "Previous Session" context
- Identify which files need changes
- Load relevant skills: `cairn.loadSkill` with `go-dev`, `pr-workflow`, etc.

### 2. Create a descriptive branch
```bash
# Your worktree already has a cairn/{taskID} branch
# Rename it to something meaningful:
cairn.gitRun: ["branch", "-m", "feat/cairn-<short-description>"]
# or: fix/cairn-<short-description>
```

### 3. Code iteratively
- `cairn.readFile` → understand existing code
- `cairn.editFile` → make targeted changes
- `cairn.shell` → run tests: `go test ./...`
- `cairn.shell` → check quality: `go vet ./...`, `gofmt -l .`
- Fix issues, repeat until green

### 4. Commit
```
cairn.gitRun: ["add", "file1.go", "file2.go"]  # specific files, never -A
cairn.gitRun: ["commit", "-m", "fix: descriptive message"]
```

### 5. Push to feature branch
```
cairn.shell: "git push -u origin HEAD"
```
NEVER push to main or master. `cairn.gitRun` blocks this.

### 6. Create DRAFT PR
```bash
cairn.shell: "gh pr create --draft --title '[cairn] Short description' --body '## Summary\n- What was changed\n- Why\n\n## Test plan\n- [ ] go test ./... passes\n- [ ] go vet clean'"
```

CRITICAL rules:
- Always `--draft` — never ready-for-review
- Always `[cairn]` prefix in title
- Never run `gh pr merge` — blocked by policy

### 7. Monitor CI
```bash
cairn.shell: "gh pr checks <number>"
```
- If CI fails: read logs with `gh run view <run-id> --log-failed`, fix, push
- If CI passes: proceed to notification

### 8. Notify
When draft PR is ready and CI green:
```
cairn.notify: message="Draft PR ready: [cairn] <title> — CI green. Review at <url>", priority=medium
```

### 9. Create PR review monitor
After creating the draft PR, set up a cron job to automatically address review comments:
```
cairn.createCron: name="pr-watch-{owner}-{repo}-<PR_NUMBER>", schedule="0 * * * *", instruction="PR #<PR_NUMBER> review monitor:
  1. Check if PR is still open: gh pr view <PR_NUMBER> --json state --jq '.state'
  2. If merged or closed → delete this cron job: cairn.deleteCron: name="pr-watch-{owner}-{repo}-<PR_NUMBER>" and stop.
  3. If open, check for unresolved review comments: gh api repos/{owner}/{repo}/pulls/<PR_NUMBER>/comments — filter for comments where the bot hasn't replied.
  4. If unresolved review comments found → address them in coding mode. Fix the issues, push, iterate until resolved.
  5. If no comments → do nothing."
```

This runs hourly (matches the 1h cooldown in `createCron`), self-terminates when the PR closes, and auto-fixes review comments.

### 10. Running out of rounds?
If you're approaching the round limit and work isn't done:
- Commit what you have
- Push to the branch
- In your final response, clearly state:
  - What was completed
  - What remains to be done
  - Which files still need changes
This enables the continuation mechanism to pick up where you left off.

## Rules
- One logical change per PR — don't mix unrelated fixes
- Run tests before every commit
- Format code: `gofmt -w .` for Go, `pnpm check` for frontend
- Address ALL review comments (when continuing after review)
- Never amend published commits — new commits only
- Never skip git hooks
