---
name: skill-vetter
description: "Assess trust and safety of external SKILL.md files before importing into Cairn. Use when asked to vet, review, import, or check an external skill. Keywords: vet, vetting, trust, safety, review skill, check skill, import skill, external skill, security review"
argument-hint: "<path-to-SKILL.md> [--repo owner/name]"
allowed-tools: "cairn.shell"
inclusion: on-demand
---

# Skill Vetter

Security assessment of external SKILL.md files before importing into Cairn's skill system.

## Steps

### 1. Resolve input

Determine the target file from `$ARGUMENTS`:

- **Local path**: use directly
- **GitHub URL**: extract owner/repo, clone or fetch the SKILL.md
  ```bash
  cairn.shell: gh api repos/OWNER/REPO/contents/PATH --jq '.content' | base64 -d > /tmp/vetting-skill.md
  ```
- **Raw URL**: fetch with curl
  ```bash
  cairn.shell: curl -sL --max-time 15 --max-filesize 524288 'URL' > /tmp/vetting-skill.md
  ```

Set `FILE` to the resolved path. If `--repo owner/name` is provided, save it for step 5.

### 2. Parse frontmatter

Read the file and validate structure:

```bash
cairn.shell: head -50 "$FILE"
cairn.shell: wc -l "$FILE"
cairn.shell: sha256sum "$FILE"
```

Check each frontmatter field:

| Field | Requirement |
|-------|-------------|
| `name` | Present, lowercase, max 64 chars. If inside a skill directory, should match directory name (warning only for temp/standalone files) |
| `description` | Present, includes trigger phrase ("Use when...") |
| `disable-model-invocation` | Set to `true` for side-effect skills (deploy, send, delete, write, execute) |
| `allowed-tools` | Present and scoped (flag bare `Bash` without `cairn.` prefix) |
| `inclusion` | `on-demand` unless explicitly justified as core reference |
| body length | Under 500 lines |

### 3. Security scan

Check file size before processing:

```bash
cairn.shell: wc -c < "$FILE" | awk '{if ($1 > 1048576) print "WARNING: file exceeds 1MB"; else print "OK:" $1 "bytes"}'
```

Run consolidated checks against the file and record findings:

**Blockers** (any match = UNTRUSTED) — single combined grep:

```bash
cairn.shell: grep -nEi \
  -e '(Bearer\s+[A-Za-z0-9._~+/\-]{20,}|gh[ps]_[A-Za-z0-9_]{36}|github_pat_[A-Za-z0-9_]{82}|ghu_[A-Za-z0-9_]{36}|ghr_[A-Za-z0-9_]{36}|-----BEGIN.*PRIVATE KEY)' \
  -e '(rm\s+-rf\s+/|chmod\s+777|mkfs|fdisk|dd\s+if=|shutdown|reboot|halt|systemctl\s+(stop|disable))' \
  -e '(curl\s+.*-X\s*POST|curl\s+.*--data|curl\s+.*-d\s|wget\s+.*\|\s*(ba)?sh|curl.*\|\s*(ba)?sh)' \
  -e '(nc\s+-e|ncat|socat.*EXEC)' \
  -e '(base64\s+-d\s*\|\s*(ba)?sh|eval\s+\$\(|eval\s+`)' \
  "$FILE" || echo "(no blocker matches)"
```

For `curl POST` matches: check if the target is localhost:8788/v1/* (Pub API) — those are OK. External POSTs to unknown hosts are blockers.

**Warnings** (flagged for review) — single combined grep:

```bash
cairn.shell: grep -nEi \
  -e '(\$\{?[A-Z_]*(TOKEN|SECRET|API_KEY|PASSWORD|CREDENTIAL|AUTH|PRIVATE_KEY)[A-Z_]*\}?|process\.env\.|os\.environ)' \
  -e '((>|>>)\s*|tee\s+)(/|\.\./|~|\$HOME)' \
  -e '(pip\s+install|npm\s+install|cargo\s+install|apt\s+install|apt-get\s+install)' \
  -e '[0-9a-f]{64,}' \
  "$FILE" || echo "(no warning matches)"
