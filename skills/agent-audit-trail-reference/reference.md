# Pub-Specific Adaptation Guide

This file contains detailed guidance for adapting audit trail patterns to Cairn's architecture. See SKILL.md for general audit trail concepts.

## Current State

Pub uses Pino for structured logging:
- JSON logs with timestamps and context
- Rotated daily
- No tampering detection
- No decision chain tracing

## Do We Need Audit Trails?

**Yes, if**:
- Compliance requirements (GDPR audit logs, SOC 2)
- Multi-tenant deployment (prove actions to customers)
- High-risk operations (production deploys, data deletion)
- User disputes ("agent did X without permission")
- Forensic investigations

**No, if**:
- Single-user self-hosted system
- No regulatory requirements
- Observability is sufficient (debug, not prove)

## Integration Approach

### Option A: Hash-Chained Audit for Critical Operations Only

Scope: approval decisions, tool executions, memory proposals

Implementation:
```typescript
// Create audit writer alongside main logger
const auditWriter = new AuditWriter({
  logDir: config.auditDir,
  captureMode: "metadata_only",
  rotation: { strategy: "daily" }
});

// In approval-dispatcher.ts
auditWriter.append({
  type: "approval_decision",
  timestamp: new Date().toISOString(),
  runId: approval.taskId,
  payload: {
    approvalId: approval.id,
    action: approval.action,
    riskLevel: approval.riskLevel,
    decision: "approved" | "denied",
    userId: session.userId
  }
});

// In tool executor
auditWriter.append({
  type: "tool_execution",
  timestamp: new Date().toISOString(),
  runId: task.id,
  payload: {
    toolName: tool.name,
    success: result.success,
    riskLevel: tool.riskLevel,
    durationMs: elapsed
  }
});
```

### Option B: Full Audit Integration

Scope: All assistant interactions, LLM calls, tool executions

- Instrument assistant-runner.ts with audit events
- Add runId/sessionId to all task contexts
- Implement verify/trail/report CLIs
- Add audit log export API

### Option C: Blockchain Anchoring (Overkill for Current Scale)

Only if Cairn becomes multi-tenant SaaS with compliance needs.

## Cost-Benefit Analysis for Cairn

| Factor | Weight | Score | Notes |
|--------|--------|-------|-------|
| Compliance Need | Med | Low | Single-user, self-hosted |
| User Disputes | Med | Low | User trusts their own system |
| Forensics Value | High | Med | Helpful for debugging complex agent loops |
| Implementation Cost | High | Med | ~3-5d for full integration |
| Performance Impact | Med | Low | Append-only, minimal overhead |
| **Total** | - | **Low-Med** | Optional, not critical |

## Recommendation

**Defer for now.** Cairn's current Pino logging is sufficient for:
- Debugging and observability
- Performance analysis
- Error tracing

Consider audit trails when:
1. Multi-tenant deployment planned
2. Compliance requirements emerge
3. High-risk autonomous operations increase
4. User disputes become common

If implementing, start with **Option A** (critical operations only) rather than full instrumentation.
