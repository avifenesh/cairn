---
name: plan-check
description: "Compare roadmap and plans against actual code state — find drift between what's documented and what's built. Keywords: plan check, drift, roadmap, reality check, what's done, progress, status, compare plan, alignment, gaps"
---

# Plan Check

Compare documented plans (ROADMAP.md, TODO.md, TECHNICAL_DEBT.md) against actual code state. Surface what's ahead of plan, behind plan, or drifted.

Think and plan — use quiet time to reflect on what's working and what should change.

## Step 1: Load plan documents

```
cairn.shell: cat -n ~/cairn-backend/ROADMAP.md | head -100
cairn.shell: cat -n ~/cairn-backend/TODO.md | head -100
cairn.shell: cat -n ~/cairn-backend/TECHNICAL_DEBT.md | head -80
```

## Step 2: Check claimed completions

For each item marked complete `[x]` in TODO.md:
- Verify the referenced PR was actually merged
- Verify the feature actually exists in code

```
cairn.shell: grep '\[x\]' ~/cairn-backend/TODO.md | tail -15
```

For each, spot-check:
```
cairn.shell: gh pr view PR_NUMBER --repo avifenesh/pub --json state,mergedAt --jq '{state, mergedAt}'
```

## Step 3: Check claimed in-progress

For items marked `[~]` (in progress):
```
cairn.shell: grep '\[~\]' ~/cairn-backend/TODO.md
```

Cross-reference with active coding sessions:
```
cairn.codingSessions: action="list"
```

Flag items marked in-progress but with no active session or recent PR.

## Step 4: Scan for undocumented completions

Check recent merged PRs that might complete TODO items not yet marked:
```
cairn.shell: gh pr list --repo avifenesh/pub --state merged --limit 15 --json number,title,mergedAt
```

Compare PR titles against open TODO items — flag matches.

## Step 5: Architecture drift

Check if documented architecture still matches reality:
- Repo layout in CLAUDE.md vs actual directories
- Listed feature flags vs actual env vars
- Documented tool count vs actual registered tools
- Skill count in SKILL_SYSTEM.md vs actual skills on disk

```
cairn.shell: ls ~/cairn-backend/src/*/  | head -30
cairn.shell: grep "registry.register" ~/cairn-backend/src/tools/builtin/index.ts | wc -l
cairn.shell: ls ~/cairn-backend/.pub/skills/*/SKILL.md | wc -l
```

## Step 6: Present drift report

```markdown
## Plan vs Reality

### Ahead of Plan (done but not documented)
- Feature X exists in code but not in ROADMAP
- PR #N completed TODO item Y but not marked [x]

### Behind Plan (documented but not done)
- [~] Item Z marked in-progress but no active session (stale since DATE)
- [ ] Item W planned for Phase N but no code exists

### Drifted (documented incorrectly)
- CLAUDE.md says N tools but actual count is M
- SKILL_SYSTEM.md lists N skills but M exist on disk
- ROADMAP says Phase X complete but feature Y is missing

### On Track
- N/M TODO items completed
- Current phase: [phase from ROADMAP]
- Active work: [from codingSessions]

### Recommended Actions
1. Mark completed items: [list] → Sonnet coding session
2. Update stale in-progress: [list] → check with Avi
3. Fix doc drift: [list] → Sonnet coding session
```

## Notes

- This is read-only analysis — delegate fixes to `cairn.startCoding` (Sonnet for doc updates)
- Don't mark items complete without verifying the PR actually merged
- Stale `[~]` items (>7 days with no activity) should be flagged
- Check TECHNICAL_DEBT.md for items that were fixed but not removed
