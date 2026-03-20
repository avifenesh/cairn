---
name: tls-inspect
description: "Deep TLS certificate inspection for any host. Reports expiry, chain, cipher, SANs, protocol, and OCSP stapling status. Warns via push notification if cert expires within 14 days. Use when asked about: TLS, SSL, certificate, cert expiry, cert check, HTTPS, cipher, is the cert ok, TLS health, certificate chain, Let's Encrypt, SAN, OCSP. Keywords: tls, ssl, certificate, cert, expiry, cipher, https, chain, san, ocsp, lets encrypt, caddy"
argument-hint: "[hostname:port]"
allowed-tools: "cairn.shell"
inclusion: on-demand
---

# TLS Certificate Inspection

Deep inspection of a TLS certificate: subject, issuer, validity, SANs, key type, chain, protocol, cipher, and OCSP stapling.

## Step 1: Parse target

Extract host and port from `$ARGUMENTS`. Default to `agntic.garden:443` if no argument provided.

```
INPUT=$(cat <<'____TLS_ARG_BOUNDARY____'
$ARGUMENTS
____TLS_ARG_BOUNDARY____
)
# When no args provided, activator leaves $ARGUMENTS literal — fall back to default
if [ -z "$INPUT" ] || [ "$INPUT" = '$ARGUMENTS' ]; then
  INPUT="agntic.garden:443"
fi
INPUT=$(printf '%s' "$INPUT" | sed 's|^https\?://||; s|/.*||')
HOST=$(printf '%s' "$INPUT" | cut -d: -f1)
PORT=$(printf '%s' "$INPUT" | grep -q ':' && printf '%s' "$INPUT" | cut -d: -f2 || echo 443)
HOST=${HOST:-agntic.garden}
PORT=${PORT:-443}

if ! printf '%s' "$HOST" | grep -qE '^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$'; then
  echo "ERROR: Invalid hostname -- only alphanumeric, dots, and hyphens allowed"; exit 1
fi
if ! printf '%s' "$PORT" | grep -qE '^[0-9]+$' || [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
  echo "ERROR: Invalid port -- must be 1-65535"; exit 1
fi
echo "Target: $HOST:$PORT"
```

## Step 2: Gather all TLS data (single connection)

Connect once with all flags and save the full output. This avoids redundant connections.

```
RAW=$(echo | timeout 10 openssl s_client -connect "$HOST:$PORT" -servername "$HOST" -showcerts -status 2>/dev/null)
if [ -z "$RAW" ]; then
  echo "ERROR: Could not connect to $HOST:$PORT"; exit 1
fi
CERT=$(echo "$RAW" | openssl x509 2>/dev/null)
if [ -z "$CERT" ]; then
  echo "ERROR: Could not parse certificate from $HOST:$PORT"; exit 1
fi
echo "=== CERTIFICATE DETAILS ==="
echo "$CERT" | openssl x509 -noout -subject -issuer -dates -serial -ext subjectAltName 2>/dev/null
echo "---"
echo "$CERT" | openssl x509 -noout -text 2>/dev/null | grep -E "Public Key Algorithm|Public-Key:"
echo ""
echo "=== EXPIRY ==="
EXPIRY=$(echo "$CERT" | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
EXPIRY_EPOCH=$(date -d "$EXPIRY" +%s 2>/dev/null)
NOW_EPOCH=$(date +%s)
DAYS_LEFT=$(( (EXPIRY_EPOCH - NOW_EPOCH) / 86400 ))
echo "Expires: $EXPIRY"
echo "Days remaining: $DAYS_LEFT"
echo ""
echo "=== CHAIN ==="
CHAIN_COUNT=$(echo "$RAW" | grep -c "BEGIN CERTIFICATE")
echo "Chain depth: $CHAIN_COUNT"
TMPCHAIN=$(mktemp)
echo "$RAW" | awk '/BEGIN CERTIFICATE/,/END CERTIFICATE/' > "$TMPCHAIN"
echo "$CERT" | openssl verify -untrusted "$TMPCHAIN" -verify_hostname "$HOST" 2>&1 | tail -1
rm -f "$TMPCHAIN"
echo ""
echo "=== PROTOCOL & CIPHER ==="
echo "$RAW" | grep -E "Protocol\s*:|Cipher\s*:" | head -2
echo ""
echo "=== OCSP STAPLING ==="
echo "$RAW" | grep -A2 "OCSP response:"
```

