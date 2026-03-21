---
name: link-checker
description: "Check URLs for broken links in artifacts, documents, or URL lists. Use when asked to verify links, find dead links, audit URL health, or check for link rot. Keywords: link checker, broken links, dead links, 404, link rot, URL audit"
argument-hint: "[artifact-id | 'recent' | file-path | URL...]"
allowed-tools: "cairn.shell"
inclusion: on-demand
---

# Link Checker

Verify external URLs in Cairn artifacts, markdown files, or a provided URL list. Reports broken links with status codes and a summary dashboard.

## Step 1: Parse target

Determine the input type from `$ARGUMENTS` and extract content to scan for URLs.

**Option A — Artifact ID** (matches UUID pattern):

```
INPUT=$(cat <<'____LC_ARG_BOUNDARY____'
$ARGUMENTS
____LC_ARG_BOUNDARY____
)
if [ -z "$INPUT" ] || [ "$INPUT" = '$ARGUMENTS' ]; then
  echo "Usage: link-checker <artifact-id | 'recent' | file-path | URL...>"
  echo "Examples:"
  echo "  link-checker abc123-def456"
  echo "  link-checker recent"
  echo "  link-checker /path/to/file.md"
  echo "  link-checker https://example.com https://other.com"
  exit 0
fi
```

If the input looks like a UUID (contains hyphens, 8+ hex chars), validate and query the artifact:

```
ART_ID="$INPUT"
if ! printf '%s' "$ART_ID" | grep -qE '^[0-9A-Za-z_-]{8,}$'; then
  echo "ERROR: Invalid artifact ID format"; exit 1
fi
SAFE_ID=$(printf '%s' "$ART_ID" | sed "s/[^a-zA-Z0-9_-]//g")
CONTENT=$(timeout 5 sqlite3 "file:/home/ubuntu/cairn/data/cairn.db?mode=ro" <<SQL
.headers off
.mode list
SELECT COALESCE(SUBSTR(rendered_text, 1, 50000), '') || ' ' || COALESCE(SUBSTR(content_json, 1, 50000), '') FROM artifacts WHERE id = '$SAFE_ID' AND archived_at IS NULL;
SQL
)
if [ -z "$CONTENT" ]; then
  echo "ERROR: Artifact '$ART_ID' not found or archived"; exit 1
fi
echo "Target: artifact $ART_ID"
echo "$CONTENT"
```

**Option B — "recent"** keyword:

```
CONTENT=$(timeout 5 sqlite3 "file:/home/ubuntu/cairn/data/cairn.db?mode=ro" <<'SQL'
.headers off
.mode list
SELECT COALESCE(SUBSTR(rendered_text, 1, 50000), '') || ' ' || COALESCE(SUBSTR(content_json, 1, 50000), '') FROM artifacts WHERE archived_at IS NULL ORDER BY updated_at DESC LIMIT 20;
SQL
)
if [ -z "$CONTENT" ]; then
  echo "No recent artifacts found"; exit 0
fi
echo "Target: 20 most recent artifacts"
echo "$CONTENT"
```

**Option C — File path** (starts with `/` or `~` or `./`):

```
FILE_PATH="$INPUT"
if [ ! -f "$FILE_PATH" ]; then
  echo "ERROR: File not found: $FILE_PATH"; exit 1
fi
CONTENT=$(cat "$FILE_PATH")
echo "Target: file $FILE_PATH"
echo "$CONTENT"
```

**Option D — Direct URL(s)** (starts with `http`):

If the input contains one or more URLs, use them directly — skip Step 2 (URL extraction) and go straight to Step 3 (filtering).

Choose the appropriate option based on the input pattern. Run the corresponding shell block via `cairn.shell`.

## Step 2: Extract URLs

From the content obtained in Step 1, extract all HTTP/HTTPS URLs:

```
URLS=$(printf '%s' "$CONTENT" | grep -oE 'https?://[^"'"'"'<>[:space:]\)\]\},]+' | sed 's/[.,;:!?)]*$//' | sort -u | head -50)
URL_COUNT=$(printf '%s\n' "$URLS" | grep -c '^http' || true)
echo "Found $URL_COUNT unique URLs (capped at 50)"
echo "$URLS"
```

The regex extracts URLs and strips trailing punctuation that often gets captured from markdown/JSON. Results are deduped and capped at 50.

If zero URLs are found, report "No URLs found in target content" and stop.

## Step 3: Filter unsafe URLs

Remove private/internal URLs that must not be checked externally. Run this filter on the URL list:

