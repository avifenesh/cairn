---
name: system-health
description: "Use when asked about system health, server resources, infrastructure status, or 'how is the server doing'. Comprehensive host-level health report: CPU, memory, disk, database, Go process, TLS cert, SSE clients, poller health. For provider-specific checks use /provider-status instead. Keywords: system health, server, cpu, memory, disk, database, db size, wal, process memory, process, sse clients, poller, infrastructure, resources, uptime"
argument-hint: "[--verbose]"
allowed-tools: "cairn.shell,cairn.getStatus"
inclusion: always
context: "tick"
---

# System Health Check

Run a comprehensive host-level infrastructure health check. Collect OS metrics, database status, application state, and network/TLS info, then format a unified dashboard.

For provider-specific checks (GitHub API, GLM, SearXNG, Gmail push, feed source circuits), use `/provider-status` instead.

## Step 1: Application metrics via builtin tools

Call these two tools (pre-authenticated, no token needed):

1. **`cairn.getStatus`** — returns `healthy` boolean, `issues` array, `taskQueue` (with `byStatus` map: `queued`, `running`, `completed`, `failed`), `agents` (with `active` and `total`), `deadLetters` (with `.count` and `.recent`), `budget` (with `.recentSpendUsd` and `.windowMs`)
2. **`cairn.getStatus`** with sections `tasks,agents,feed,memory,costs` — returns per-section data: task counts, agent counts, feed stats, memory counts (total/accepted/proposed), cost totals (`todayUsd`, `dailyCapUsd`, `dailyPctUsed`)

Extract from the results:
- `healthy` boolean and any `issues` (from getSystemHealth)
- `taskQueue.byStatus` — `queued`, `running`, `failed` counts (from getSystemHealth)
- `agents.active` / `agents.total` (from getSystemHealth)
- `budget.recentSpendUsd` from getSystemHealth; `costs.todayUsd` / `costs.dailyCapUsd` / `costs.dailyPctUsed` from cairn.getStatus
- Memory system stats: total, accepted, proposed (from cairn.getStatus memory section)

## Step 2: Infrastructure checks via cairn.shell

Run these commands to gather host-level metrics. Commands are read-only but may require shell approval depending on policy.

**Memory usage:**
```
bash -c 'set -o pipefail; free -m | awk "/^Mem:/ { if (\$2 > 0) printf \"total: %dMB, used: %dMB, available: %dMB, pct_used: %.1f%%\\n\", \$2, \$3, \$7, (\$3/\$2)*100; else print \"memory data incomplete\" }"' || echo "memory check failed"
```

**CPU load:**
```
echo "load_avg: $(cut -d' ' -f1-3 /proc/loadavg), cores: $(nproc)"
```

**Disk usage:**
```
bash -c 'set -o pipefail; df -h /home/ubuntu/cairn | tail -1 | awk "{printf \"size: %s, used: %s, avail: %s, pct_used: %s\n\", \$2, \$3, \$4, \$5}"' || echo "disk check unavailable"
```

**Database files:**
```
du -sb /home/ubuntu/.cairn/data/cairn.db /home/ubuntu/.cairn/data/cairn.db-wal /home/ubuntu/.cairn/data/cairn.db-shm 2>/dev/null | awk 'BEGIN { db=0; wal=0; shm=0; sum=0 } { if ($2 ~ /\.db$/) {db=$1} else if ($2 ~ /\.db-wal$/) {wal=$1} else if ($2 ~ /\.db-shm$/) {shm=$1}; sum+=$1 } END { printf "db_bytes: %d\nwal_bytes: %d\nshm_bytes: %d\ntotal_bytes: %d\n", db, wal, shm, sum }'
```

**Database integrity:**
```
bash -c 'set -o pipefail; timeout 10 sqlite3 /home/ubuntu/.cairn/data/cairn.db "PRAGMA integrity_check;" | head -1' || echo "integrity check timed out or failed"
```

**Database table count and row stats:**
```
timeout 5 sqlite3 /home/ubuntu/.cairn/data/cairn.db "SELECT 'tables: ' || COUNT(*) FROM sqlite_master WHERE type='table' UNION ALL SELECT 'events: ' || COUNT(*) FROM events UNION ALL SELECT 'memories: ' || COUNT(*) FROM memories UNION ALL SELECT 'tasks: ' || COUNT(*) FROM tasks;" 2>&1 || echo "database query timed out or failed"
```

**Runtime metrics (SSE clients, poller, request stats):**
```
if [ -z "$READ_API_TOKEN" ]; then echo "metrics auth required — READ_API_TOKEN not set"; else TMPF=$(mktemp --mode=0600) && CFG=$(mktemp --mode=0600) && trap 'rm -f "$TMPF" "$CFG"' EXIT && echo "header = \"Authorization: Bearer $READ_API_TOKEN\"" > "$CFG" && HTTP_CODE=$(curl -s -o "$TMPF" -w '%{http_code}' --connect-timeout 3 --max-time 5 --config "$CFG" http://localhost:8788/v1/metrics 2>/dev/null); if [ "$HTTP_CODE" = "200" ]; then jq -r '"sse_clients: \(.sse.connectedClients // 0), poller_cycles: \(.poller.cycles // 0), poller_failures: \(.poller.failures // 0), poller_inserted: \(.poller.inserted // 0), req_total: \(.requests.total // 0), req_p50: \(.requests.p50LatencyMs // 0), req_p99: \(.requests.p99LatencyMs // 0)"' "$TMPF" 2>/dev/null || echo "invalid JSON response"; elif [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then echo "metrics auth failed (HTTP $HTTP_CODE)"; else echo "metrics unavailable (HTTP $HTTP_CODE)"; fi; fi
```

