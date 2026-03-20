---
name: email-triage
description: "Use when user asks to check email, triage inbox, summarize emails, or clean up mail. Keywords: email, inbox, gmail, triage, unread, clean"
inclusion: on-demand
allowed-tools: "cairn.readFeed,cairn.archiveFeedItem,cairn.deleteFeedItem,cairn.gwsQuery,cairn.markRead"
---

# Email Triage

## Check Inbox
1. Read recent emails: `cairn.readFeed` with `source=gmail`
2. Summarize: group by sender/topic, highlight important ones
3. Note: GitHub notification emails are auto-archived (in DB but not in feed)

## Triage Actions
- **Archive**: `cairn.archiveFeedItem` — hides from feed but stays in DB
- **Delete**: `cairn.deleteFeedItem` — permanent removal
- **Mark read**: `cairn.markRead` — mark individual items as read

## Gmail Search (deeper)
- Use `cairn.gwsQuery` with service=gmail, resource=users, subResource=messages, method=list
- Params: `{"userId":"me","q":"from:someone@example.com","maxResults":10}`

## Priority
1. Emails from real people (not automated)
2. Emails requiring action/reply
3. Newsletters and updates (archive candidates)
4. GitHub notifications (already auto-archived)
