---
name: find-work
description: "Find what to work on next — scan issues, TODO.md, technical debt, and the codebase for improvements. Keywords: what should I work on, next task, find work, what needs attention, improve, tech debt, backlog, issues, priorities, what can be better"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.searchFiles,cairn.readFile,cairn.gitRun,cairn.listTasks,cairn.searchMemories"
---


# Find Work

Scan all sources for actionable work, then prioritize and present options.

Code is your body — improving the codebase expands your abilities. Think strategically: fix before build, quality over quantity.

## Step 1: Scan sources (run all in parallel)

**GitHub issues**
```
cairn.shell: gh issue list --repo avifenesh/pub --state open --json number,title,labels,assignees,createdAt --limit 30
```

**TODO.md**
```
cairn.shell: grep -n '\[ \]\|TODO\|FIXME\|HACK' ~/cairn-backend/TODO.md ~/cairn-backend/TECHNICAL_DEBT.md 2>/dev/null | head -40
```

**Recent CI failures**
```
cairn.shell: gh run list --repo avifenesh/pub --limit 5 --json name,conclusion,headBranch,updatedAt
```

**Stale PRs**
```
cairn.shell: gh pr list --repo avifenesh/pub --state open --json number,title,updatedAt,author --limit 10
```

**Codebase improvements** (scan for common issues)
```
cairn.shell: grep -rn 'console\.log\|TODO\|FIXME\|HACK\|XXX\|@deprecated' ~/cairn-backend/src/ --include='*.ts' | grep -v node_modules | grep -v '.test.' | head -30
cairn.shell: grep -rn 'any\b' ~/cairn-backend/src/ --include='*.ts' | grep -v node_modules | grep -v '.test.' | grep -v '.d.ts' | wc -l
```

**Test coverage gaps**
```
cairn.shell: find ~/cairn-backend/src -name '*.ts' ! -name '*.test.*' ! -name '*.d.ts' -path '*/tools/*' -o -path '*/skills/*' -o -path '*/services/*' | while read f; do test -f "${f%.ts}.test.ts" || echo "NO TEST: $f"; done | head -20
```

## Step 2: Categorize

| Priority | Category | Examples |
|----------|----------|---------|
| **P0: Broken** | CI failures, stuck PRs, blocking bugs | Fix immediately |
| **P1: Committed** | Open issues with assignees, in-progress TODO items `[~]` | Resume or complete |
| **P2: Planned** | Unchecked TODO items `[ ]`, filed issues | Pick strategically |
| **P3: Improve** | Tech debt, missing tests, code smells, type safety | Ongoing quality |
| **P4: Explore** | New capabilities, features from ROADMAP.md, ideas | When bandwidth allows |

## Step 3: Assess codebase health

Look for:
- **Type safety**: `any` count, missing return types
- **Test gaps**: modules without corresponding test files
- **Dead code**: unused exports, unreachable branches
- **Security**: hardcoded secrets, unsafe patterns
- **Performance**: synchronous operations in async paths, N+1 patterns
- **Documentation drift**: CLAUDE.md or API contract stale vs actual code

## Step 4: Present prioritized list

```markdown
## Work Available

### P0: Fix Now
- CI failure on main: [run link] — [what failed]
- Stale PR #N: [title] — needs rebase/review

### P1: Resume
- [~] TODO item: description
- Issue #N: title (assigned to you)

### P2: Next Up
- [ ] TODO item: description
- Issue #N: title

### P3: Improve Quality
- N files missing tests (top: services/X.ts, tools/Y.ts)
- N `any` types could be strengthened
- N TODO/FIXME comments in codebase

### P4: Explore
- ROADMAP item: description

## Recommendation
Start with: [most impactful item] — because [reason]
```

## Step 5: Offer action

- "Start a coding session for [item]?" → `cairn.startCoding` (use Sonnet for small fixes like doc updates, test additions, lint fixes; Opus for complex features)
- "Create an issue for [improvement]?" → `cairn.shell: gh issue create`
- "Show me more details on [item]?"

## Notes

- If $ARGUMENTS provided, focus search on that area (e.g., "find-work tests" → focus on test coverage)
- Check `cairn.codingSessions` for in-progress work before suggesting new tasks
- Don't suggest work that's already being done by another session
- Respect CODING_ALLOWED_REPOS — only suggest coding for allowed repos