## Step 3: Parse the results

From the output above, record:

**Certificate details:** Subject (CN), Issuer, notBefore, notAfter, Serial, SANs, Key Algorithm, Key Size.

**Expiry classification:**
- `DAYS_LEFT >= 14` -- **OK**
- `DAYS_LEFT >= 3` -- **WARN** (Caddy should have auto-renewed by now)
- `DAYS_LEFT >= 0` -- **CRITICAL** (imminent expiry, investigate Caddy renewal)
- `DAYS_LEFT < 0` -- **EXPIRED**

**Chain:** Number of certs, verification result (OK or error).

**Protocol/Cipher:** TLS version (e.g., TLSv1.3) and cipher suite (e.g., TLS_AES_256_GCM_SHA384).

**OCSP stapling:**
- Contains "OCSP Response Status: successful" -- **Stapled** (good)
- Contains "no response sent" -- **Not stapled** (not an error, but suboptimal)
- Empty or error -- **Unknown** (OCSP check failed, not critical)

## Step 4: Summary dashboard

Present all findings from Step 2 output in a structured table. Fill each value from the actual data collected:

```
## TLS Inspection: $HOST:$PORT

| Field           | Value                |
|-----------------|----------------------|
| Subject (CN)    | ...                  |
| Issuer          | ...                  |
| Valid From      | ...                  |
| Valid Until     | ...                  |
| Days Remaining  | N -- OK/WARN/CRITICAL/EXPIRED |
| Serial          | ...                  |
| SANs            | ...                  |
| Key Algorithm   | ...                  |
| Key Size        | ...                  |
| TLS Protocol    | ...                  |
| Cipher Suite    | ...                  |
| Chain Depth     | N certs              |
| Chain Valid     | Yes/No               |
| OCSP Stapled    | Yes/No/Unknown       |
```

If any field is WARN or CRITICAL, add a **Recommendations** section with actionable steps (e.g., "Check Caddy auto-renewal logs: `journalctl -u caddy --since '1 hour ago'`").

## Step 5: Push notification (if expiry warning)

If the certificate expires within 14 days, send a push notification. Substitute `$HOST` and `$DAYS_LEFT` with the actual values from Step 2 output:

```bash
HOST="agntic.garden"  # substitute actual host from Step 1
DAYS_LEFT=75          # substitute actual days from Step 2
if [ -x /home/ubuntu/bin/notify ]; then
  if [ "$DAYS_LEFT" -lt 3 ]; then
    /home/ubuntu/bin/notify "CRITICAL: TLS cert for $HOST expires in $DAYS_LEFT days! Check Caddy renewal immediately." 10 "TLS Certificate CRITICAL"
  elif [ "$DAYS_LEFT" -lt 14 ]; then
    /home/ubuntu/bin/notify "WARNING: TLS cert for $HOST expires in $DAYS_LEFT days. Caddy should have auto-renewed -- investigate." 8 "TLS Certificate Warning"
  fi
fi
```

Skip this step entirely if the expiry status is OK (>= 14 days).

## Notes

- Single TLS connection gathers all data (cert, chain, protocol, cipher, OCSP) via `-showcerts -status` flags
- Connection uses `timeout 10` to accommodate slow TLS handshakes; local parsing has no timeout concerns
- No API keys or secrets required -- uses only local `openssl`
- This skill complements `provider-status` (which does a surface cert check). Use this skill for deep inspection
- Let's Encrypt certs are 90-day; Caddy auto-renews ~30 days before expiry. A 14-day warning means renewal is failing
- Hostname validated to prevent command injection (alphanumeric, dots, hyphens); port validated as numeric 1-65535
