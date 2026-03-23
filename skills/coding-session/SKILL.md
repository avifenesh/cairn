---
name: coding-session
description: "Autonomous coding workflow: branch, code, test, draft PR with [cairn] prefix. Use when starting a coding task from the idle loop or continuing an incomplete session. Keywords: coding, implement, fix, bug, PR, CI, test, review, worktree, autonomous"
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

### 7. CI & Review Monitor Loop (BLOCKING - do not skip)

This loop runs until BOTH conditions are met: CI green AND 0 unresolved review threads.
Do NOT exit the coding session until this loop completes.

**Max iterations: 10. Initial reviewer wait: 180 seconds (3 minutes).**

```
iteration = 0
while iteration < 10:
    iteration += 1

    # Step A: Wait for CI to complete (poll every 60s, max 15 polls)
    poll = 0
    while poll < 15:
        cairn.shell: "sleep 60 && gh pr checks <number>"
        poll += 1
        # If all checks completed (no "pending"): break
        # If any check failed:
        #   1. Get failure logs: cairn.shell: "gh run view <run-id> --log-failed"
        #   2. Fix the issue
        #   3. Commit + push
        #   4. Continue loop (CI will re-run)

    # Step B: Wait 180s for auto-reviewers after every push (not just first).
    # Auto-reviewers re-analyze after EVERY push. They need time.
    cairn.shell: "sleep 180"

    # Step C: Check for unresolved review threads
    cairn.shell: "gh api graphql -f query='query($owner:String!,$repo:String!,$pr:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$pr){reviewThreads(first:100){nodes{isResolved}}}}}' -f owner=OWNER -f repo=REPO -F pr=<number> --jq '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)] | length'"

    # Step D: If 0 unresolved → quiet period check
    if unresolved == 0:
        # MANDATORY: wait 3 minutes for late comments from auto-reviewers.
        cairn.shell: "sleep 180"

        # Re-check: run the SAME GraphQL query from Step C again
        unresolved_after_wait = <result of same query>
        if unresolved_after_wait == 0:
            break  # truly clean — exit loop
        # else: new comments arrived — fall through to Step E

    # Step E: Address ALL comments
    # E1. Fetch unresolved thread IDs + comment bodies:
    cairn.shell: "gh api graphql -f query='query($owner:String!,$repo:String!,$pr:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$pr){reviewThreads(first:100){nodes{id,isResolved,comments(last:1){nodes{body,path,line}}}}}}}' -f owner=OWNER -f repo=REPO -F pr=<number> --jq '.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false) | {threadId: .id, comment: .comments.nodes[0]}'"
    # E2. Fix every comment — high, medium, AND low priority. No skipping.
    # E3. Resolve each thread after fixing:
    cairn.shell: "gh api graphql -f query='mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{isResolved}}}' -f id=<threadId from E1>"
    # E4. Commit + push:
    cairn.gitRun: ["commit", "-m", "fix: address review feedback (iteration N)"]
    cairn.shell: "git push"

    # Step F: Brief pause before next iteration (CI re-triggers on push)
    cairn.shell: "sleep 30"
```

EXPECT 5+ ITERATIONS. This is normal. Auto-reviewers post new comments after every push.
The loop only exits when: CI green AND 0 unresolved AND 3-minute quiet period passed with no new comments.

When the loop exits clean:
```
cairn.shell: "gh pr ready <number>"  # Remove draft status
```

### 8. Notify
When PR is ready and all reviews addressed:
```
cairn.notify: message="PR ready: [cairn] <title> — CI green, all reviews addressed, quiet period clean. Review at <url>", priority=medium
```

### 9. Completion
The session is COMPLETE only after:
- [x] CI checks all pass
- [x] 0 unresolved review threads
- [x] 3-minute quiet period passed with no new comments
- [x] PR marked as ready (not draft)
- [x] Notification sent

Output: `[SESSION_COMPLETE] PR #<number> — CI green, 0 unresolved, quiet period clean, ready for merge.`

### 10. Running out of rounds?
If you're approaching the round limit (check current round vs 400 max) and the CI/review loop is not done:
- Commit what you have
- Push to the branch
- Create a continuation cron job:
```
cairn.createCron: name="pr-watch-{owner}-{repo}-<PR_NUMBER>", schedule="0 * * * *", instruction="Continue PR #<PR_NUMBER> review loop. Check CI + unresolved comments. Fix issues and push. Delete this cron when PR is merged/closed."
```
- In your final response, state what remains for the continuation to pick up.

## Rules
- One logical change per PR — don't mix unrelated fixes
- Run tests before every commit
- Format code: `gofmt -w .` for Go, `pnpm check` for frontend
- Address ALL review comments (when continuing after review)
- Never amend published commits — new commits only
- Never skip git hooks
