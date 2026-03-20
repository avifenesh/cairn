---
name: daily-standup
description: "Use for morning standup, daily briefing, catching up on what happened, or end-of-day summary. Keywords: standup, morning, briefing, catch up, what happened, summary, EOD"
inclusion: on-demand
allowed-tools: "cairn.digest,cairn.readFeed,cairn.getConfig,cairn.listCrons,cairn.getStatus,cairn.searchMemory,cairn.listTasks"
---

# Daily Standup / Briefing

## Morning Briefing Flow
1. **Digest**: `cairn.digest` — LLM-summarized overview of unread events
2. **Feed check**: `cairn.readFeed` — recent items, filter by source if needed
3. **Task status**: `cairn.listTasks` — pending/running/failed tasks
4. **Cron status**: `cairn.listCrons` — scheduled jobs and their last runs
5. **System health**: `cairn.getStatus` — uptime, pollers, memory count

## Briefing Format
- Lead with the most important item
- Group by category: GitHub activity, emails, news, metrics
- Include counts: "3 new PRs, 5 unread emails, 2 HN stories"
- Flag errors or failures prominently
- End with action items

## End-of-Day Summary
- Review what was accomplished today
- Check completed tasks
- Search memories for today's learnings: `cairn.searchMemory`
- Summarize key decisions and outcomes
