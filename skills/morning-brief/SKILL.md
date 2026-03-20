---
name: morning-brief
description: "Start-of-day briefing combining calendar, email, GitHub, and feed into one quick scan. Keywords: morning, brief, start of day, daily, today, good morning, what's on today, schedule, agenda"
inclusion: "on-demand"
allowed-tools: "cairn.shell,cairn.searchFeed,cairn.getStatus"
---

# Morning Brief

Combined daily briefing — 30-second scan of everything that matters today.

## Steps (run in parallel where possible)

1. **Today's calendar**
   ```
   cairn.shell: gws calendar events list --params '{"calendarId":"primary","maxResults":10,"timeMin":"<today 00:00 ISO>","timeMax":"<today 23:59 ISO>","singleEvents":true,"orderBy":"startTime"}' --format json
   ```

2. **Unread email count + top urgent**
   ```
   cairn.shell: gws gmail users messages list --params '{"userId":"me","maxResults":5,"q":"is:unread is:important"}' --format json
   ```

3. **GitHub status**
   ```
   cairn.shell: gh pr list --repo avifenesh/pub --state open --json number,title,updatedAt --limit 5
   cairn.shell: gh run list --repo avifenesh/pub --limit 3 --json name,conclusion,headBranch
   ```

4. **Feed highlights**
   ```
   searchFeed: query="", unreadOnly=true, limit=10
   ```

5. **Format as brief**

```markdown
## Good morning — [date]

### Calendar (N events today)
- 09:00 — Meeting name
- 14:00 — Meeting name
- No conflicts detected

### Email (N unread, M important)
- [urgent] Subject — from sender
- [urgent] Subject — from sender

### GitHub
- N open PRs (oldest: #123, 3 days)
- CI: all green / 1 failure on branch X

### Feed Highlights
- Top item from each source (if unread)

### Suggested Focus
- Priority 1: [most urgent item across all sources]
- Priority 2: [next most important]
```

## Notes

- Keep the entire brief under 30 lines — this is a scan, not a report
- If calendar is empty, say "No meetings today — deep work day"
- If everything is quiet, say "All clear — nothing urgent"
- Always end with 1-2 suggested focus items
