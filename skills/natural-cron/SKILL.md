---
name: natural-cron
description: "Use when user asks to create a schedule, cron expression, recurring task, automation rule, run daily/weekly/monthly, set a timer, or mentions cron/cronjob/crontab. Converts natural language like 'every weekday at 9am' to valid cron expressions and optionally creates automation rules."
inclusion: on-demand
allowed-tools: "cairn.shell"
---


# Natural Language to Cron Expression

Convert natural language scheduling requests to valid 5-field cron expressions, validate them with `croner`, and optionally create automation rules via the Cairn API.

## Cron Format Reference

Standard 5-field format: `minute hour day-of-month month day-of-week`

| Field         | Range   | Special |
|---------------|---------|---------|
| Minute        | 0-59    | `*` `,` `-` `/` |
| Hour          | 0-23    | `*` `,` `-` `/` |
| Day of month  | 1-31    | `*` `,` `-` `/` |
| Month         | 1-12    | `*` `,` `-` `/` |
| Day of week   | 0-6     | `*` `,` `-` `/` (0=Sun, 1=Mon, ..., 6=Sat) |

## Common Patterns

| Natural Language | Cron Expression | Notes |
|---|---|---|
| Every minute | `* * * * *` | |
| Every 5 minutes | `*/5 * * * *` | |
| Every 15 minutes | `*/15 * * * *` | |
| Every 30 minutes | `*/30 * * * *` | |
| Every hour | `0 * * * *` | At minute 0 |
| Every 2 hours | `0 */2 * * *` | |
| Daily at midnight | `0 0 * * *` | UTC midnight |
| Daily at 6 AM UTC | `0 6 * * *` | |
| Daily at 9 AM Israel | `0 6 * * *` (IDT) / `0 7 * * *` (IST) | See timezone rules |
| Israel workdays at 9 AM | `0 6 * * 0-4` (IDT) / `0 7 * * 0-4` (IST) | Sun-Thu |
| Every Monday at 8 AM UTC | `0 8 * * 1` | |
| Weekends at noon UTC | `0 12 * * 5,6` | Fri-Sat (Israel weekend) |
| 1st of every month at 3 AM UTC | `0 3 1 * *` | |
| Every quarter (Jan/Apr/Jul/Oct 1st) | `0 0 1 1,4,7,10 *` | |
| Twice a day (8 AM and 8 PM UTC) | `0 8,20 * * *` | |
| Every weekday at 7:30 AM UTC | `30 7 * * 1-5` | Mon-Fri (international) |
| Last workday of month at 5 PM UTC | Not expressible in cron | Use nearest: `0 17 28-31 * 0-4` |

## Timezone Rules

**All cron expressions are evaluated in UTC** (hardcoded in the automation engine).

Avi is in Israel. Convert local Israel times to UTC before writing the cron expression:

| Season | Israel TZ | UTC Offset | Conversion |
|--------|-----------|------------|------------|
| Summer (late March - late October) | IDT | UTC+3 | Subtract 3 hours |
| Winter (late October - late March) | IST | UTC+2 | Subtract 2 hours |

**Examples:**

| User says | Current season | UTC hour | Cron |
|-----------|---------------|----------|------|
| "9 AM" | Summer (IDT) | 06:00 | `0 6 ...` |
| "9 AM" | Winter (IST) | 07:00 | `0 7 ...` |
| "6 PM" | Summer (IDT) | 15:00 | `0 15 ...` |
| "midnight" | Summer (IDT) | 21:00 (prev day) | `0 21 ...` |

**Israel workweek**: Sunday through Thursday (day-of-week 0-4).
- "workdays" or "weekdays" for Avi = `0-4` (Sun-Thu), NOT `1-5` (Mon-Fri)
- "weekend" for Avi = `5,6` (Fri-Sat)

