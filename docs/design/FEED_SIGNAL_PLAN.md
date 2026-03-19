# Feed System Overhaul — Signal Intelligence

> Not a notification dump. Surface what matters: external human engagement,
> growth metrics, filtered email, calendar awareness.

## Current State (updated 2026-03-19)

5 pollers working (GitHub notifications, HN, Reddit, NPM, Crates). Event store with dedup.
**PR A COMPLETE (#80)**: Feed API wired, types fixed, archive/delete, source filters, pagination.
- GET /v1/feed wired to EventStore.List() with source/kind/unread/cursor/excludeArchived params
- GET /v1/dashboard returns real stats (total, unread, bySource via CountBySource SQL)
- POST /v1/feed/{id}/read, POST /v1/feed/read-all, POST /v1/feed/{id}/archive, DELETE /v1/feed/{id}
- FeedItem.id fixed: string (was number), archive/delete buttons, source filter chips
- cairn.archiveFeedItem + cairn.deleteFeedItem tools added
- 37 tools total (24 base + 5 Z.ai HTTP + 8 Vision), 39 with GWS
- 232 frontend tests, all backend tests green
Google Workspace CLI (`gws`) authenticated and available with 17 services.

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

### PR D: Future Integrations Spec (updated 2026-03-19)

> Detailed spec for each integration. All follow the existing Poller interface:
> `Source() string` + `Poll(ctx, since) ([]*RawEvent, error)`.
> Config via env vars + PatchableConfig (settings UI).

---

#### 1. X/Twitter Poller

**Available now:** MCP tools (`mcp__twitter__*`) are connected and authenticated.

| Tool | What it does | Polling use |
|------|-------------|-------------|
| `search_twitter` | Search by query, sort by Top/Latest | Track mentions, keywords |
| `get_user_tweets` | User's timeline | Track your own reach |
| `get_latest_timeline` | Following timeline | Curated feed |
| `get_timeline` | For You timeline | Discovery |
| `post_tweet` | Post | Agent can post on your behalf |
| `send_dm` / `delete_dm` | DMs | Agent can respond to DMs |

**Poller design:**
```
Source: "twitter"
Poll cycle (every 5 min):
  1. search_twitter(query="@avifenesh OR from:avifenesh", sort="Latest", count=20)
     → kind=mention for replies/mentions, kind=post for your tweets
  2. search_twitter(query="cairn OR agent-os OR <keywords>", sort="Latest", count=10)
     → kind=post for keyword matches
  Dedup: twitter:<tweet_id>
  Metadata: {author, likes, retweets, replies, isReply, quotedTweet}
```

**Config:**
- `TWITTER_ENABLED` (bool), `TWITTER_KEYWORDS` (comma-sep), `TWITTER_USERNAME` (string)
- MCP calls via agent tool execution (not HTTP — use existing MCP infrastructure)

**Implementation approach:** Since MCP tools are available, the poller would call them via the tool system rather than HTTP. Requires injecting tool execution capability into the poller, or using a dedicated MCP client. Simplest: exec via the agent's tool context.

**Complexity:** Medium — MCP tool bridge needed. Estimated ~150 lines.

---

#### 2. RSS/Atom Feed Poller

**Available:** No Go RSS library installed yet. Best option: `github.com/mmcdole/gofeed` (universal parser, supports RSS 1.0/2.0, Atom, JSON Feed).

**Poller design:**
```
Source: "rss"
Poll cycle (every 30 min):
  For each configured feed URL:
    1. HTTP GET <url>
    2. Parse with gofeed.NewParser().ParseURL()
    3. Filter items by since timestamp (item.PublishedParsed)
    4. Emit events:
       - kind=post for blog posts
       - kind=release for changelogs/release notes
       - Dedup: rss:<feed_url_hash>:<item_guid_or_link>
       - Metadata: {feedTitle, feedURL, author, categories, enclosure}
```

**Use cases:**
- Track competitor blogs, tech blogs (e.g., go.dev/blog, svelte.dev/blog)
- Monitor project changelogs (GitHub releases have Atom feeds: `https://github.com/{owner}/{repo}/releases.atom`)
- Track Hacker News RSS for broader coverage than keyword polling
- Dev.to articles (RSS available: `https://dev.to/feed/{username}`)

**Config:**
- `RSS_FEEDS` (comma-sep URLs), `RSS_ENABLED` (bool)
- Settings UI: textarea for feed URLs

**Complexity:** Low — ~100 lines + `go get github.com/mmcdole/gofeed`. Standard HTTP + parse pattern.

---

#### 3. Stack Overflow Poller

**Available:** REST API at `api.stackexchange.com/2.3/` — no auth needed (300 req/day anonymous, 10K/day with key).

**Poller design:**
```
Source: "stackoverflow"
Poll cycle (every 1 hour):
  1. GET /2.3/questions?tagged=<tags>&sort=creation&order=desc&site=stackoverflow&fromdate=<since_unix>&pagesize=20
     → kind=post for new questions
  2. GET /2.3/questions?tagged=<tags>&sort=activity&order=desc&site=stackoverflow&pagesize=10
     → kind=comment for recently active questions (answers/comments)
  Dedup: stackoverflow:<question_id>
  Metadata: {tags, score, answerCount, viewCount, isAnswered, owner}
```

**Use cases:**
- Track questions about your projects: tagged `[cairn]`, `[mcp]`, `[agent-os]`
- Track your tech stack: `[go]`, `[svelte]`, `[sqlite]`
- Community engagement signal

**Config:**
- `SO_ENABLED` (bool), `SO_TAGS` (comma-sep), `SO_API_KEY` (optional, for higher rate limit)

**Complexity:** Low — ~80 lines. Simple REST + JSON. No auth needed.

---

#### 4. Dev.to Poller

**Available now:** MCP tools (`mcp__devto__*`) are connected.

| Tool | What it does |
|------|-------------|
| `get_articles` | List articles by tag/username/state |
| `get_article` | Get specific article |
| `get_comments` | Get comments on article |
| `get_user` | User profile |
| `get_tags` | Popular tags |
| `search_articles` | Search articles |

**Poller design:**
```
Source: "devto"
Poll cycle (every 30 min):
  1. get_articles(username=<your_username>) → your articles, track comments/reactions
  2. get_articles(tag=<keywords>) → new articles in your interest areas
  3. For articles with new comments: get_comments(article_id) → kind=comment
  Dedup: devto:article:<id> or devto:comment:<id>
  Metadata: {tags, reactions, comments, readingTime, coverImage}
```

**Config:**
- `DEVTO_ENABLED` (bool), `DEVTO_USERNAME` (string), `DEVTO_TAGS` (comma-sep)

**Complexity:** Low — ~100 lines. MCP tools available, similar to Twitter approach.

---

#### 5. Enhanced Reddit (via MCP)

**Available now:** MCP tools (`mcp__reddit__*`) supplement the existing HTTP poller.

The existing `RedditPoller` fetches new posts via anonymous HTTP. MCP tools add:
- `search_reddit` — search across all of Reddit (not just configured subs)
- `browse_subreddit` — hot/top/rising (existing poller only does new)
- `user_analysis` — track your own Reddit presence
- `get_post_details` — deep comment threading

**Enhancement spec:**
- Add keyword search across Reddit (not just configured subs): "cairn", "agent os", "mcp protocol"
- Track mentions of your username
- Track comment replies on your posts

**Complexity:** Low — extend existing poller with MCP calls.

---

#### 6. Cron/Scheduled Tasks

**Design:**
```
New tool: cairn.createCron
  Inputs: schedule (cron expression), action (tool call or message), description
  Stored in: source_state or dedicated cron_jobs table

Scheduler integration:
  - On each tick, check if any cron jobs are due
  - Execute action (tool call or send message to agent)
  - Emit event: source=cron, kind=scheduled, title=description

Use cases:
  - "Remind me to check CI every morning at 9am"
  - "Run digest every Friday at 5pm"
  - "Check if my server is up every hour"
```

**Config:**
- No env vars — managed via tool (`cairn.createCron`, `cairn.listCrons`, `cairn.deleteCron`)
- Persisted in SQLite

**Complexity:** Medium — ~200 lines. New table, cron expression parser (`github.com/robfig/cron/v3`).

---

#### 7. Webhook Improvements

**Already exists:** `internal/signal/webhook.go` with HMAC-SHA256 verification.

**Enhancements:**
- Generic webhook templates (parse JSON body into RawEvent fields via JSONPath)
- Webhook management UI in settings (list, create, delete, test)
- Pre-built templates: Stripe (payment events), Vercel (deploy events), Linear (issue events)

---

#### Priority Order (recommended)

1. **RSS/Atom** — lowest effort, highest utility (blogs, changelogs, release feeds)
2. **Stack Overflow** — low effort, good for community engagement
3. **Dev.to** — MCP tools ready, low effort
4. **Twitter/X** — medium effort (MCP bridge), high value for social presence
5. **Cron** — medium effort, enables proactive agent behavior
6. **Enhanced Reddit** — extend existing poller
7. **Webhook templates** — nice-to-have polish

#### Existing Tools (documented)

These tools already exist and work. No implementation needed:
- `cairn.shell` — shell commands
- `cairn.gitRun` — git operations
- `cairn.gwsQuery` / `cairn.gwsExecute` — Google Workspace (17 services)
- `cairn.webSearch` / `cairn.webFetch` — web search + fetch
- `cairn.readFeed` / `cairn.markRead` / `cairn.archiveFeedItem` / `cairn.deleteFeedItem` — feed management
- `cairn.imageAnalysis` + 7 vision tools — visual intelligence
- `cairn.searchDoc` / `cairn.repoStructure` / `cairn.readRepoFile` — code search (Z.ai)

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
PR A (Wire Feed API)          ← DONE (#80)
  ↓
PR B (GitHub Signal)          ← DONE (#81) (engagement, metrics, stargazers, followers, new repos, bot filter)
PR C (Gmail + Calendar)       ← DONE (#84) (filtered email + auto-archive GH emails + calendar via gws CLI)
  ↓
PR D (Future Spec)            ← DONE (docs + RSS/SO/DevTo implemented in #85, 11 pollers total)
```

## Implementation Order

1. **PR A** — wire feed API, fix types, archive/delete — **DONE (#80)**
2. **PR B** — GitHub signal intelligence — **DONE (#81)**
3. **PR C** — Gmail + Calendar pollers — **DONE (#84)**
4. **PR D** — write specs only

## Success Criteria

- Feed page shows real events from all configured sources
- No bot comments in feed
- Growth metrics visible as daily deltas
- Email filtered (no GitHub notifications)
- Calendar events show upcoming 48h
- Archive/delete persists across refresh
- Agent can read, archive, delete feed items via tools
