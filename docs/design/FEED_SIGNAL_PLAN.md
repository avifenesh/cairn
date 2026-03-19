# Feed System Overhaul — Signal Intelligence

> Not a notification dump. Surface what matters: external human engagement,
> growth metrics, filtered email, calendar awareness.

## Current State

5 pollers working (GitHub notifications, HN, Reddit, NPM, Crates). Event store with dedup.
Feed API routes **stubbed** (return empty — not wired to EventStore). Tools work (readFeed, markRead, digest).
FeedItem type mismatch: frontend expects `id: number`, backend produces `ev_<hex>` string.
No archive/delete API despite `archived_at` column in schema.
Google Workspace CLI (`gws`) authenticated and available with 17 services.
37 tools total (22 base + 5 Z.ai HTTP + 8 Vision + 2 GWS).

## Signal Philosophy

| Category | What to Surface | What to Filter |
|----------|----------------|----------------|
| **GitHub Engagement** | External users: issues, PRs, comments, discussions | Bot comments (dependabot, copilot, gemini, claude-review, renovate, snyk, etc.) |
| **GitHub Growth** | Stars, forks, follows gained (as deltas) | Your own activity, internal commits |
| **Gmail** | Real human emails, important service notifications | GitHub notification emails, promotions, social, marketing |
| **Calendar** | Upcoming events (48h window), new invitations | Past events |
| **HN/Reddit/NPM/Crates** | Existing pollers (keywords, packages) | Already filtered by config |

## Phase Plan — 4 PRs

---

### PR A: Wire Feed API + Fix Types + Archive/Delete

**Priority: HIGHEST — unblocks everything else**

#### Backend

1. **Wire `GET /v1/feed`** to `EventStore.List()`
   - Accept query params: `source`, `kind`, `unreadOnly`, `limit`, `before` (cursor)
   - Return: `{ items: StoredEvent[], hasMore: boolean }`
   - Currently returns empty array (stub at routes.go:128)

2. **Add `POST /v1/feed/{id}/archive`**
   - Set `archived_at` timestamp on event
   - Return `{ ok: true }`

3. **Add `DELETE /v1/feed/{id}`**
   - Hard delete from events table
   - Return `{ ok: true }`

4. **Add `POST /v1/feed/read` (bulk)**
   - Body: `{ ids: string[] }` or `{ all: true }`
   - Mark multiple events read at once

5. **New tools:**
   - `cairn.archiveFeedItem` — archive by ID
   - `cairn.deleteFeedItem` — delete by ID

6. **EventStore additions:**
   - `Archive(ctx, id) error` — set archived_at
   - `DeleteByID(ctx, id) error` — hard delete single event

#### Frontend

1. **Fix FeedItem type**: `id: number` → `id: string` (match backend `ev_<hex>`)
2. **Wire `GET /v1/feed`** in client.ts `getFeed()` — currently returns mock data
3. **Archive button** on FeedItem (alongside mark-read check icon)
4. **Delete** in bulk actions bar
5. **Filter chips**: source filter (github, gmail, calendar, hn, reddit, npm, crates)
6. **Pagination**: "Load more" with cursor-based fetch

#### Files

| File | Change |
|------|--------|
| `internal/server/routes.go` | Wire handleListFeed, add archive/delete endpoints |
| `internal/signal/event_store.go` | Add Archive(), DeleteByID() methods |
| `internal/tool/builtin/feed.go` | Add archiveFeedItem, deleteFeedItem tools |
| `internal/tool/builtin/register.go` | Register new feed tools |
| `frontend/src/lib/types.ts` | Fix FeedItem.id to string |
| `frontend/src/lib/api/client.ts` | Wire getFeed, archiveFeed, deleteFeed |
| `frontend/src/lib/stores/feed.svelte.ts` | Add archiveItem, removeItem |
| `frontend/src/lib/components/feed/FeedItem.svelte` | Archive button |
| `frontend/src/routes/today/+page.svelte` | Source filters, pagination, bulk delete |

---

### PR B: GitHub Signal Intelligence

**Replace noisy notifications with meaningful signals**

#### New Poller: `github_signal`

Replaces the existing GitHub notifications poller with targeted queries.

##### External Engagement (per tracked repo)

