---
name: github
description: "GitHub operations via gh CLI — shell-first, permissions-aware"
inclusion: always
allowed-tools: "cairn.shell,cairn.gitRun,cairn.readFile,cairn.webFetch"
---

# Skill: github

# GitHub Operations

**Always use `cairn.shell` with `gh` CLI for GitHub operations.** The `gh` CLI is authenticated and available at `/usr/bin/gh`. It covers everything GitHub exposes via API — REST and GraphQL. You do NOT need a dedicated GitHub API tool. `gh` IS the API tool.

## Decision: shell vs webFetch

- **Never** use `cairn.webFetch` for GitHub API endpoints — it's unauthenticated, rate-limited to 60 req/hr, and returns raw JSON.
- **Always** use `cairn.shell` with `gh` commands. Authenticated (5000 req/hr), formatted output.

## Permissions

### Auto-execute (no approval needed)
- All read operations: `list`, `view`, `search`, `diff`, `checks`, `api` with GET
- `gh pr create` — drafting PRs with `[cairn]` prefix
- `gh pr edit` — on my own `[cairn]` PRs only (title, body)
- `gh pr ready` — marking my own PRs as ready for review
- `gh pr comment` — on my own `[cairn]` PRs (e.g. responding to review feedback)
- `gh pr revert` — reverting my own `[cairn]` merges
- `gh run view --log` — reading CI logs

### Require explicit approval from Avi
- `gh pr merge` — **never merge without Avi's explicit approval**, regardless of CI status
- `gh release create` — releases are public-facing
- `gh issue create` / `gh issue edit` / `gh issue close` / `gh issue comment` — affects external project tracking
- `gh workflow run` — dispatching workflows can have side effects
- `gh run rerun` — re-triggering CI jobs
- Any `gh api` call with `--method POST|PUT|PATCH|DELETE` that modifies external state
- Any operation on repos/PRs that don't have the `[cairn]` prefix

### Never
- Push to main/master — blocked by `cairn.gitRun`
- Delete repos, branches on repos you don't own, or any irreversible destructive action
- Approve PRs you didn't review
- Modify issues or PRs owned by Avi or others without being asked

## `gh api` — Raw REST & GraphQL access

Any GitHub REST endpoint or GraphQL query.
```bash
# REST
gh api <endpoint> [--method METHOD] [--paginate] [--jq '...'] [--field key=value] [--input file.json] [--header 'Accept: ...']
# GraphQL
gh api graphql -f query='...' -F owner=... -F name=...
```
Flags: `--method`, `--input`, `--jq`, `--paginate`, `--slurp`, `--field`/`-F` (typed), `--raw-field`/`-f` (string), `--header`/`-H`, `--preview`

## Built-in commands

### PRs
```
gh pr list [-R owner/repo] [--limit N] [--state open|closed|merged|all] [--author LOGIN] [--head BRANCH] [--base BRANCH] [--label LBL] [--search QRY]
gh pr view <number|url|branch> [-R owner/repo]
gh pr create --title "..." --body "..." [-R owner/repo]
gh pr checks <number> [-R owner/repo]
gh pr merge <number> --squash|--merge|--rebase [--delete-branch] [-R owner/repo]
gh pr review <number> --approve|--request-changes|--comment -b "..." [-R owner/repo]
gh pr diff <number> [-R owner/repo]
gh pr close <number> [-R owner/repo]
gh pr comment <number> -b "..." [-R owner/repo]
gh pr edit <number> --title "..." --body "..." [-R owner/repo]
gh pr ready <number> [-R owner/repo]
gh pr revert <number> [-R owner/repo]
```

### Issues
```
gh issue list [-R owner/repo] [--limit N] [--state open|closed|all] [--author LOGIN] [--assignee LOGIN] [--label LBL] [--milestone NAME] [--search QRY]
gh issue view <number|url> [-R owner/repo]
gh issue create --title "..." --body "..." [-R owner/repo]
gh issue close <number> [-R owner/repo]
gh issue comment <number> -b "..." [-R owner/repo]
gh issue edit <number> --title "..." --body "..." [-R owner/repo]
```

### Repos
```
gh repo view [owner/repo]
gh repo list <owner> [--limit N] [--visibility public|private|internal] [--source|--fork] [--language LANG] [--topic TOPIC] [--no-archived]
gh repo clone <owner/repo> [dir]
```

### CI / Actions
```
gh run list [-R owner/repo] [--limit N] [--workflow WORKFLOW]
gh run view <run-id> [-R owner/repo] [--log] [--log-failed] [--verbose]
gh run watch <run-id> [-R owner/repo]
gh run rerun <run-id> [-R owner/repo]
gh workflow list [-R owner/repo]
gh workflow run <workflow> [-R owner/repo] [-f key=value]
```

### Releases
```
gh release list [-R owner/repo]
gh release view <tag> [-R owner/repo]
gh release create <tag> [-R owner/repo] [--title "..."] [--notes "..."] [--draft]
```

### Search
```
gh search repos|--issues|--prs|--code|--commits "<query>"
gh search repos --owner=agent-sh --limit 10
gh search issues "is:open label:bug" --repo owner/repo
```

## Local git

- Use `cairn.gitRun` for local git commands (status, diff, log, branch, commit)
- Use `cairn.shell` with `gh` for all remote/interactive operations

## Tips

- All commands accept `-R owner/repo` to target a repo (or set `GH_REPO` env var)
- `--json field1,field2` outputs structured JSON on any list/view command
- `--jq '.[].name'` filters JSON output
- `--paginate` on `gh api` auto-follows next pages
- Space `gh api` calls at least 2s apart. Never poll CI in tight loops — use `sleep 30`.

