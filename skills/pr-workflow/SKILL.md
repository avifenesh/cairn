---
name: pr-workflow
description: "Use when creating PRs, monitoring CI, addressing review comments, or shipping code. Keywords: create PR, push, CI, review, merge, ship"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.gitRun,cairn.readFile,cairn.editFile"
disable-model-invocation: true
---

# PR Workflow

## Create PR
1. Create branch: `cairn.gitRun` with `checkout -b feat/description`
2. Make changes with `cairn.editFile`
3. Stage: `cairn.gitRun` with `add <files>` (specific files, not -A)
4. Commit: `cairn.gitRun` with `commit -m "type: description"`
5. Push: `cairn.shell` with `git push -u origin <branch>`
6. Create PR: `cairn.shell` with `gh pr create --title "..." --body "..."`

## Monitor CI
1. Check: `cairn.shell` with `gh pr checks <number>`
2. If pending, wait 2 minutes: `cairn.shell` with `sleep 120 && gh pr checks <number>`
3. If failed, get logs: `cairn.shell` with `gh run view <run-id> --log-failed`
4. Fix issues, commit, push again

## Address Review Comments
1. Read comments: `cairn.shell` with `gh api repos/{owner}/{repo}/pulls/{number}/comments`
2. Fix ALL comments — high, medium, AND low priority
3. Commit fixes as new commit (never amend)
4. Push and re-check CI

## Merge
1. Verify all checks pass
2. Verify no new comments
3. Merge: `cairn.shell` with `gh pr merge <number> --squash --admin`

## Rules
- Never use `--no-verify` or `--force`
- Never skip review comments regardless of severity
- Create new commits for fixes (don't amend)
- Always run tests before pushing
