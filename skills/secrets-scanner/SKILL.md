---
name: secrets-scanner
description: "Scan files or text for hardcoded secrets, API keys, tokens, PEMs, passwords, and connection strings. Use when importing external skills, vetting content, or auditing for credential leaks. Keywords: scan, secrets, credentials, api key, token, password, PEM, leak, vet, audit"
inclusion: on-demand
allowed-tools: "Read,Glob"
argument-hint: "[file-path]"
---

# Secrets Scanner

Scan a file or set of files for hardcoded secrets and credentials.

## Steps

1. **Read the target file(s)**
   - If `$ARGUMENTS` is a file path, read it
   - If `$ARGUMENTS` is a glob pattern, expand and read matching files
   - If no argument, ask the user which file(s) to scan

2. **Check content against these secret patterns**

   | Pattern | Type | Examples |
   |---------|------|----------|
   | Well-known token prefixes | vendor_token | `ghp_`, `ghs_`, `sk-`, `xoxb-`, `glpat-`, `pypi-`, `npm_` |
   | AWS access keys | aws_access_key | `AKIA` followed by 16 alphanumeric chars |
   | Stripe keys | stripe_key | `sk_live_`, `pk_live_`, `rk_live_` |
   | Twilio SID | twilio_sid | `AC` followed by 32 hex chars |
   | SendGrid keys | sendgrid_key | `SG.` followed by base64 segments |
   | PEM private keys | pem_private_key | `-----BEGIN * PRIVATE KEY-----` blocks |
   | Connection strings | connection_string | `postgres://user:pass@host`, `mongodb://...`, `redis://...` |
   | Labeled credentials | labeled_credential | `api_key=...`, `token: ...`, `password=...`, `secret=...` |
   | Bearer tokens | bearer_token | `Bearer ` followed by long token string |
   | High-entropy strings | entropy | Hex strings 32-256 chars, base64-like 64-512 chars |

3. **Report findings**

   For each finding, report:
   ```
   - **[TYPE]** line N: `prefix...suffix` (redacted)
   ```

   Group by severity:
   - **Critical**: Well-known token prefixes, PEM keys, connection strings with passwords
   - **High**: Labeled credentials, vendor-specific keys
   - **Medium**: High-entropy strings, Bearer tokens

4. **Summary and remediation**

   ```markdown
   ## Scan Results: filename

   **Status**: CLEAN | N findings detected

   ### Findings
   [grouped by severity]

   ### Remediation
   - Move secrets to environment variables
   - Use a `.env` file (excluded from git)
   - For production: use a secrets manager (AWS SSM, Vault, etc.)
   - For skills: reference `${ENV_VAR}` instead of hardcoding values
   ```

## Notes

- This skill is read-only and makes no changes to files
- Findings are redacted in output (only prefix/suffix shown)
- False positives are possible with high-entropy patterns — use judgment
- This scanner also runs automatically whenever any skill is activated — skills containing secrets are redacted before being sent to the LLM
