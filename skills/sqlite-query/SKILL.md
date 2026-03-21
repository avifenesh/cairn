---
name: sqlite-query
description: "Use when user asks to query the database, check stats, count events, analytics questions, SQL queries. Keywords: query, stats, count, how many, top, database, sql, events this week, source volume, acceptance rate"
inclusion: on-demand
allowed-tools: "cairn.shell"
---


# SQLite Query — Read-Only Database Analytics

Run ad-hoc read-only SQL queries against Cairn's SQLite database. Answer questions like "how many events this week?", "top sources by volume", "memory acceptance rate", or any custom SELECT.

## Database Access Pattern

**DB path:** `/home/ubuntu/cairn/data/cairn.db`

Always open read-only with a timeout:

```
timeout 5 sqlite3 "file:/home/ubuntu/cairn/data/cairn.db?mode=ro" <<'SQL'
.headers on
.mode markdown
SELECT ...;
SQL
```

Always use heredoc with single-quoted delimiter (`<<'SQL'`) to prevent shell variable expansion in queries. For multiple statements:

```
timeout 5 sqlite3 "file:/home/ubuntu/cairn/data/cairn.db?mode=ro" <<'SQL'
.headers on
.mode markdown
SELECT source, COUNT(*) as cnt FROM events GROUP BY source ORDER BY cnt DESC LIMIT 10;
SQL
```

Use `.mode markdown` with `.headers on` for human-readable tables. Use `.mode csv` for data processing.

## Schema Reference

### Signal Plane

**`events`** — normalized feed items from all sources
| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | autoincrement |
| source | TEXT | github, reddit, hackernews, npm, gmail, etc. |
| source_item_id | TEXT | unique per source |
| kind | TEXT | pr, issue, comment, post, email, release, etc. |
| title | TEXT | |
| body | TEXT | nullable |
| url | TEXT | nullable |
| actor | TEXT | nullable |
| repo | TEXT | nullable |
| metadata_json | TEXT | JSON blob, default '{}' |
| created_at | TEXT | **ISO datetime** e.g. `2026-03-13T10:30:00.000Z` |
| observed_at | TEXT | ISO datetime |
| read_at | TEXT | nullable, ISO datetime |
| archived_at | TEXT | nullable, ISO datetime |

Unique constraint: `(source, source_item_id)`

**`source_state`** — poller cursor per source
| Column | Type | Notes |
|--------|------|-------|
| source | TEXT PK | |
| cursor_json | TEXT | JSON |
| updated_at | TEXT | ISO datetime |

**`dead_letters`** — failed ingestion attempts
| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | autoincrement |
| source | TEXT | |
| occurred_at | TEXT | ISO datetime |
| error | TEXT | |
| attempts | INTEGER | default 0 |
| checkpoint | TEXT | nullable |
| recorded_at | TEXT | ISO datetime |

### Action Plane

**`tasks`** — all work items
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| status | TEXT | queued/running/awaiting_approval/blocked/completed/failed/canceled |
| type | TEXT | assistant_chat/draft_email/code_task/agent_run/etc. |
| priority | TEXT | low/normal/high/critical |
| cost | TEXT | **JSON**: `{"inputTokens":N,"outputTokens":N,"estimatedCostUsd":N}` |
| created_at | INTEGER | **epoch milliseconds** |
| updated_at | INTEGER | epoch ms |
| workflow_template | TEXT | nullable |
| workflow_step | TEXT | nullable |
| retry_count | INTEGER | default 0 |
| parent_task_id | TEXT | nullable, FK to tasks |
| delegation_depth | INTEGER | default 0 |
| archived_at | INTEGER | nullable |

**`artifacts`** — generated content
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| type | TEXT | email/post/slide_deck/itinerary/checklist/summary/pr_patch/agent_log/doc_patch/digest |
| title | TEXT | |
| content_json | TEXT | JSON blob |
| rendered_text | TEXT | nullable |
| created_at | INTEGER | **epoch ms** |
| updated_at | INTEGER | epoch ms |
| version | INTEGER | default 1 |
| sensitivity | TEXT | normal/sensitive/secret |
| archived_at | INTEGER | nullable |

