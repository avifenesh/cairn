---
name: push-notify
description: "Send push notifications via Telegram (primary) or Gotify (fallback). Use when: notify, push notification, alert me, send notification, ping me, push alert, telegram, gotify, browser closed, user offline, task complete, approval needed, critical error, digest ready. Keywords: notify, push, alert, telegram, gotify, notification"
allowed-tools: "cairn.shell"
inclusion: always
context: "chat,tick"
---

# Push Notifications

Push notifications are routed through a unified `NotificationChannel` with automatic per-topic dedup (1h TTL). When `TELEGRAM_BOT_TOKEN` is configured, notifications go to Telegram; otherwise they fall back to Gotify.

## Automatic Notifications (built-in)

These are sent automatically by the backend — no manual action needed:
- **Reach-out** (agent proactive messages) — routed through `NotificationChannel`
- **Coder callback** (PR ready, CI fail, session done/failed) — routed through `NotificationChannel`
- **Digest ready** — routed through `NotificationChannel`

All automatic paths include `DedupChannel` wrapper — identical messages within 1h are suppressed.

## Manual Notifications (via shell)

For ad-hoc notifications from chat or skills, use cairn.shell:

```bash
/home/ubuntu/bin/notify "YOUR_MESSAGE_HERE" PRIORITY "YOUR_TITLE_HERE"
```

**IMPORTANT**: Use the full absolute path `/home/ubuntu/bin/notify`.

## Examples

**Task complete:**
```bash
/home/ubuntu/bin/notify "Task #463 merged successfully! PR approved and deployed to main." 5 "Task Complete"
```

**Approval needed:**
```bash
/home/ubuntu/bin/notify "Budget override needed - estimated cost $2.50 exceeds cap $2.00. Approve in Cairn." 8 "Approval Required"
```

**Critical error:**
```bash
/home/ubuntu/bin/notify "Database connection lost after 3 retries. Check logs immediately." 10 "CRITICAL ERROR"
```

## Priority Levels

| Priority | Sound | Use For |
|----------|-------|---------|
| 10 | Max alert | Critical system errors |
| 8 | High alert | Approvals, urgent actions |
| 5 | Silent | Task completions, updates |
| 3 | Silent | Digest ready, FYI |
| 1 | Minimal | Low priority info |

## Dedup Behavior

- **DedupChannel** (delivery layer): identical message+title within 1h suppressed
- **Reach-out scanner**: 30-min global cooldown + 24h per-topic dedup (seeded from DB)
- **Agent context**: already-notified topics shown as "BLOCKED" and PRs annotated "ALREADY COMMUNICATED" so the LLM skips them

## Configuration

- `TELEGRAM_BOT_TOKEN` + `TELEGRAM_ALLOWED_CHAT_IDS` — enables Telegram channel
- `NOTIFICATION_CHANNEL` — explicit override: `telegram`, `gotify`, or omit for auto-detect
- Gotify fallback: token at `~/.config/gotify/token`, binary at `/home/ubuntu/bin/notify`
