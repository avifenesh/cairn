# Agent Hardening -- Cairn Defense Inventory & Gap Analysis

Companion reference for the `agent-hardening` skill. Documents Cairn's existing security layers, identifies gaps, and recommends remediations.

## Cairn's Current Defense Inventory

| # | Defense Layer | File | OWASP | Trust Boundary |
|---|--------------|------|-------|----------------|
| 1 | Dual-LLM email quarantine | `email-triage-runner.ts` | LLM01 | External email -> assistant |
| 2 | XML escaping in prompt construction | `conversation-context.ts:26-28`, `catalog.ts:30` | LLM01 | User/assistant content -> LLM prompt |
| 3 | Tag stripping in memory pipeline | `memory-context.ts:80-87`, `memory-extractor.ts:289` | LLM01 | Stored memories -> system prompt |
| 4 | Feed title sanitization | `agent-loop.ts:578` | LLM01 | External feed -> agent context |
| 5 | Secret scanning in skills | `secrets-scanner.ts`, `catalog.ts:124-131` | LLM02 | Skill content -> LLM context |
| 6 | PII detection in memory extraction | `memory-extractor.ts:87-123` | LLM02 | LLM output -> memory storage |
| 7 | Redirect blocking on fetch | `web-fetch.ts:330` | LLM07 | Tool -> external URL |
| 8 | Zod schema validation for tools | `registry.ts`, `executor.ts:67` | LLM05 | LLM -> tool execution |
| 9 | Tool loop guard | `loop-guard.ts` | LLM05 | LLM -> repeated tool calls |
| 10 | Shell command policy engine | `shell-policy.ts` | LLM05 | LLM -> shell execution |
| 11 | Environment isolation | `cli-subprocess.ts:31` | LLM05 | Coding subprocess -> host |
| 12 | SSRF protection + DNS rebinding | `web-fetch.ts:229-313` | LLM07 | Tool -> internal network |
| 13 | Webhook signature verification | `routes/webhooks.ts` | LLM01 | External webhook -> feed |
| 14 | MCP write rate limiting | `tools-write.ts` | LLM05 | External agent -> tools |

### Defense Detail Notes

**Defense 1 -- Dual-LLM email quarantine:**
The quarantine LLM receives raw email content but has NO tool access and NO memory context. Email fields are XML-escaped before being sent to the quarantine LLM. The quarantine output summary is then tag-stripped and capped at 200 characters. The triage LLM (with tools and memories) only sees these sanitized summaries, never raw email bodies.

**Defense 2 -- XML escaping:**
`escapeXml()` converts angle brackets and ampersands to XML entities. The `catalog.ts` variant also escapes quotes. Applied in `conversation-context.ts` (user/assistant turns), `catalog.ts` (skill names/descriptions), and `trip-runner.ts` (search snippets).

**Defense 3 -- Memory pipeline sanitization:**
`sanitizeMemoryContent()` in `memory-context.ts` collapses newlines and strips all XML/HTML tags before injecting memories into system prompts. Separately, `memory-extractor.ts` strips tags from user/assistant messages before passing them to the extraction LLM.

**Defense 7 -- Redirect blocking:**
`web-fetch.ts` uses `redirect: "error"` on all fetch calls. This prevents SSRF bypass via open redirectors -- a public URL that 301-redirects to an internal address would be blocked rather than followed.

**Defense 12 -- SSRF + DNS rebinding:**
Two-phase protection: (a) URL-level check blocks private IPs, loopback, link-local, cloud metadata, non-HTTP(S) protocols, and IPv4-mapped IPv6 addresses; (b) DNS resolution check catches domain names that resolve to private IPs after the URL check passes, preventing DNS rebinding attacks.

**Defense 14 -- MCP rate limiting:**
Per-session sliding window rate limiter on all MCP write operations. Default 100 writes/minute. Prevents runaway external agent from exhausting resources or flooding the system.

---

## Gap Analysis

### GAP-1 (HIGH): A2A message content unsanitized

**Location:** `a2a/worker.ts` -- remote agent messages (text parts) are processed without tag stripping or XML escaping before being forwarded to the assistant context.

**Risk:** A compromised or malicious remote agent could embed adversarial content in A2A message payloads. Since these messages flow into the assistant's context during task execution, they create an indirect injection path.

**Impact:** Could redirect assistant behavior during A2A task execution -- the most direct external-to-LLM path that currently lacks sanitization.

### GAP-2 (MEDIUM): Webhook payloads stored raw

**Location:** `routes/webhooks.ts` -- the webhook body JSON is stored directly into feed events (up to 4KB) after signature verification passes. The body content is not tag-stripped or sanitized.

**Risk:** A legitimate webhook source (with valid signature) could carry adversarial content in its payload. When the agent loop processes unread feed items, this content enters the LLM context via the feed title/body rendering path. While feed titles are sanitized at `agent-loop.ts:578`, the full event body may reach the LLM through other code paths (e.g., event detail views, search results).

**Impact:** Indirect injection when agent loop processes feed items containing webhook data.

### GAP-3 (LOW): Memory not sanitized at storage time