**`approvals`** — human-in-the-loop gates
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| task_id | TEXT | FK to tasks |
| approval_type | TEXT | send_email/merge_pr/open_pr/budget_override/etc. |
| status | TEXT | pending/approved/denied/expired |
| preview_json | TEXT | JSON |
| created_at | INTEGER | **epoch ms** |
| decided_at | INTEGER | nullable |
| policy_id | TEXT | nullable |

**`tool_calls`** — tool execution log
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| task_id | TEXT | FK to tasks |
| tool_name | TEXT | |
| risk_level | TEXT | read/write/execute |
| status | TEXT | ok/error |
| started_at | INTEGER | **epoch ms** |
| ended_at | INTEGER | nullable |

**`workflow_step_log`** — workflow progression history
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| task_id | TEXT | FK to tasks |
| from_step | TEXT | nullable |
| to_step | TEXT | |
| status | TEXT | ok/error/skipped/retry |
| created_at | INTEGER | epoch ms |

### Memory System

**`memory_items`** — extracted knowledge
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| content | TEXT | non-empty |
| status | TEXT | proposed/accepted/rejected |
| category | TEXT | hard_rule/writing_style/preference/fact/decision |
| scope | TEXT | personal/project/global |
| usage_count | INTEGER | |
| confidence | REAL | 0.0 to 1.0 |
| created_at | TEXT | **ISO datetime** |
| updated_at | TEXT | ISO datetime |
| last_used_at | TEXT | nullable, ISO datetime |

### Agents

**`agents`** — registered agent endpoints
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| name | TEXT | unique |
| endpoint | TEXT | |
| status | TEXT | idle/busy/offline/draining |
| last_heartbeat_at | INTEGER | **epoch ms** |
| max_concurrent | INTEGER | default 1 |

**`agent_task_assignments`** — agent-to-task mapping
| Column | Type | Notes |
|--------|------|-------|
| agent_id | TEXT | composite PK |
| task_id | TEXT | composite PK |
| assigned_at | INTEGER | epoch ms |

### Automation

**`automation_rules`** — event-triggered or scheduled rules
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| name | TEXT | unique |
| enabled | INTEGER | 0/1 |
| trigger_json | TEXT | JSON |
| cooldown_ms | INTEGER | |
| budget_cap_usd | REAL | |
| created_at | INTEGER | epoch ms |

**`automation_rule_executions`** — execution history
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| rule_id | TEXT | FK |
| task_id | TEXT | nullable FK |
| cost_usd | REAL | |
| fired_at | INTEGER | epoch ms |
| status | TEXT | fired/task_created/skipped_cooldown/skipped_budget/error |

### Other Tables

**`approval_policies`** — auto-approve/deny rules (id, name, conditions_json, action, priority)

**`assistant_coding_sessions`** — coding task tracking (task_id PK, repo, status, pr_url, pr_number)

**`conversation_extractions`** — memory extraction tracking (conversation_id PK, extracted_at epoch ms)

### SENSITIVE — Do Not Query

`sessions`, `webauthn_credentials`, `webauthn_challenges`, `shell_grants`, `assistant_session_messages`

## Timestamp Formats

Two formats coexist. Using the wrong one returns zero rows.

| Tables | Format | Example |
|--------|--------|---------|
| events, source_state, dead_letters, memory_items | ISO TEXT | `2026-03-13T10:30:00.000Z` |
| tasks, artifacts, approvals, tool_calls, agents, automation_rules | Epoch INTEGER (ms) | `1741862400000` |

**ISO tables — filter by relative date** (use `strftime` with `T` separator to match ISO format):
```sql
WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-7 days')
WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', 'start of day')
WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-1 month')
```

**Epoch ms tables — filter by relative date:**
```sql
WHERE created_at >= (strftime('%s', 'now') - 7*86400) * 1000
WHERE created_at >= (strftime('%s', 'now', 'start of day')) * 1000
```

**Convert epoch ms to readable:**
```sql
datetime(created_at/1000, 'unixepoch') as created
```

## Introspection Commands

```sql
-- List all tables
.tables

-- Describe a table's columns
PRAGMA table_info(events);

-- Show CREATE statement
.schema events

-- Row counts per table (run one at a time)
SELECT 'events' as tbl, COUNT(*) as rows FROM events;
SELECT 'tasks' as tbl, COUNT(*) as rows FROM tasks;
SELECT 'memory_items' as tbl, COUNT(*) as rows FROM memory_items;
SELECT 'artifacts' as tbl, COUNT(*) as rows FROM artifacts;
SELECT 'agents' as tbl, COUNT(*) as rows FROM agents;
```

