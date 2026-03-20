---
name: google-workspace
description: "Query and manage Google Workspace. Use when asked about emails, calendar, documents, spreadsheets, presentations, contacts, or tasks. Keywords: gmail, inbox, email, calendar, events, drive, files, sheets, spreadsheet, docs, document, slides, presentation, tasks, todo, contacts, people"
inclusion: always
context: "chat"
---

# Google Workspace via gws CLI

Use `cairn.shell` to run `gws` commands. Format:

```
gws <service> <resource> [subResource] <method> --params '{"key":"value"}' --format json
```

## Services

drive, gmail, calendar, sheets, docs, slides, tasks, people

## Read operations (auto-execute)

Methods: list, get, watch, export, batchGet, search

**Gmail:**
- List emails: `gws gmail users messages list --params '{"userId":"me","maxResults":10}' --format json`
- Get message: `gws gmail users messages get --params '{"userId":"me","id":"MSG_ID","format":"full"}' --format json`
- Get thread: `gws gmail users threads get --params '{"userId":"me","id":"THREAD_ID"}' --format json`
- Search: `gws gmail users messages list --params '{"userId":"me","q":"from:someone@example.com","maxResults":5}' --format json`

**Calendar:**
- List events: `gws calendar events list --params '{"calendarId":"primary","maxResults":10,"timeMin":"<ISO date for today>"}' --format json`
- Get event: `gws calendar events get --params '{"calendarId":"primary","eventId":"EVT_ID"}' --format json`

**Drive:**
- List files: `gws drive files list --params '{"pageSize":10}' --format json`
- Search: `gws drive files list --params '{"q":"name contains '\''report'\''","pageSize":10}' --format json`

**Sheets:**
- Get spreadsheet: `gws sheets spreadsheets get --params '{"spreadsheetId":"SHEET_ID"}' --format json`
- Get values: `gws sheets spreadsheets values get --params '{"spreadsheetId":"SHEET_ID","range":"Sheet1!A1:D10"}' --format json`

**Tasks:**
- List task lists: `gws tasks tasklists list --format json`
- List tasks: `gws tasks tasks list --params '{"tasklist":"TASKLIST_ID"}' --format json`

**Docs:**
- Get document: `gws docs documents get --params '{"documentId":"DOC_ID"}' --format json`

**Slides:**
- Get presentation: `gws slides presentations get --params '{"presentationId":"PRES_ID"}' --format json`

## Write operations (need approval via cairn.shell)

Methods: create, update, patch, delete, send, insert, move, trash, untrash, modify

Write commands use `--json` for the request body:

- Send email: `gws gmail users messages send --params '{"userId":"me"}' --json '{"raw":"BASE64_ENCODED"}' --format json`
- Create event: `gws calendar events insert --params '{"calendarId":"primary"}' --json '{"summary":"Meeting","start":{"dateTime":"..."},"end":{"dateTime":"..."}}' --format json`
- Create task: `gws tasks tasks insert --params '{"tasklist":"TASKLIST_ID"}' --json '{"title":"Task name"}' --format json`
- Update spreadsheet: `gws sheets spreadsheets values update --params '{"spreadsheetId":"ID","range":"Sheet1!A1","valueInputOption":"USER_ENTERED"}' --json '{"values":[["val1","val2"]]}' --format json`

## Notes

- params must be valid JSON objects
- Always add `--format json` for parseable output
- Gmail userId is always "me" for the authenticated user
- Calendar calendarId is "primary" for the default calendar
- For write operations, cairn.shell will request approval before executing
