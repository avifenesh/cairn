---
name: github
description: "Use whenever GitHub operations are needed — by user request or by agent workflow. PRs, issues, repos, CI, merging, branching, releases. Keywords: PR, issue, repo, CI, merge, branch, release, gh, push, pull"
inclusion: always
allowed-tools: "cairn.shell,cairn.gitRun,cairn.readFile,cairn.webFetch"
disable-model-invocation: true
---

# GitHub Operations

Use the `gh` CLI (authenticated, available at `/usr/bin/gh`) for all GitHub operations.

## Common Operations

### PRs
- List: `cairn.shell` with `gh pr list --limit 10`
- View: `cairn.shell` with `gh pr view <number>`
- Create: `cairn.shell` with `gh pr create --title "..." --body "..."`
- Merge: `cairn.shell` with `gh pr merge <number> --squash`
- Checks: `cairn.shell` with `gh pr checks <number>`

### Issues
- List: `cairn.shell` with `gh issue list --limit 10`
- Create: `cairn.shell` with `gh issue create --title "..." --body "..."`
- View: `cairn.shell` with `gh issue view <number>`
- Close: `cairn.shell` with `gh issue close <number>`

### CI
- View runs: `cairn.shell` with `gh run list --limit 5`
- View run: `cairn.shell` with `gh run view <run-id>`
- Failed logs: `cairn.shell` with `gh run view <run-id> --log-failed`

### Repos
- View: `cairn.shell` with `gh repo view`
- Clone: `cairn.shell` with `gh repo clone <owner>/<repo>`

## Rate Limits
- Space `gh api` calls at least 2 seconds apart
- Never poll CI in tight loops — use `sleep 30` between checks
- GitHub API: 5000 req/hr REST, 5000 req/hr GraphQL

## Git Operations
- Use `cairn.gitRun` for local git commands (status, diff, log, branch, commit)
- Use `cairn.shell` with `gh` for remote operations (push, PR, issues)