## Example Query Patterns

```sql
-- Events this week
SELECT COUNT(*) as count FROM events
WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-7 days');

-- Events by source (top 10)
SELECT source, COUNT(*) as cnt FROM events
GROUP BY source ORDER BY cnt DESC LIMIT 10;

-- Events today by hour
SELECT strftime('%H', created_at) as hour, COUNT(*) as cnt
FROM events WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', 'start of day')
GROUP BY hour ORDER BY hour;

-- Unread events by source
SELECT source, COUNT(*) as unread FROM events
WHERE read_at IS NULL AND archived_at IS NULL
GROUP BY source ORDER BY unread DESC LIMIT 20;

-- Memory acceptance rate
WITH total AS (SELECT COUNT(*) as cnt FROM memory_items)
SELECT status, COUNT(*) as cnt,
  ROUND(COUNT(*) * 100.0 / total.cnt, 1) as pct
FROM memory_items, total GROUP BY status;

-- Task status breakdown
SELECT status, COUNT(*) as cnt FROM tasks
GROUP BY status ORDER BY cnt DESC;

-- Recent artifacts
SELECT id, type, title, datetime(created_at/1000, 'unixepoch') as created
FROM artifacts ORDER BY created_at DESC LIMIT 10;

-- Cost summary (last 7 days)
SELECT ROUND(SUM(CAST(json_extract(cost, '$.estimatedCostUsd') AS REAL)), 4) as total_usd,
  COUNT(*) as tasks_with_cost
FROM tasks
WHERE cost IS NOT NULL AND cost != '{}'
  AND created_at >= (strftime('%s', 'now') - 7*86400) * 1000;

-- Dead letters (failed ingestion)
SELECT source, COUNT(*) as cnt, MAX(recorded_at) as latest
FROM dead_letters GROUP BY source;

-- Tool usage (top 10, last 30 days)
SELECT tool_name, COUNT(*) as calls,
  SUM(CASE WHEN status='error' THEN 1 ELSE 0 END) as errors
FROM tool_calls
WHERE started_at >= (strftime('%s', 'now') - 30*86400) * 1000
GROUP BY tool_name ORDER BY calls DESC LIMIT 10;

-- Automation rule execution stats (last 30 days)
SELECT r.name, COUNT(e.id) as runs,
  SUM(CASE WHEN e.status='error' THEN 1 ELSE 0 END) as errors,
  ROUND(SUM(e.cost_usd), 4) as total_cost
FROM automation_rules r
LEFT JOIN automation_rule_executions e ON e.rule_id = r.id
  AND e.fired_at >= (strftime('%s', 'now') - 30*86400) * 1000
GROUP BY r.id ORDER BY runs DESC LIMIT 20;

-- Agent activity
SELECT name, status,
  datetime(last_heartbeat_at/1000, 'unixepoch') as last_heartbeat
FROM agents ORDER BY last_heartbeat_at DESC;
```

## Safety Rules

1. **ALWAYS** use read-only URI: `file:...?mode=ro`
2. **ALWAYS** wrap in `timeout 5`
3. **NEVER** generate INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, ATTACH, DETACH, VACUUM, REINDEX, or ANALYZE
4. **NEVER** query sensitive tables: `sessions`, `webauthn_credentials`, `webauthn_challenges`, `shell_grants`, `assistant_session_messages`
5. **ALWAYS** add `LIMIT` clause (default 20) to avoid overwhelming output
6. If a query returns no results, check the timestamp format for that table
7. If the user provides raw SQL, validate it starts with SELECT, PRAGMA, WITH, or a safe dot-command (`.tables`, `.schema`, `.headers`, `.mode`). Reject anything else. **NEVER** allow `.shell`, `.system`, `.import`, `.output`, `.once`, `.log`, `.save`, `.restore`, `.dump` — these can execute commands or write files
8. **NEVER** use SQLite file I/O functions: `readfile()`, `writefile()`, `edit()`, `load_extension()`. Reject any query containing these

## Output Guidelines

- Use `.mode markdown` with `.headers on` for tabular results
- Use `.mode list` for single values
- After showing raw data, summarize results in natural language
- For large result sets, highlight key insights rather than dumping all rows
- When comparing time periods, show both absolute numbers and percentage change