```
For each repo in (avifenesh/* + agent-sh/*):

1. Issues by external users:
   GET /repos/{owner}/{repo}/issues?state=open&sort=created&since={since}&per_page=30
   Filter: author != owner AND author NOT IN botList
   Event: source=github, kind=issue, groupKey=repo

2. PRs by external users:
   GET /repos/{owner}/{repo}/pulls?state=open&sort=created&since={since}&per_page=30
   Filter: same
   Event: source=github, kind=pr, groupKey=repo

3. Issue/PR comments by external humans:
   GET /repos/{owner}/{repo}/issues/comments?sort=created&since={since}&per_page=50
   Filter: author != owner AND NOT bot
   Event: source=github, kind=comment, groupKey=repo

4. Discussions (if enabled):
   GraphQL: repository.discussions(first:10, orderBy:{field:CREATED_AT})
   Filter: author != owner
   Event: source=github, kind=discussion, groupKey=repo
```

##### Bot Filter List

```go
var defaultBotFilter = []string{
    "dependabot", "dependabot[bot]", "github-actions[bot]",
    "copilot", "gemini-code-assist[bot]", "chatgpt-codex-connector[bot]",
    "claude-review[bot]", "renovate[bot]", "snyk-bot",
    "codecov[bot]", "stale[bot]", "allcontributors[bot]",
    "gitguardian[bot]", "sonarcloud[bot]",
}

func isBot(login string) bool {
    // Check exact match
    // Check suffix patterns: *[bot], *-bot, *-action
    // Check config override: GH_BOT_FILTER env var
}
```

##### Growth Metrics (snapshot + delta)

```
For each tracked repo:

1. Snapshot current metrics:
   GET /repos/{owner}/{repo}
   → { stargazers_count, forks_count, subscribers_count, open_issues_count }

2. Compare to previous snapshot (stored in source_state.extra):
   delta = current - previous
   If delta.stars > 0: emit event "cairn gained 3 stars"
   If delta.forks > 0: emit event "cairn gained 1 fork"

3. Store new snapshot for next comparison

Interval: every 4 hours (separate from engagement polling)
```

##### New Stargazers (who starred)

```
GET /repos/{owner}/{repo}/stargazers
Headers: Accept: application/vnd.github.star+json
→ [{ user: { login }, starred_at }]

Compare to previous list → new entries since last poll
Event: source=github, kind=star, actor=login, title="starred {repo}"
```

##### New Followers

```
GET /users/avifenesh/followers → list of logins
Compare to previous list (stored in source_state.extra)
New followers → event: source=github, kind=follow, actor=login
```

#### Config

```
GH_TRACKED_REPOS     []string  // auto-detect: all repos from user + org
GH_TRACKED_ORGS      []string  // "agent-sh" (existing GH_ORGS)
GH_BOT_FILTER        []string  // additional bot logins to filter
GH_OWNER             string    // "avifenesh" — your login (for self-filter)
GH_METRICS_INTERVAL  int       // seconds (default 14400 = 4h)
```

#### Rate Limit Management

- GitHub: 5000 req/hr REST + 5000 GraphQL
- Space calls ≥2s apart (prevent secondary rate limits)
- Batch repos: 1 call per repo per endpoint per poll
- Skip metrics snapshot if approaching rate limit (< 500 remaining)
- Engagement: poll every 5 min
- Metrics: poll every 4 hours

#### Files

| File | Change |
|------|--------|
| `internal/signal/github_signal.go` | **NEW**: engagement + metrics + stargazers + followers poller |
| `internal/signal/bot_filter.go` | **NEW**: bot detection (list + patterns) |
| `internal/signal/github_signal_test.go` | **NEW**: tests |
| `internal/config/config.go` | Add GH_TRACKED_REPOS, GH_BOT_FILTER, GH_OWNER, GH_METRICS_INTERVAL |
| `cmd/cairn/main.go` | Register github_signal poller with scheduler |

---

### PR C: Gmail + Calendar Pollers

**Filtered email + calendar awareness via gws CLI**

#### Gmail Poller

Uses `gws` CLI — no OAuth complexity, already authenticated.

```
Poll cycle (every 5 min):

1. gws gmail users messages list --params '{
     "userId": "me",
     "maxResults": 20,
     "q": "-from:notifications@github.com -from:noreply@github.com -category:promotions -category:social -category:forums"
   }'

2. For each new message ID (dedup: gmail:<messageId>):
   gws gmail users messages get --params '{
     "userId": "me",
     "id": "<messageId>",
     "format": "metadata"
   }'
   Extract: From, Subject, Date, snippet

3. Ingest as event:
   source: "gmail"
   kind: "email"
   title: Subject
   body: snippet (truncated 200 chars)
   actor: From (parsed name)
   url: "https://mail.google.com/mail/u/0/#inbox/<messageId>"
   metadata: { from, to, threadId, labelIds }
```