```

Also flag:
- `allowed-tools` not scoped (bare `Bash`, `cairn.startCoding`, `cairn.deploy`, or no restriction)
- `inclusion: always` on a non-core skill
- `curl` to external APIs (check if expected and documented in the skill body)

### 4. Dependency check

Scan the skill body for command references and verify availability:

```bash
# Extract command names (handles cairn.shell: prefix and bare commands)
cairn.shell: grep -oE '\b(python3|node|npm|pip|cargo|go|docker|curl|jq|gh|gws|sqlite3|openssl)\b' "$FILE" | sort -u
```

For each binary found, batch-check availability:

```bash
cairn.shell: for cmd in $(grep -oE '\b(python3|node|npm|pip|cargo|go|docker|curl|jq|gh|gws|sqlite3|openssl)\b' "$FILE" | sort -u); do printf '%s: ' "$cmd"; which "$cmd" 2>/dev/null && echo "found" || echo "MISSING"; done
```

Flag any MISSING dependencies.

### 5. Repository check (if --repo provided)

Query the source repository for trust signals:

```bash
cairn.shell: gh repo view OWNER/REPO --json name,description,stargazerCount,createdAt,pushedAt,isArchived

cairn.shell: gh api repos/OWNER/REPO/contributors --jq '.[0:5] | .[] | .login'
```

Flag:
- Archived repository
- Fewer than 10 stars
- No commits in 6+ months (compare `pushedAt` to today)
- Single contributor with no other public repos

Space `gh` calls 2s apart.

### 6. Classify

Apply the scoring rubric based on scan results:

| Rating | Criteria |
|--------|----------|
| **VERIFIED** | Registry source + clean scan + active repo (>50 stars, recent commits, multiple contributors) |
| **TRUSTED** | Known source + clean scan + no warnings |
| **UNKNOWN** | Clean scan but no repo info or unverifiable source |
| **SUSPICIOUS** | Has warnings but no blockers — requires human review |
| **UNTRUSTED** | Has any blocker pattern — DO NOT IMPORT |

### 7. Report

Present findings as a structured report:

```markdown
## Skill Vetting Report

**File:** <path>
**SHA-256:** <hash>
**Lines:** <count>
**Rating:** VERIFIED | TRUSTED | UNKNOWN | SUSPICIOUS | UNTRUSTED

### Frontmatter
- name: <value> [OK | ISSUE: ...]
- description: <value> [OK | MISSING TRIGGER]
- allowed-tools: <value> [OK | WARNING: too broad]
- inclusion: <value> [OK | WARNING: always on untrusted]
- disable-model-invocation: <value> [OK | WARNING: missing for side-effect skill]

### Security Scan — Blockers
- Secrets/tokens: <count> findings
- Destructive commands: <count> findings
- Data exfiltration (curl POST, nc -e, socat): <count> findings
- Encoded payloads / command substitution: <count> findings

### Security Scan — Warnings
- Env var token reads: <count> findings
- External file writes: <count> findings
- Runtime dependency installs: <count> findings

### Dependencies
- <binary>: found | MISSING

### Repository (if checked)
- Stars: <n> | Created: <date> | Last push: <date>
- Contributors: <list>
- Archived: yes/no

### Recommendation
<IMPORT | REVIEW_REQUIRED | REJECT> — <reasoning>
```

### 8. Follow-up

Offer the appropriate next action:

- **VERIFIED / TRUSTED**: "Copy to `backend/.pub/skills/<name>/SKILL.md`?"
- **SUSPICIOUS**: "Review flagged items above. Override to TRUSTED?"
- **UNTRUSTED**: "Rejected. Do not import this skill."

After import, remind: run `/enhance --focus=skills` to validate the imported skill.

## Rules

- This skill does not modify the target repo or skill files — it may write temp files to /tmp for analysis. Never import without user confirmation
- Always show the full report before offering to import
- If multiple SKILL.md files are found in a repo, list them and ask which to vet
- Space `gh api` calls 2s apart to avoid rate limiting
