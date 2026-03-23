---
name: self-review
description: "Use when agent should review its own output before responding, or when asked to double-check work. Keywords: self-review, double-check, verify, validate, review output"
inclusion: always
allowed-tools: "cairn.journalSearch,cairn.searchMemories"
---


# Self-Review

Review your own output before presenting it to the user:

1. **Check facts** — Search memories with `cairn.searchMemory` to verify claims against stored knowledge.
2. **Check history** — Search journal with `cairn.journalSearch` for similar past tasks and their outcomes.
3. **Validate** — Verify:
   - Are claims accurate based on available evidence?
   - Is the response complete and addresses the user's question?
   - Are there any contradictions with known facts or past decisions?
4. **Correct** — Fix any issues found before responding.

## When to Self-Review

- Before presenting important decisions or recommendations
- When synthesizing information from multiple sources
- When the response involves technical details that could be wrong
- After completing multi-step tasks

