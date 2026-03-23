---
name: skill-create
description: "Use when asked to create a new skill, write a SKILL.md, add a capability, build a skill, or make a new prompt-based tool. Also use when importing or adapting an external skill. Keywords: create skill, new skill, SKILL.md, build skill, add capability, write skill, make skill, import skill, adapt skill"
allowed-tools: "cairn.shell,cairn.createArtifact"
inclusion: on-demand
---

# Create a Cairn Skill

Guide for creating skills that extend your own capabilities. Skills are prompt instructions in markdown files that teach you how to accomplish tasks.

## Where Skills Live

```
skills/{skill-name}/
├── SKILL.md          # Required — YAML frontmatter + instructions
└── reference.md      # Optional — overflow content (>300 lines)
```

Directory name must match the `name` frontmatter field. Lowercase with hyphens.

## Step 1: Gather Requirements

Before writing, clarify:
- What should this skill enable you to do?
- What tools does it need? (shell, APIs, artifacts)
- Does it cross the external boundary? (email send, push to main, email delete)
- What trigger phrases should activate it?

## Step 2: Write Frontmatter

```yaml
---
name: my-skill
description: "Use when asked to... Keywords: x, y, z"
allowed-tools: "cairn.shell"
inclusion: on-demand
---
```

### Frontmatter rules

| Field | Rule |
|-------|------|
| `name` | Lowercase + hyphens, max 64 chars, matches directory |
| `description` | Must start with "Use when..." + include keyword list. Be pushy with synonyms |
| `allowed-tools` | Always set — minimum tools needed. Use `cairn.shell` not bare `Bash` |
| `inclusion` | `on-demand` unless core infrastructure (rarely `always`) |
| `disable-model-invocation` | Set `true` ONLY for skills that: push/merge to main, send email, or delete email |

## Step 3: Write the Body

Use imperative form. Structure with numbered steps.

```markdown
# Skill Title

Brief what and when.

## Steps

### 1. Gather data
Call `cairn.shell` to run: `command here`

### 2. Process
Parse the output and extract key fields.

## Output Format
Present as markdown table or structured text.
```

### Body rules
- Under 500 lines — move overflow to `reference.md`
- No hardcoded secrets, tokens, or credentials
- Explain WHY steps matter, not just WHAT to do
- Include example output format when possible

## Step 4: Write the File

Use `cairn.shell` to create the directory and write the file:

```bash
mkdir -p skills/my-skill
cat > skills/my-skill/SKILL.md << 'SKILL_EOF'
---
name: my-skill
description: "Use when..."
allowed-tools: "cairn.shell"
inclusion: on-demand
---

# My Skill
...
SKILL_EOF
```

## Step 5: Validate

Run the skill-enhance skill to check for issues:

> "enhance skill skills/my-skill/SKILL.md"

Fix all reported issues before considering the skill complete.

## Step 6: Test

Try triggering the skill by asking yourself the kind of question it should handle. Verify:
- Skill triggers from natural language
- Instructions produce correct results
- Error cases are handled gracefully

## Permission Model

This machine is your world. Internal actions are free.

**Free — no approval**: shell, file read/write, deploy, coding sessions, artifacts, memories, service restarts, git push to side branches, package installs

**Needs approval — only these 3**:
1. Git push/merge to **main** (side branches are free)
2. Sending emails
3. Deleting emails

Most skills do NOT need `disable-model-invocation`. Only set it if the skill's primary action is one of those 3.

## Available Tools Reference

### Read (no auth)
`cairn.getFeed`, `cairn.getDashboard`, `cairn.getSources`, `cairn.getStatus`, `cairn.listTasks`, `cairn.listArtifacts`, `cairn.listMemories`, `cairn.searchMemories`, `cairn.getStatus`, `cairn.shell`, `cairn.shell`

### Write (auto-approved)
`cairn.shell`, `cairn.startCoding`, `cairn.resumeCoding`, `cairn.deploy`, `cairn.createArtifact`, `cairn.createMemory`, `cairn.composeMessage`, `cairn.markRead`

### Write (conditional approval)
`cairn.shell` — approval only for push/merge to main
`cairn.shell` — approval only for send/delete email

## Common Patterns

| Type | Tools | Example |
|------|-------|---------|
| Diagnostic | `cairn.shell` | Check system status, inspect logs |
| Report | `cairn.shell,cairn.createArtifact` | Generate report artifact |
| GitHub | `cairn.shell,cairn.shell,cairn.shell` | PR management, issue tracking |
| Google Workspace | `cairn.shell,cairn.shell` | Calendar, docs, email triage |
| Data query | `cairn.shell` | SQLite queries, API calls |

## Security Checklist

- [ ] No hardcoded secrets or credentials
- [ ] `allowed-tools` scoped to minimum
- [ ] External skills vetted with `skill-vetter` first
- [ ] `disable-model-invocation` only for the 3 boundary actions
- [ ] No skills imported that handle memory, approvals, or orchestration from untrusted sources