#### Calendar Poller

```
Poll cycle (every 15 min):

1. gws calendar events list --params '{
     "calendarId": "primary",
     "maxResults": 20,
     "timeMin": "<now>",
     "timeMax": "<now+48h>",
     "singleEvents": true,
     "orderBy": "startTime"
   }'

2. For each event (dedup: calendar:<eventId>:<startTime>):
   source: "calendar"
   kind: "event"
   title: summary
   body: description (truncated 200 chars)
   url: htmlLink
   metadata: { start, end, location, attendees, status, creator }

New invitations: status="needsAction" → kind="invitation"
```

#### Config

```
GMAIL_POLL_ENABLED     bool     // default true when gws available
CALENDAR_POLL_ENABLED  bool     // default true when gws available
GMAIL_FILTER_QUERY     string   // custom Gmail search query override
CALENDAR_LOOKAHEAD_H   int      // hours ahead (default 48)
```

#### Files

| File | Change |
|------|--------|
| `internal/signal/gmail.go` | **NEW**: Gmail poller via gws CLI |
| `internal/signal/calendar.go` | **NEW**: Calendar poller via gws CLI |
| `internal/signal/gmail_test.go` | **NEW**: tests |
| `internal/signal/calendar_test.go` | **NEW**: tests |
| `internal/config/config.go` | Add Gmail/Calendar config |
| `cmd/cairn/main.go` | Register pollers with scheduler |

---

### PR D: Future Integrations (spec only — not implementing now)

Design the poller interface and config for:

| Source | API | Frequency | Signal |
|--------|-----|-----------|--------|
| **X/Twitter** | MCP `mcp__twitter__*` or REST API | 5 min | Mentions, replies, DMs, quote tweets |
| **RSS/Atom** | HTTP GET + XML parse | 30 min | Blog posts, changelogs, release notes |
| **Stack Overflow** | REST API | 1 hour | Questions tagged with your tech stack |
| **Product Hunt** | REST API | 1 hour | Launches in your interest areas |
| **Cron Jobs** | `cairn.createCron` tool | user-defined | Scheduled tasks, reminders |

Tools that already exist and should be documented:
- `cairn.shell` — shell commands (exists)
- `cairn.gitRun` — git operations (exists)
- `cairn.gwsQuery` / `cairn.gwsExecute` — Google Workspace (exists)
- `cairn.webSearch` / `cairn.webFetch` — web (exists)

---

## Frontend Feed Redesign (across all PRs)

### Today Page Improvements

1. **Source filter chips**: github, gmail, calendar, hn, reddit, npm, crates, webhook
2. **Kind filter chips**: issue, pr, comment, star, fork, email, event, story, post, package
3. **Group by source** or **chronological** toggle
4. **Growth summary card**: "Today: +5 stars, +2 forks, 3 new issues, 12 emails"
5. **Pagination**: cursor-based "Load more" button
6. **Archive/Delete**: archive button on items, bulk delete in toolbar
7. **Calendar widget**: upcoming events sidebar (next 24h)

### New FeedItem Variants

| Source | Kind | Display |
|--------|------|---------|
| github | issue/pr | Title + repo badge + author avatar + "external" badge |
| github | comment | Quote snippet + repo + thread link |
| github | star | "username starred repo" + avatar |
| github | fork | "username forked repo" |
| github | follow | "username followed you" |
| github | metrics | "cairn: +3 stars, +1 fork today" (summary card) |
| gmail | email | Subject + From + snippet + time |
| calendar | event | Title + time + location + "in 2h" badge |
| calendar | invitation | Title + time + "RSVP" action buttons |

---

## Dependency Graph

```
PR A (Wire Feed API)          ← FOUNDATION, do first
  ↓
PR B (GitHub Signal)          ← depends on A for feed display
PR C (Gmail + Calendar)       ← depends on A for feed display
  ↓
PR D (Future Spec)            ← independent, docs only
```

## Implementation Order

1. **PR A** — wire feed API, fix types, archive/delete (~1 session)
2. **PR B** — GitHub signal intelligence (~1-2 sessions)
3. **PR C** — Gmail + Calendar pollers (~1 session)
4. **PR D** — write specs only

## Success Criteria

- Feed page shows real events from all configured sources
- No bot comments in feed
- Growth metrics visible as daily deltas
- Email filtered (no GitHub notifications)
- Calendar events show upcoming 48h
- Archive/delete persists across refresh
- Agent can read, archive, delete feed items via tools
