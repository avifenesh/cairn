---
name: meeting-to-tasks
description: "Convert meeting notes to action items. Use when asked to extract tasks, action items, follow-ups, or decisions from meeting notes, transcripts, or calendar events. Keywords: meeting, action items, tasks, follow-up, decisions, minutes, transcript, notes, extract tasks, meeting notes"
argument-hint: "<paste meeting notes, provide file path, or specify calendar event>"
allowed-tools: "cairn.shell"
inclusion: on-demand
---

# Meeting to Tasks

Parse meeting notes, transcripts, or calendar event details and extract structured action items. Save as an Obsidian-flavored markdown file in the vault with task checkboxes, callouts for blockers, and frontmatter properties.

Adapted from OpenClaw `codedao12/meeting-to-action` concept. Output follows the `obsidian` skill conventions.

## Step 1: Detect Input Type

Determine the content source from `$ARGUMENTS` or conversation context:

| Input Pattern | Type | Extraction Method |
|--------------|------|-------------------|
| User pasted text in conversation | Plain text | Use directly |
| File path (`.txt`, `.md`) | File | Read via `cairn.shell` |
| Calendar reference ("today's standup", "the 2pm meeting") | Calendar event | Fetch via `cairn.shell` |
| Google Doc URL (`docs.google.com/document/d/...`) | Google Doc | Fetch via `cairn.shell` |

If the input is ambiguous, ask the user to clarify. If no input is provided, offer to check today's calendar for recent meetings.

## Step 2: Extract Content

### Plain Text

Use the content directly from the conversation — no extraction needed.

### File

Read the file via shell:

```bash
cat 'FILE_PATH' | head -1000
```

**Shell safety**: Insert actual file paths inside single quotes. If the path contains single quotes, escape with `'\''`.

### Calendar Event

Find matching events using `cairn.shell` (read operations auto-execute, no approval needed):

```json
cairn.shell({
  "service": "calendar",
  "resource": "events",
  "method": "list",
  "params": {
    "calendarId": "primary",
    "maxResults": 10,
    "timeMin": "<start-of-day ISO>",
    "timeMax": "<end-of-day ISO>",
    "singleEvents": true,
    "orderBy": "startTime",
    "q": "SEARCH_TERM"
  }
})
```

Replace `SEARCH_TERM` with the meeting name the user mentioned. Replace date placeholders with actual ISO timestamps.

Once you find the event, check its `description` field for inline notes. If the event has an attached Google Doc link (common for meeting notes), extract the document ID and fetch it.

### Google Doc

**URL validation**: Verify the URL matches `https://docs.google.com/document/d/...` before proceeding. Reject URLs targeting other domains, private IPs, or non-HTTPS protocols.

Extract the document ID from the URL — the segment between `/d/` and the next `/` (e.g. `/edit`, `/view`, `/copy`) or end of path. For example, from `docs.google.com/document/d/ABC123/edit` or `docs.google.com/document/d/ABC123/view`, the ID is `ABC123`. Then fetch:

```json
cairn.shell({
  "service": "docs",
  "resource": "documents",
  "method": "get",
  "params": { "documentId": "DOC_ID" }
})
```

Parse the document body content from the returned JSON. The content is in the `body.content` array — extract text from `paragraph.elements[].textRun.content` fields.

## Step 3: Parse Action Items

Read the extracted meeting content and identify action items. For each item, extract:

| Field | Description | Example |
|-------|-------------|---------|
| **text** | What needs to be done | "Set up staging environment for demo" |
| **owner** | Person responsible (if mentioned) | "@alice" |
| **due** | Deadline (if mentioned) | "2026-03-20" |
| **priority** | Urgency level | high, medium, low |
| **category** | Type of item | Decision, Action Item, Follow-up, Blocker |

### Extraction Rules

- **Action items**: Look for phrases like "will do", "needs to", "action:", "TODO", "take away", "follow up on", "assigned to", imperative verbs
- **Decisions**: Look for "decided", "agreed", "we'll go with", "consensus", "approved"
- **Blockers**: Look for "blocked by", "waiting on", "dependency", "can't proceed until"
- **Follow-ups**: Look for "check back", "revisit", "schedule", "next steps", "circle back"
- **Owners**: Look for @-mentions, "Alice will", "[name] to", "assigned to [name]"
- **Dates**: Look for "by Friday", "next week", "March 20", "EOD", "end of sprint". Convert relative dates to absolute (YYYY-MM-DD) based on today's date
- **Priority**: Default to medium. Mark as high if: words like "urgent", "critical", "ASAP", "blocker", or deadline is within 2 days. Mark as low if: "nice to have", "eventually", "when possible"

### Formatting

Format each item's `text` field as:

