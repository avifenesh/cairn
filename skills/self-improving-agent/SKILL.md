---
name: self-improving-agent
description: "Formalized learning loop that captures learnings, errors, and corrections to Cairn's memory system for continuous improvement. Use when: self-review, self-improve, learning loop, what did I learn, improve myself, reflect, what went wrong, error patterns, metacognition, weekly review, log this learning, should I log this. Keywords: self-review, self-improve, reflect, learn, errors, corrections, patterns, metacognition, soul, review, learning"
allowed-tools: "cairn.shell,cairn.searchMemories,cairn.listMemories,cairn.createMemory,cairn.createArtifact,cairn.getStatus,cairn.journalSearch"
disable-model-invocation: false
inclusion: always
context: "chat,tick"
---

# Self-Improving Agent — Learning Loop

Captures learnings, errors, and corrections into Cairn's memory system for continuous improvement.

## Quick Reference

| Situation | Action |
|-----------|--------|
| Command/operation fails | Propose error pattern via `cairn.createMemory` (category: `fact`, **proposed: true**) |
| User corrects you | Propose correction via `cairn.createMemory` (category: `hard_rule` or `preference`, **proposed: true**) |
| User wants missing capability | Propose as feature insight via `cairn.createMemory` (category: `decision`, **proposed: true**) |
| API/external tool fails | Propose workaround via `cairn.createMemory` (category: `fact`, **proposed: true**) |
| Knowledge was outdated | Update or propose replacement for stale memory |
| Found better approach | Propose via `cairn.createMemory` (category: `decision` or `preference`, **proposed: true**) |
| Broadly applicable pattern | Propose SOUL.md change via approval flow (see Step 4) |
| User corrects identity/approach | Immediately propose SOUL.md update as `doc_patch` artifact |
| Similar to existing memory | Search first, update existing memory instead of creating duplicate |
| Periodic review requested | Run full self-review protocol (Steps 1-6 below) |

**CRITICAL: Always use `proposed: true` when creating memories.** This ensures the user reviews and approves every learning before it becomes active. Never create memories as accepted directly — the user decides what to remember.

## Detection Triggers

Log a learning when you notice any of these signals:

**Corrections** (→ `hard_rule` or `preference`):
- "No, that's not right...", "Actually, it should be..."
- "You're wrong about...", "That's outdated..."
- "I told you before...", "Always/never do X"

**Feature gaps** (→ `decision`):
- "Can you also...", "I wish you could..."
- "Is there a way to...", "Why can't you..."

**Knowledge gaps** (→ `fact`):
- User provides information you didn't know
- Documentation you referenced is outdated
- API behavior differs from your understanding

**Errors** (→ `fact` with workaround):
- Tool returns error status
- Command returns non-zero exit code
- Unexpected output or timeout

**Best practices** (→ `decision` or `preference`):
- A better approach is discovered for a recurring task
- User praises a particular approach ("yes, always do it this way")

## Guardrails

- Max 3 new memories per self-review session
- SOUL.md changes are proposed as artifacts only — NEVER auto-applied
- Flag issues but do not auto-delete memories (user decides)
- Search for existing memories before creating new ones (avoid duplicates)
- Periodic review: run weekly or on-demand, not more frequently

## Memory Format

When creating memories via `cairn.createMemory`, always use `proposed: true`:

- **content**: Concise, actionable statement. Format: "[Context]: [What to do/know]. [Why, if non-obvious]."
- **category**: `hard_rule` (must always/never), `preference` (user likes), `fact` (how things work), `decision` (chosen approach), `writing_style` (communication patterns)
- **scope**: `global` (cross-session behavior) or `project` (repo-specific)
- **proposed**: `true` (always — user must approve before memory becomes active)

**Good examples:**
- `hard_rule`: "Never auto-apply changes to SOUL.md — always create an artifact for user review"
- `preference`: "Avi prefers concise responses without emoji unless explicitly requested"
- `fact`: "cairn.shell only inherits SAFE_ENV_KEYS (PATH, HOME, etc.) — use builtin tools instead of curl for authenticated API calls"
- `decision`: "For skill REST API queries, prefer builtin tools (cairn.listMemories, cairn.getStatus) over curl"

**Bad examples** (do NOT create):
- "I made a mistake today" (too vague, not actionable)
- "Error occurred in tool X" (one-time, no pattern)
- "The user seems frustrated" (observation, not knowledge)

## Self-Review Protocol

Run this when asked for a self-review, weekly review, or "what did I learn?"

**IMPORTANT:** Before starting, review your session journal for concrete evidence:
- Call **cairn.journalSearch** with `query: "failure"`, `sinceHours: 168` (7 days) to find past failures
- Call **cairn.journalSearch** with `query: "learning"`, `sinceHours: 168` to find past learnings
- Check the "Latest Reflection" section in your context for patterns already identified
- Base all proposals on specific journal entries, not guesswork

### Step 1: Error Analysis