**TLS cert expiry:**
```
DOMAIN=${FRONTEND_DOMAIN:-agntic.garden}; timeout 5 bash -c "set -o pipefail; echo | openssl s_client -connect $DOMAIN:443 -servername $DOMAIN 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2" || echo "UNKNOWN"
```

**Systemd services:**
```
echo "cairn: $(systemctl is-active cairn 2>/dev/null || echo unknown)"
echo "caddy: $(systemctl is-active caddy 2>/dev/null || echo unknown)"
```

**Process info:**
```
if [ -z "$READ_API_TOKEN" ]; then echo "status auth required — READ_API_TOKEN not set"; else TMPF=$(mktemp --mode=0600) && CFG=$(mktemp --mode=0600) && trap 'rm -f "$TMPF" "$CFG"' EXIT && echo "header = \"Authorization: Bearer $READ_API_TOKEN\"" > "$CFG" && HTTP_CODE=$(curl -s -o "$TMPF" -w '%{http_code}' --connect-timeout 3 --max-time 5 --config "$CFG" http://localhost:8788/v1/status 2>/dev/null); if [ "$HTTP_CODE" = "200" ]; then jq -r '"uptime_sec: \(.system.uptime // "unknown"), version: \(.system.version // "unknown"), go: \(.system.go // "unknown")"' "$TMPF" 2>/dev/null || echo "invalid JSON response"; elif [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then echo "status auth failed (HTTP $HTTP_CODE)"; else echo "status unavailable (HTTP $HTTP_CODE)"; fi; fi
```

If `$ARGUMENTS` contains `--verbose`, also run:

**Artifact storage size:**
```
du -sh /home/ubuntu/.cairn/data/artifacts/ 2>/dev/null || echo "no artifacts dir"
```

**WAL journal mode and auto-checkpoint interval:**
```
timeout 5 sqlite3 /home/ubuntu/.cairn/data/cairn.db 'PRAGMA journal_mode; PRAGMA wal_autocheckpoint;' 2>&1 || echo "wal query failed"
```

**Per-source poller stats** (from metrics response): report `poller.bySource` object with per-source `inserted`, `failures`, and `retries` counts.

## Step 3: Format dashboard

Present results as a structured health report. Use these indicators:
- `OK` — within normal range
- `WARN` — approaching threshold, attention needed soon
- `CRIT` — exceeds safe threshold, action needed now

### Output template

```
## System Health Report

**Generated:** YYYY-MM-DD HH:MM UTC | **App status:** healthy/unhealthy

### Host Resources
| Metric | Value | Status |
|--------|-------|--------|
| Memory | X/Y MB (Z%) | OK |
| CPU Load | X.XX (N cores) | OK |
| Disk | X/Y GB (Z%) | OK |
| Uptime | Xd Xh Xm | — |

### Database
| Metric | Value | Status |
|--------|-------|--------|
| DB Size | X MB | OK |
| WAL Size | X MB | OK |
| Integrity | ok | OK |
| Tables | N | — |
| Events | N rows | — |
| Memories | N rows | — |

### Application
| Metric | Value | Status |
|--------|-------|--------|
| Version | abc1234 | — |
| Go | vX.Y.Z | — |
| SSE Clients | N connected | — |
| Poller | N cycles, M failures | OK |
| Task Queue | N queued, M running | — |
| Agents | N active / M total | — |
| Budget | $X.XX today / $Y.YY cap (Z%) | OK |
| Req Latency | p50: Xms, p99: Yms | — |

### Network / TLS
| Service | Status | Detail |
|---------|--------|--------|
| cairn | OK | active (systemd) |
| Caddy | OK | active (systemd) |
| TLS Cert | OK | expires YYYY-MM-DD (N days) |

### Issues & Recommendations
- [List any WARN/CRIT items with actionable recommendations]
- [If no issues: "All systems nominal."]

> For provider-specific checks (GitHub, GLM, SearXNG, feed sources), run `/provider-status`.
```

If `--verbose` was passed, include all rows. Otherwise, omit rows that are purely informational (like version, runtime version) and focus on rows with status indicators.

## Notes

- **Thresholds:**
  - Memory: > 80% = WARN, > 95% = CRIT
  - Disk: > 80% = WARN, > 95% = CRIT
  - CPU load: > nproc = WARN, > 2 * nproc = CRIT
  - DB WAL size: > 100MB = WARN, > 500MB = CRIT
  - TLS cert: < 14 days = WARN, < 3 days = CRIT
  - Budget: > 80% daily cap = WARN, > 95% = CRIT
- **DB path:** uses `/home/ubuntu/.cairn/data/cairn.db` (production deployment path, configured via `DATABASE_PATH` in `.env.cairn`)
- **Metrics/status endpoints:** require `READ_API_TOKEN` when token auth is configured. If curl returns 401, report as "auth required" rather than "unavailable"
- **This skill executes read-only shell commands** — no system modifications are made. Some commands may require shell approval depending on policy
- **Overlap with /provider-status:** This skill does NOT check external provider reachability or feed source circuits. Always refer the user to `/provider-status` for those checks