- With owner and date: `"Description — @owner, due YYYY-MM-DD"`
- With owner only: `"Description — @owner"`
- With date only: `"Description — due YYYY-MM-DD"`
- Neither: `"Description"`

## Step 4: Present and Save

### 4a: Present Inline

Before saving, present the extracted items to the user grouped by category:

```markdown
## Extracted Action Items — [Meeting Name]

### Decision (N items)
- [high] Decided to use PostgreSQL for the new service — @alice
- Approved Q2 budget increase — @bob

### Action Item (N items)
- [high] Set up staging environment — @charlie, due 2026-03-20
- Write API documentation — @diana, due 2026-03-25

### Follow-up (N items)
- Check vendor pricing next week — @eve

### Blocker (N items)
- Waiting on security review before deploy — @frank
```

Ask: "Save these to your Obsidian vault? I can adjust any items first."

### 4b: Save to Obsidian Vault

After the user confirms, write an Obsidian-flavored markdown file using `cairn.shell`:

```bash
VAULT="${OBSIDIAN_VAULT_PATH:-$HOME/obsidian-vault}"
FILENAME="YYYY-MM-DD-meeting-slug.md"
mkdir -p "$VAULT/meetings"
cat > "$VAULT/meetings/$FILENAME.tmp" << 'ENDOFFILE'
---
title: "Meeting Name"
date: YYYY-MM-DD
type: meeting
source: google-doc
tags: [pub, meeting, action-items]
---

# Meeting Name — Date

**Attendees:** Alice, Bob, Charlie

## Decision
- [x] Decided to use PostgreSQL for analytics service
- [x] Approved new CI pipeline changes

## Action Item
- [ ] Deploy auth refactor to staging — @alice, due 2026-03-20 #urgent
- [ ] Review PR #42 — @bob, due 2026-03-20
- [ ] Write API documentation — @diana, due 2026-03-25

## Follow-up
- [ ] Check vendor pricing next week — @eve

## Blocker
> [!warning] Database migration blocked
> Waiting on DevOps to provision new RDS instance — @bob
ENDOFFILE
mv "$VAULT/meetings/$FILENAME.tmp" "$VAULT/meetings/$FILENAME"
```

**Format rules:**
- Decisions use `- [x]` (already decided, checked off)
- Action items and follow-ups use `- [ ]` (open tasks with checkboxes)
- Blockers use `> [!warning]` callouts (visually distinct in Obsidian)
- High priority items get `#urgent` tag
- Owners prefixed with `@`, dates as `due YYYY-MM-DD`
- Generate slug from meeting name: lowercase, hyphens, max 50 chars
- Atomic write: `.tmp` then `mv`

## Step 5: Optional Follow-up Actions

After saving to the vault, offer these optional actions:

1. **Create calendar reminders** for items with due dates via `cairn.shell` (auto-executes for calendar writes). Note: Google Calendar all-day events use an exclusive end date — set `end.date` to the day after `start.date`:
   ```json
   cairn.shell({
     "service": "calendar",
     "resource": "events",
     "method": "insert",
     "params": { "calendarId": "primary" },
     "body": {
       "summary": "[ACTION] Description",
       "start": { "date": "YYYY-MM-DD" },
       "end": { "date": "NEXT_DAY_YYYY-MM-DD" },
       "reminders": { "useDefault": false, "overrides": [{ "method": "popup", "minutes": 1440 }] }
     }
   })
   ```

2. **Send summary to attendees** (if the meeting had participants): compose an email draft via `cairn.shell`. Note: gmail send/delete requires approval, but draft creation auto-executes:
   ```json
   cairn.shell({
     "service": "gmail",
     "resource": "users",
     "method": "create",
     "subResource": "drafts",
     "params": { "userId": "me" },
     "body": { "message": { "raw": "BASE64_ENCODED_EMAIL" } }
   })
   ```

Only offer these if relevant — skip if the user just pasted ad-hoc notes with no calendar context.

## Notes

- If no action items are found, tell the user: "No clear action items found in these notes. Would you like me to create a general summary instead?" (delegate to `/summarize` skill)
- For very long transcripts (>1000 lines), process in 1000-line chunks: `head -1000 'FILE_PATH'`, then `sed -n '1001,2000p' 'FILE_PATH'`, then `sed -n '2001,3000p' 'FILE_PATH'`, and so on until no more output is returned. Extract items from each chunk, then deduplicate by matching item text (ignoring whitespace). If an item appears in multiple chunks, keep the version with more metadata (owner/date)
- Meeting notes in languages other than English: extract in the original language, ask if the user wants translation
- The output file is Obsidian-flavored markdown — task checkboxes are interactive in Obsidian's reading view
