---
name: pr-workflow
description: "Use when creating PRs, monitoring CI, addressing review comments, or shipping code. Keywords: create PR, push, CI, review, merge, ship"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.gitRun,cairn.readFile,cairn.editFile"
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

### Distinguish comment types

GitHub has two distinct comment systems on PRs:

| Type | API Endpoint | Has threading | Created by `gh pr comment` |
|------|-------------|---------------|---------------------------|
| **Review comments** (inline diff) | `pulls/{pull_number}/comments` | Yes — `in_reply_to_id` field | No |
| **Issue comments** (timeline) | `issues/{pull_number}/comments` | No | Yes |

To list review comments:
```bash
gh api repos/{owner}/{repo}/pulls/{pull_number}/comments --paginate --jq '.[] | {id, path, line, body, user: .user.login, in_reply_to_id}'
```

To list issue comments:
```bash
gh api repos/{owner}/{repo}/issues/{pull_number}/comments --paginate --jq '.[] | {id, body, user: .user.login}'
```

### Reply to review comments (inline diff comments)

Use the **dedicated replies endpoint** — it's the cleanest and most reliable:
```bash
gh api repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies \
  --method POST \
  --field body="Addressed in COMMIT_HASH" \
  --jq '.html_url'
```

This endpoint only requires `body`. It automatically threads the reply under the parent review comment. The `comment_id` must be a top-level review comment (replies to replies are not supported by the API).

**Alternative** — create a review comment with `in_reply_to`:
```bash
gh api repos/{owner}/{repo}/pulls/{pull_number}/comments \
  --method POST \
  --field body="Addressed in COMMIT_HASH" \
  --field in_reply_to={comment_id} \
  --jq '.html_url'
```

When `in_reply_to` is set, all other parameters (commit_id, path, line, etc.) are ignored — only `body` matters.

### Reply to issue comments (timeline comments)

Issue comments have no threading API. Reply by posting a new issue comment and referencing the original:
```bash
gh pr comment <number> --body "Re: review feedback — addressed in COMMIT_HASH"
```

Or via API:
```bash
gh api repos/{owner}/{repo}/issues/{pull_number}/comments \
  --method POST \
  --field body="Re: review feedback — addressed in COMMIT_HASH"
```

### Reply workflow
1. Fetch review comments: `gh api repos/{owner}/{repo}/pulls/{pull_number}/comments --paginate`
2. Fetch issue comments: `gh api repos/{owner}/{repo}/issues/{pull_number}/comments --paginate`
3. Check review state — fetch all reviews and collapse to latest **non-approval** state per reviewer. A `COMMENTED` or `DISMISSED` review does NOT supersede an earlier `CHANGES_REQUESTED`. Only `APPROVED` clears a blocking review: `gh api repos/{owner}/{repo}/pulls/{pull_number}/reviews --paginate --slurp --jq '[.[] | .[]] | group_by(.user.login)[] | sort_by(.submitted_at) | reverse | map(select(.state == "APPROVED" or .state == "CHANGES_REQUESTED")) | first // {state: "COMMENTED", user: .[0].user.login, id: .[0].id}'` — this returns each reviewer's latest meaningful state (APPROVED or CHANGES_REQUESTED); earlier CHANGES_REQUESTED persists until explicitly approved
4. Dedupe review comments by top-level thread — `in_reply_to_id` is null on top-level comments; replies share the same parent. Reply only to the top-level parent, not to each reply.
5. Fix ALL comments — high, medium, AND low priority
6. Commit fixes as new commit (never amend)
7. Push and re-check CI
8. Reply to each top-level comment using the correct endpoint based on comment type

### Reply to review-summary feedback

When a reviewer submits `CHANGES_REQUESTED` with a summary body but no inline comments, there's no thread to reply to. Post an issue comment acknowledging the review and referencing the commit that addresses the feedback:
```bash
gh pr comment <number> --body "Addressed review feedback from @{reviewer_login} in COMMIT_HASH"
```

### Fallback for position mapping failures

When creating a new inline review comment (using path/line/position) fails because the diff `position` no longer maps to the current code (after new commits), fall back to a regular PR comment referencing the affected file/line or review discussion:
```bash
gh pr comment <number> --body "Addressed review on {path} L{line} in COMMIT_HASH"
```

## Merge
1. Verify all checks pass
2. Verify no new comments
3. Merge: `cairn.shell` with `gh pr merge <number> --squash --admin`

## Rules
- Never use `--no-verify` or `--force`
- Never skip review comments regardless of severity
- Create new commits for fixes (don't amend)
- Always run tests before pushing
