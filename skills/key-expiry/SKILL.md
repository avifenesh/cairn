---
name: key-expiry
description: "Check API key, token, and TLS certificate expiry. Use when: key expiry, token expiry, credential check, cert expiry, are my keys valid, credential health, rotate keys. Keywords: key, token, expiry, credential, certificate, TLS, rotate, renewal"
argument-hint: "[--notify]"
allowed-tools: "cairn.shell"
inclusion: always
context: "tick"
---

# Key & Certificate Expiry Check

Check the health and expiry status of all API keys, tokens, and TLS certificates used by the Cairn stack. Collect data via shell commands, then format a dashboard.

## Step 1: TLS Certificate Checks

**Origin certificate (Caddy):**
```
bash -c 'set -o pipefail; openssl x509 -in /etc/caddy/certs/origin-cert.pem -noout -enddate 2>/dev/null | cut -d= -f2' || echo "N/A"
```
Parse the `notAfter` date and compute days remaining.
- \>30 days = OK
- <30 days = WARN
- <7 days = CRITICAL
- File not found = N/A

**Edge certificate (Cloudflare):**
```
timeout 3 bash -c 'set -o pipefail; echo | openssl s_client -connect agntic.garden:443 -servername agntic.garden 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2' || echo "N/A"
```
Same thresholds as origin cert. If the command fails or times out, report as N/A.

## Step 2: GitHub Token

```
timeout 10 gh api /user --include 2>/dev/null | head -20 || echo "UNAVAILABLE"
```
- If `gh` is not installed or times out = N/A
- HTTP 401 = CRITICAL (token expired or revoked)
- Check response headers for `github-authentication-token-expiration` header (present on fine-grained PATs, contains a date string)
- If header exists: parse the date, compute days remaining. <14 days = WARN, <3 days = CRITICAL
- If no header (OAuth token like `gho_`): OK (no expiry)

Note: space this call at least 2s after any other `gh api` call to avoid secondary rate limits.

## Step 3: GLM/Zhipu API Key

Check that the key is configured and the endpoint is reachable:
```
[ -n "${ZHIPU_API_KEY:-}" ] && echo "CONFIGURED" || echo "NOT_SET"
```
```
curl -sf --connect-timeout 5 --max-time 10 -o /dev/null -w '%{http_code}' https://api.z.ai/api/coding/paas/v4
```
- Key configured + endpoint reachable (401 expected without auth) = OK
- Key not set = WARN
- 000 or connection refused = DOWN

IMPORTANT: Do NOT send the actual API key in the command. The env var check uses `-n` (non-empty test), never printing the value.

## Step 4: GWS OAuth

```
timeout 10 gws gmail users messages list --params '{"userId":"me","maxResults":1}' 2>&1 | head -3
```
- Returns messages or empty list = OK
- `invalid_grant` or `Token has been expired` = CRITICAL
- `command not found` = N/A (gws not installed)

## Step 5: Gotify

Check server health (no auth required) and token file presence separately:
```
curl -sf --connect-timeout 3 --max-time 5 -o /dev/null -w '%{http_code}' http://localhost:8070/health
```
```
test -f ~/.config/gotify/token && echo "PRESENT" || echo "MISSING"
```
- Server 200 + token present = OK
- Server 200 + token missing = WARN
- 000 or connection refused = DOWN (Gotify not running, skip gracefully)

IMPORTANT: Never read or echo the token file contents. Only check file existence with `test -f`.

## Step 6: GitHub App Key (if configured)

Check via environment variable (the backend reads `GITHUB_APP_PRIVATE_KEY`):
```
[ -n "${GITHUB_APP_PRIVATE_KEY:-}" ] && echo "CONFIGURED" || echo "NOT_CONFIGURED"
```
- If CONFIGURED: report as OK (env var set, PEM key present)
- If NOT_CONFIGURED: report as N/A and skip gracefully

IMPORTANT: Never print the env var value. The `-n` test only checks non-empty.

## Step 7: Format Dashboard

Present results as a Markdown table. Use these indicators:
- `OK` — valid, not expiring soon
- `WARN` — expiring within threshold window
- `CRITICAL` — expired, revoked, or expiring imminently
- `DOWN` — service unreachable
- `N/A` — not configured, skip

### Output template

```
## Key & Certificate Expiry Dashboard

| Credential         | Type        | Status   | Detail                          |
|--------------------|-------------|----------|---------------------------------|
| TLS Origin Cert    | Certificate | OK       | Expires 2041-02-22 (5475 days)  |
| TLS Edge Cert      | Certificate | OK       | Expires 2026-05-27 (75 days)    |
| GitHub Token       | OAuth       | OK       | No expiry (OAuth token)         |
| GLM API Key        | API Key     | OK       | Endpoint reachable              |
| GWS OAuth          | OAuth       | OK       | Token valid                     |
| Gotify             | App Token   | OK       | Server healthy, token present   |
| GitHub App Key     | PEM         | N/A      | Not configured                  |
```

Adjust rows based on actual results. Omit the table header/footer chrome — just the table.

## Step 8: Push Notification (only with `--notify`)

Notifications are only sent when `$ARGUMENTS` contains `--notify`. Without the flag, only the dashboard is displayed (no side effects).

If `--notify` is present and any credential is CRITICAL:
```bash
/home/ubuntu/bin/notify "N credential(s) need attention: [summary of CRITICAL items]" 8 "Key Expiry Alert"
```

If `--notify` is present and there are WARN findings (but no CRITICAL):
```bash
/home/ubuntu/bin/notify "N credential(s) expiring soon: [summary of WARN items]" 5 "Key Expiry Warning"
```

Priority 8 for any CRITICAL finding, priority 5 for WARN-only.

## Security Notes

- Never print token or key values — only status codes and expiry dates
- All curl commands use `-o /dev/null` to suppress response bodies
- All network commands have `--connect-timeout` and `--max-time` guards
- Gracefully skip unavailable services (report as DOWN or N/A, not as errors)
- GitHub App and GLM checks use `[ -n ]` test only — never print env var values
- Gotify check uses `test -f` only — never reads token file contents
- Schedule periodic checks with the `natural-cron` skill (e.g., "check key expiry every Monday at 9 AM" → automation rule with `--notify`)
