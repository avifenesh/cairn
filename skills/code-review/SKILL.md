---
name: code-review
description: "Use when user asks to review code, check for bugs, review a PR, audit code quality, or find security issues. Keywords: review, audit, check, bugs, security, style, PR"
inclusion: on-demand
allowed-tools: "cairn.readFile,cairn.listFiles,cairn.searchFiles,cairn.gitRun"
---

# Code Review

Systematic code review workflow:

1. **Scope** — Identify files to review. Use `cairn.gitRun` with `diff --name-only` to find changed files, or `cairn.listFiles` to explore.
2. **Read** — Read each file with `cairn.readFile`. Understand the context.
3. **Analyze** — Check for:
   - **Bugs**: Logic errors, nil/null dereferences, off-by-one, race conditions
   - **Security**: Injection (SQL, XSS, command), auth bypass, SSRF, hardcoded secrets
   - **Style**: Naming conventions, dead code, unnecessary complexity
   - **Performance**: N+1 queries, unbounded allocations, missing indexes
4. **Report** — Present findings grouped by severity (critical > high > medium > low).

## Output Format

For each finding:
- **File:Line** — location
- **Severity** — critical/high/medium/low
- **Issue** — what's wrong
- **Fix** — how to fix it