**Always confirm the UTC conversion with the user.** Show the conversion and cron expression. Once confirmed, you can create the rule (which requires approval via cairn.shell's write policy).

## Validation

Before creating a rule, validate the expression using `croner` and show the next 5 run times:

Use `cairn.shell` with `cwd: "~/cairn-backend"` to run:

```bash
node -e "
import('croner').then(({Cron}) => {
  const expr = process.argv[2];
  try {
    const c = new Cron(expr, { timezone: 'UTC' });
    const runs = c.nextRuns(5);
    console.log('Valid. Next 5 runs (UTC):');
    runs.forEach(d => console.log('  ' + d.toISOString()));
  } catch(e) {
    console.error('Invalid:', e.message);
    process.exit(1);
  }
});
" -- "EXPR"
```

Replace `EXPR` with the cron expression to test. The expression is passed as a command-line argument (after `--`, at `process.argv[2]`) to prevent injection attacks. Set `cwd: "~/cairn-backend"` in the `cairn.shell` call so `croner` is importable without a `cd` that triggers write-approval prompts. Uses `import()` for ESM compatibility (the backend uses `"type": "module"`). The `timezone: 'UTC'` option matches the automation engine's runtime behavior.

Show the user the next 5 run times and ask them to confirm the schedule looks correct before creating any rule.

## Creating an Automation Rule

Once the expression is validated and the user confirms, create an automation rule.

**Important:** `cairn.shell` does not forward `WRITE_API_TOKEN` to subprocesses (only safe env vars like PATH, HOME are forwarded). Use a node script that reads the token from the backend `.env` file via `cwd: "~/cairn-backend"`:

```bash
node -e "
import('fs').then(({readFileSync}) => {
  const env = readFileSync('.env', 'utf8');
  const match = env.match(/^WRITE_API_TOKEN=(.+)$/m);
  if (!match) { console.error('WRITE_API_TOKEN not found in .env'); process.exit(1); }
  const token = match[1].trim();
  const body = JSON.parse(process.argv[2]);
  fetch('http://localhost:8788/v1/automation-rules', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
    body: JSON.stringify(body),
  }).then(r => r.json()).then(d => console.log(JSON.stringify(d, null, 2)))
    .catch(e => { console.error(e.message); process.exit(1); });
});
" -- '{"name":"RULE_NAME","description":"DESCRIPTION","enabled":false,"trigger":{"type":"schedule","cronLike":"CRON_EXPR"},"taskType":"TASK_TYPE","taskPriority":"normal"}'
```

The JSON payload is passed as a single `process.argv[2]` argument and parsed with `JSON.parse`, avoiding shell interpolation of user-supplied values. The token is read from the `.env` file at runtime — never embedded in the command string — so it won't appear in approval/audit logs.

**Required fields:**
- `name` — short descriptive name (max 200 chars)
- `trigger.cronLike` — the validated cron expression
- `taskType` — one of the valid types listed below

**Valid `taskType` values** (see `backend/src/schemas.ts` TaskTypeSchema for current list):
`assistant_chat`, `draft_email`, `draft_post`, `generate_deck`, `inbox_triage`, `plan_trip`, `summarize_feed`, `summarize_pr`, `weekly_plan`, `cleanup_chore`, `agent_run`, `code_task`

**Optional fields with defaults:**
- `description` — longer description (default: empty, max 2000 chars)
- `enabled` — **always set to `false`** (user enables after review)
- `taskPriority` — `low`, `normal`, `high`, `critical` (default: `normal`)
- `cooldownMs` — minimum ms between runs (default: 3600000 = 1 hour)
- `budgetCapUsd` — max spend per budget window (default: 1.00)
- `budgetWindowMs` — budget window duration (default: 86400000 = 24 hours)
- `taskInputRefsTemplate` — JSON object passed as input to the task (default: `{}`)

**Security:** Token is read from the backend `.env` file at runtime — never hardcoded or passed in the command string.

## Workflow

1. **Parse** the user's natural language request
2. **Convert** to a 5-field cron expression, applying Israel timezone offset
3. **Validate** with croner and show next 5 runs (UTC)
4. **Confirm** with the user that the schedule is correct
5. **Create** the automation rule (disabled by default)
6. **Tell** the user the rule ID and remind them to enable it when ready

## Cron Run Guard

When a cron-triggered automation rule fires, the resulting task should execute in a **focused context**:
- Execute the task directly — no greetings, no chitchat, no "let me help you with that"
- Output only the result (e.g., the digest, the scan report, the status check)
- If the task fails, log the error concisely — do not troubleshoot interactively
- Cron context = silent, focused execution

## Notes

- **DST transitions**: Israel switches to IDT (UTC+3) on the last Friday of March and back to IST (UTC+2) on the last Sunday of October each year. Schedules that target a specific local time will drift by 1 hour across DST boundaries. Example: A rule set to run at 9 AM Israel time will run at 06:00 UTC in summer and 07:00 UTC in winter. Warn the user to review their rules after each transition.
- **Cooldown**: Default 1 hour (3600000ms). For rules that run more frequently than hourly, the user should lower `cooldownMs` accordingly.
- **Budget**: Default $1.00 per 24-hour window. Adjust `budgetCapUsd` and `budgetWindowMs` for expensive task types.
- **Safety**: Rules are always created with `enabled: false`. The user must explicitly enable them after verifying the schedule.
- **Legacy values**: The API also accepts `"daily"`, `"weekly_monday"`, `"monthly_first"` as `cronLike` for backward compatibility, but prefer explicit cron expressions.