```
SAFE_URLS=""
SKIPPED=0
while IFS= read -r URL; do
  [ -z "$URL" ] && continue
  DOMAIN=$(printf '%s' "$URL" | sed 's|https\?://||' | cut -d/ -f1 | cut -d: -f1)
  if printf '%s' "$DOMAIN" | grep -qE '^(localhost|127\.|10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.|169\.254\.|0\.|::1|\[::1\]|metadata\.google|169\.254\.169\.254)'; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi
  if printf '%s' "$URL" | grep -qE '^(file|mailto|data|javascript|tel|ftp):'; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi
  # DNS rebinding protection: resolve domain and verify all IPs are public
  RESOLVED_IPS=$(getent ahosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | sort -u)
  if printf '%s\n' "$RESOLVED_IPS" | grep -qE '^(127\.|10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.|169\.254\.|0\.|::1|fe80:|fc00:|fd[0-9a-f])'; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi
  SAFE_URLS="$SAFE_URLS
$URL"
done <<'____LC_URL_LIST____'
$URLS_FROM_STEP2
____LC_URL_LIST____
SAFE_URLS=$(printf '%s' "$SAFE_URLS" | sed '/^$/d')
SAFE_COUNT=$(printf '%s\n' "$SAFE_URLS" | grep -c '^http' || true)
echo "Checking $SAFE_COUNT URLs ($SKIPPED skipped as private/internal)"
echo "$SAFE_URLS"
```

Substitute `$URLS_FROM_STEP2` with the actual URL list from Step 2 output. If zero safe URLs remain, report and stop.

## Step 4: Check each URL

For each safe URL, send a HEAD request and record the HTTP status code. Run this check loop:

```
echo "=== LINK CHECK RESULTS ==="
OK=0; BROKEN=0; ERRORS=0; REDIRECTS=0; IDX=0
while IFS= read -r URL; do
  [ -z "$URL" ] && continue
  IDX=$((IDX + 1))
  CODE=$(curl -sI -o /dev/null -w '%{http_code}' --max-time 5 --max-redirs 5 -L --proto-redir =https -A 'Mozilla/5.0 (compatible; PubLinkChecker/1.0)' "$URL" 2>/dev/null || echo "000")
  # Retry with GET if server rejects HEAD (405 Method Not Allowed)
  if [ "$CODE" = "405" ]; then
    CODE=$(curl -so /dev/null -w '%{http_code}' --max-time 5 --max-redirs 5 -L --proto-redir =https -A 'Mozilla/5.0 (compatible; PubLinkChecker/1.0)' "$URL" 2>/dev/null || echo "000")
  fi
  if [ "$CODE" -ge 200 ] && [ "$CODE" -lt 300 ]; then
    STATUS="OK"; OK=$((OK + 1))
  elif [ "$CODE" -ge 300 ] && [ "$CODE" -lt 400 ]; then
    STATUS="REDIRECT"; REDIRECTS=$((REDIRECTS + 1))
  elif [ "$CODE" -ge 400 ] && [ "$CODE" -lt 500 ]; then
    STATUS="BROKEN"; BROKEN=$((BROKEN + 1))
  elif [ "$CODE" -ge 500 ]; then
    STATUS="ERROR"; ERRORS=$((ERRORS + 1))
  else
    STATUS="TIMEOUT"; ERRORS=$((ERRORS + 1))
  fi
  echo "$IDX|$CODE|$STATUS|$URL"
  sleep 0.5
done <<'____LC_CHECK_LIST____'
$SAFE_URLS_FROM_STEP3
____LC_CHECK_LIST____
echo "=== SUMMARY ==="
echo "OK=$OK BROKEN=$BROKEN REDIRECTS=$REDIRECTS ERRORS=$ERRORS"
```

Substitute `$SAFE_URLS_FROM_STEP3` with the filtered URL list from Step 3. The `curl` command:
- Follows redirects (`-L --max-redirs 5 --proto-redir =https`) — only allows HTTPS redirects to prevent SSRF via redirect to private IPs
- Uses a descriptive User-Agent to avoid bot-blocking on sites that reject default curl UA
- Times out after 5 seconds per URL (HEAD requests are lightweight)
- Automatically retries with GET if server returns 405 Method Not Allowed for HEAD
- 0.5s delay between requests to avoid triggering rate limits on target servers

## Step 5: Report

Parse the output from Step 4 and present a structured report. Format the `IDX|CODE|STATUS|URL` lines into this table:

```
## Link Check Report

**Target:** [artifact title / file path / URL list]
**Checked:** N URLs | **OK:** X | **Broken:** Y | **Redirects:** Z | **Errors:** W

| # | URL | Status | Code |
|---|-----|--------|------|
| 1 | https://example.com/page | OK | 200 |
| 2 | https://dead.example.com | BROKEN | 404 |
| 3 | https://slow.example.com | TIMEOUT | 000 |
```

If all links are OK, summarize briefly: "All N links are healthy."

If broken links are found, list them prominently and suggest actions:
- 404: link target may have moved or been deleted
- 403: link target blocks automated checks (may still work in browser)
- 5xx: server-side error, may be temporary — recheck later
- 000/TIMEOUT: host unreachable or very slow

## Notes

- HEAD requests that return 405 are automatically retried with GET
- CDNs (Cloudflare, Akamai) may return 403 to automated requests — flag as "BLOCKED (may work in browser)" not "BROKEN"
- 50-URL cap prevents long-running checks; for larger scans, run multiple times with specific artifact IDs
- Private IP filtering prevents SSRF — never sends requests to internal networks, localhost, or cloud metadata