**Location:** Memories are stored with their original content in the database. Sanitization occurs only at injection time in `memory-context.ts:80-87`.

**Risk:** An accepted memory containing adversarial content persists indefinitely. The defense relies on a single sanitization layer at read time.

**Impact:** Low -- the injection-time sanitization is effective and well-tested. However, defense-in-depth argues for an additional sanitization pass at storage time so that if the read-time layer is ever bypassed or modified, the stored content is already safe.

### GAP-4 (LOW): MCP create_memory allows 16KB free-text

**Location:** `internal/tool/builtin/memory.go` -- `cairn.createMemory` accepts content up to 16,384 characters with no content sanitization beyond Zod string validation.

**Risk:** An external MCP client could store large content blocks that influence future LLM behavior when injected via memory context. The `sanitizeMemoryContent()` function at read time mitigates this, but a tighter size limit reduces the attack surface.

**Impact:** Low -- memory-context sanitization provides effective defense. Tighter limits are a defense-in-depth measure.

### GAP-5 (LOW): Skill bodies not scanned for injection patterns

**Location:** `secrets-scanner.ts` scans skill content for hardcoded credentials (API keys, tokens, connection strings, PEM keys) but does not check for prompt injection markers.

**Risk:** An imported skill could contain role override markers or system prompt manipulation patterns that alter LLM behavior when the skill body is loaded into context. The `skill-vetter` skill checks for data exfiltration risk but not subtle prompt manipulation.

**Impact:** Low -- skill import requires manual vetting, and the `escapeXml()` applied to skill names/descriptions in `catalog.ts` provides partial protection. The skill body itself is loaded as raw content within `<skill>` tags.

---

## Remediation Recommendations

### GAP-1 (HIGH priority): Sanitize A2A message content

Add `stripTags()` to text parts in A2A message processing within `a2a/worker.ts` before forwarding to the assistant. Wrap remote agent content in delimited XML tags with explicit source attribution:

```
<agent_message source="remote" agent="agent-name">
  {sanitized content}
</agent_message>
```

This follows the same pattern used by `conversation-context.ts` for user/assistant turns.

### GAP-2 (MEDIUM priority): Sanitize webhook event content

Add tag stripping to webhook event title and body before feed insertion in `routes/webhooks.ts`. Cap the body field to 1KB when it will enter LLM context (the full payload can remain in the database for programmatic access). Apply the same tag-stripping function used in `agent-loop.ts:578`.

### GAP-3 (LOW priority): Add storage-time memory sanitization

Add a `sanitizeMemoryContent()` call at insert time in the memory repository, preserving the existing injection-time sanitization as a second layer. This ensures adversarial content is neutralized at rest, not just at read time.

### GAP-4 (LOW priority): Tighten MCP memory content limits

Reduce `cairn.createMemory` content maximum from 16KB to 4KB in the Zod schema. Add `stripTags()` to the content before storage. This limits the volume of unsanitized external content that can accumulate in the memory store.

### GAP-5 (LOW priority): Add injection marker detection to skill scanner

Add a heuristic scan in the skill loader (alongside `secrets-scanner.ts`) that warns on common injection markers in skill bodies. Patterns to detect include bare model-specific delimiters and role override tags. This should warn (log) but not block, since legitimate skill content may reference these patterns in documentation.

---

## Cost-Benefit Assessment

Pub is single-user and self-hosted -- the threat model is narrower than multi-tenant SaaS systems. The 14 existing defense layers cover all major OWASP LLM Top 10 categories relevant to agent systems.

**Priority ordering:**
1. **GAP-1** (HIGH) -- close first. A2A is the most direct unsanitized external-to-LLM path
2. **GAP-2** (MEDIUM) -- close second. Webhook payloads are a less direct but real injection vector
3. **GAP-3/4/5** (LOW) -- defense-in-depth improvements. Not urgent for single-user deployment

**Out of scope for Cairn's threat model:**
- Canary token injection (requires multi-user with distinct token sets)
- Perplexity-based injection detection (high false-positive rate, significant compute cost)
- Instruction hierarchy enforcement (emerging research, not production-ready)
- Hardware-backed attestation for agent identity (enterprise/regulated use case)

---

## Testing Recommendations

All security testing should use benign marker strings, never actual adversarial payloads:

- **A2A sanitization**: Embed `[TEST_MARKER_12345]` in A2A message text parts and verify the marker is tag-stripped before reaching assistant context
- **Webhook sanitization**: Send a webhook payload containing a benign test marker string and verify it is tag-stripped in the stored feed event
- **Memory sanitization**: Store a memory containing `[ROLE_TEST_MARKER]` and verify sanitization is applied both at storage and at injection time
- **Redirect blocking**: Attempt to fetch a URL that 301-redirects to a private IP -- verify the request is blocked with "Request blocked by security policy"
- **SSRF protection**: Attempt to fetch the cloud metadata service endpoint -- verify it is blocked at the URL check layer
- **DNS rebinding**: Attempt to fetch a domain that resolves to `127.0.0.1` -- verify it is blocked at the DNS resolution layer
- **Tool loop guard**: Simulate repeated identical tool calls and verify the guard triggers after the configured threshold
