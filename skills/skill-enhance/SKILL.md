---
name: skill-enhance
description: "Use when asked to validate a skill, check a SKILL.md, enhance a skill, lint skills, review skill quality, or fix skill issues. Runs structural and quality checks on Cairn skill files. Keywords: enhance skill, validate skill, check skill, lint skill, review skill, skill quality, fix skill, skill issues, SKILL.md check"
allowed-tools: "cairn.shell"
inclusion: on-demand
---

# Skill Enhance — Validate & Improve Cairn Skills

Run structural, quality, and security checks on SKILL.md files. Report issues by severity and suggest fixes.

## Arguments

If a path is provided, check that single skill. Otherwise, scan all skills in `skills/`.

## Step 1: Find skill files

If a specific path was given:
```bash
ls <path>
```

Otherwise scan all:
```bash
find backend/.pub/skills -name "SKILL.md" -type f | sort
```

## Step 2: For each SKILL.md, run all checks

Read the file content and evaluate each check below. Collect findings as a list of `{severity, check, message, suggestion}`.

### Frontmatter Checks

| Check | Severity | Rule |
|-------|----------|------|
| `name` present | HIGH | Must exist and be lowercase+hyphens, max 64 chars |
| `name` matches directory | HIGH | `name` field must equal the parent directory name |
| `description` present | HIGH | Must exist and be non-empty |
| `description` has trigger phrase | HIGH | Must contain "Use when" or "Trigger when" |
| `description` has keywords | MEDIUM | Should end with "Keywords: ..." list |
| `description` length | MEDIUM | Should be 50-1024 chars. Too short = poor triggering |
| `allowed-tools` present | HIGH | Must be set — unscoped tools are a security risk |
| `allowed-tools` no bare Bash | HIGH | Must not contain "Bash" — use "cairn.shell" instead |
| `inclusion` value | LOW | Must be "on-demand" or "always". Default to on-demand |
| `inclusion: always` justified | MEDIUM | Only core infra skills should be always-included |
| `disable-model-invocation` check | HIGH | Must be `true` ONLY if skill sends email, deletes email, or pushes to main. Must NOT be set for local-only skills |

### Body Checks

| Check | Severity | Rule |
|-------|----------|------|
| Line count | MEDIUM | Body should be under 500 lines. Flag if over |
| Has steps/structure | LOW | Should have `##` headings for structure |
| No hardcoded secrets | HIGH | Scan for patterns: API keys, tokens, passwords, `Bearer`, `sk-`, `ghp_`, `gho_`, connection strings |
| No TODO/FIXME | LOW | Unfinished work should be resolved before shipping |
| Output format defined | LOW | Skills that produce output should document the format |

### Security Checks

| Check | Severity | Rule |
|-------|----------|------|
| Secret patterns | HIGH | Regex scan for: `/(?:sk-|ghp_|gho_|AIza|AKIA|Bearer\s+[A-Za-z0-9])/` |
| PEM blocks | HIGH | Check for `-----BEGIN` patterns |
| Connection strings | MEDIUM | Check for `://.*:.*@` patterns |
| URL with credentials | MEDIUM | Check for embedded auth in URLs |

## Step 3: Run the checks

Use `cairn.shell` to read each file and apply checks. Here's how to scan for secrets:

```bash
grep -nE '(sk-[a-zA-Z0-9]{20}|ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|AIza[a-zA-Z0-9_-]{35}|AKIA[A-Z0-9]{16}|-----BEGIN|Bearer\s+[A-Za-z0-9]{10})' skills/*/SKILL.md
```

For frontmatter parsing, extract the YAML block between `---` markers:
```bash
sed -n '/^---$/,/^---$/p' <path-to-SKILL.md> | head -20
```

For line count:
```bash
wc -l skills/*/SKILL.md | sort -rn
```

## Step 4: Report findings

Group by severity and present as a table:

```
## Skill Enhance Report

### HIGH Issues (must fix)
| Skill | Check | Issue | Fix |
|-------|-------|-------|-----|
| my-skill | missing-tools | `allowed-tools` not set | Add `allowed-tools: "cairn.shell"` |

### MEDIUM Issues (should fix)
| Skill | Check | Issue | Fix |
|-------|-------|-------|-----|
| my-skill | no-keywords | Description missing keyword list | Add `Keywords: x, y, z` at end |

### LOW Issues (nice to fix)
| Skill | Check | Issue | Fix |
|-------|-------|-------|-----|
| my-skill | no-headings | Body has no ## structure | Add section headings |

### Summary
- Skills checked: N
- HIGH: N | MEDIUM: N | LOW: N
- Clean: N skills passed all checks
```

## Step 5: Offer to fix

If issues were found, offer to fix them automatically:

- **HIGH issues**: Fix immediately — these are blockers
- **MEDIUM issues**: Fix and explain what changed
- **LOW issues**: Mention but don't force — ask if user wants them fixed

For each fix, use `cairn.shell` to edit the file with `sed` or write a corrected version.

## Severity Definitions

- **HIGH**: Skill will malfunction, trigger incorrectly, or has a security vulnerability. Must fix before use.
- **MEDIUM**: Skill works but has quality or maintainability issues. Should fix.
- **LOW**: Polish and best-practice suggestions. Nice to have.
