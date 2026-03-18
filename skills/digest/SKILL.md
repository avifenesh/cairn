---
name: digest
description: "Use when user asks for a digest, summary of updates, what's new, catch me up, or daily briefing. Keywords: digest, summary, updates, what's new, catch up, briefing"
inclusion: on-demand
allowed-tools: "cairn.digest,cairn.readFeed,cairn.markRead"
---

# Feed Digest

Generate a prioritized digest of recent events:

1. **Generate** — Use `cairn.digest` to get an LLM-summarized overview of unread events.
2. **Detail** — If the user wants more detail on a source, use `cairn.readFeed` with a source filter.
3. **Triage** — After presenting the digest, offer to mark events as read with `cairn.markRead`.

## Priority Order

1. Security alerts and critical issues
2. PRs and issues requiring action
3. New releases of tracked packages
4. Community discussions and news

## Format

Present as a concise briefing:
- Lead with the most important items
- Group by source when there are many events
- Include counts (e.g., "3 new PRs, 5 HN stories")
- End with "Mark all as read?" prompt