Call **cairn.getStatus** with `errorWindowMs: 604800000` (7 days).

From `toolErrors`, identify tools with recurring failures. For each:
1. Note tool name and error count
2. Search for existing workaround: **cairn.searchMemories** with `q: "<tool-name> error"`, `limit: 5`
3. If recurring pattern has no memory, draft a workaround

**Output:** Error summary table.

### Step 2: Correction Tracking

**2a.** Call **cairn.listMemories** with `status: "rejected"`, `limit: 30`

Analyze rejected memories for themes:
- Was the assistant extracting noise instead of signal?
- Were memories too specific or too vague?
- Did rejections cluster around a topic?

**2b.** Call **cairn.searchMemories** with `q: "always never from now on prefer"`, `limit: 20`

Cluster accepted correction-type memories by theme.

### Step 3: Pattern Recognition

**3a.** Call **cairn.getStatus** with `sections: ["memory"]`

Extract memory stats. Compute acceptance rate. Evaluate:
- Acceptance rate < 50% → extraction too aggressive
- Large proposed backlog → memories need review
- Very few total → assistant not learning enough

**3b.** For each category, call **cairn.listMemories** with `status: "accepted"`, `category: "<cat>"`, `limit: 20` where `<cat>` is each of: `hard_rule`, `preference`, `fact`, `decision`, `writing_style`.

Look for:
- Contradictions (e.g., "always X" vs "never X")
- Stale facts (outdated information)
- Near-duplicates that should be consolidated
- Category gaps (too few in an important category)

### Step 4: Promotion Check — SOUL.md Evolution

Read `SOUL.md` via cairn.shell:
```
cat SOUL.md
```

Compare findings from Steps 1-3 against current SOUL.md. Promote to SOUL.md when ALL of these apply:
- Pattern has recurred 3+ times (across multiple conversations)
- Applies broadly (not one-off)
- Occurred within a 30-day window

If changes warranted, create an **approval request** so the user can review and approve:

1. First, create a task to track the proposal:
```
use_tool: cairn.listTasks  (to get a parent task ID context)
```

2. Create an artifact with the proposed changes:
```json
cairn.createArtifact({
  "type": "doc_patch",
  "title": "SOUL.md Evolution Proposal — [date]",
  "contentJson": {
    "targetFile": "SOUL.md",
    "rationale": "Based on N recurring patterns...",
    "proposedChanges": [
      { "section": "section name", "action": "add|modify|remove", "content": "..." }
    ]
  }
})
```

3. Send a push notification so the user knows to review it:
```
/home/ubuntu/bin/notify "SOUL.md change proposed — review in Artifacts panel" 7 "Self-Review"
```

Write promoted rules as short prevention rules (what to do), not incident write-ups.

**CRITICAL: Never write to SOUL.md directly. Always create an artifact + notification for user review. The user applies the patch manually after approval.**

**CRITICAL: Never write to SOUL.md directly. Always create an artifact for user review.**

### Step 5: Propose Learnings

Based on steps 1-4, draft up to 3 new memories. Each must be:
- **Durable** — a pattern, not a one-time observation
- **Actionable** — tells the assistant what to do differently
- **Deduplicated** — search first to avoid duplicates

**Present each proposed memory to the user** with its content, category, and rationale. Wait for user confirmation before saving.

**Important:** Always pass `proposed: true` to `cairn.createMemory`. This creates the memory as "proposed" — the user must approve it in the Memory panel before it becomes active.

### Step 6: Self-Review Report

```markdown
## Self-Review Report — [date]

### Error Summary
| Tool | Errors (7d) | Memory Exists | Action |
|------|-------------|---------------|--------|
| ... | N | Yes/No | ... |

### Correction Analysis
- **Theme 1**: [description] (N directives)
- **Theme 2**: [description] (N directives)
- Rejected memory themes: [summary]

### Memory Health
- Total: N | Accepted: N | Proposed: N | Rejected: N
- Acceptance rate: X%
- Category distribution: [breakdown]
- Issues: [contradictions, staleness, gaps]

### SOUL.md Status
- [Aligned / Proposal created as artifact #ID]

### New Memories Proposed
- [0-3 memories proposed this session]

### Recommended Next Actions
1. [Most important action]
2. [Second priority]
```

## Priority Guidelines

| Priority | When to Use |
|----------|-------------|
| `hard_rule` | Blocks core functionality, user explicitly said always/never |
| `preference` | User expressed preference, workaround exists |
| `fact` | How things work, API behavior, system constraints |
| `decision` | Chosen approach after evaluating alternatives |

## Notes

- Uses Cairn's memory system (not file-based)
- Tool error data: `cairn.getStatus` reads `tool_calls` table
- Memory data: `cairn.searchMemories`, `cairn.listMemories`, `cairn.getStatus`
- Shell only used for reading `SOUL.md`
- Real-time learning: `MemoryExtractor` catches correction signals automatically — this skill provides the periodic review layer
- Detection triggers can be used during any conversation, not just during explicit self-review
