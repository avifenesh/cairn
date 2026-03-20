---
name: agent-audit-trail-reference
description: "Use when user asks about audit trails, verifiable logs, compliance logging, tamper-evident logging, hash-chained logs, or detecting log tampering. Reference material on tamper-evident audit logging patterns for AI Agent systems. Keywords: audit trail, tamper evident, hash chain, immutable logs, compliance, forensics, agent logging"
inclusion: on-demand
---

# Agent Audit Trail — Reference

Reference-only skill documenting tamper-evident audit logging patterns for AI Agent systems.

## Core Problem

Standard application logs (Pino, Winston, etc.) solve observability but not **verifiability**:
- Logs can be modified after-the-fact without detection
- No proof that logs weren't deleted or selectively edited
- Difficult to reconstruct complete decision chains across multiple components
- No built-in compliance features for regulated environments

For AI Agents making autonomous decisions, this becomes critical when:
- User disputes what the agent actually did
- Security team needs to prove logs weren't tampered with
- Compliance requires immutable records
- Incident investigation needs complete decision trace

## Hash-Chained Audit Logs

### Core Concept

Each audit event contains:
```json
{
  "seq": 3,
  "prevHash": "a3f2...",  // SHA-256 of previous event
  "hash": "d8f1...",      // SHA-256 of current event
  "type": "tool_call_after",
  "timestamp": "2026-03-13T10:00:03.000Z",
  "runId": "run-xyz",
  "sessionId": "session-001",
  "payload": { "toolName": "bash", "success": true }
}
```

**Verification**: Any modification/deletion/insertion breaks the chain. Validator can detect exactly which `seq` was tampered.

**Canonical JSON**: Objects are serialized with stable key ordering to ensure identical content produces identical hashes.

### Key Benefits Over Plain Logs

| Feature | Plain Logs | Hash-Chained Audit |
|---------|-----------|-------------------|
| Tampering Detection | No | Yes (locates exact seq) |
| Decision Chain | Manual reconstruction | Built-in runId/sessionId tracing |
| Compliance | Rotation only | Verifiable immutability |
| Privacy Control | All or nothing | Metadata-only + field redaction |
| Integration | Logging framework | Event-driven audit system |

## Two Battle-Tested Patterns

### Pattern 1: Agent-Audit-Trail (kanson1996)

**Source**: TypeScript, well-tested

**Architecture**:
- Framework-agnostic core (`agent-audit-trail` package)
- Plugin adapter (`@kanson1996/audit-trail`)
- JSONL append-only files with hash chains
- Four operations: verify / trail / search / report

**Capture Modes**:
- `metadata_only`: Production mode — lengths, summaries, counts (no raw content)
- `full_capture`: Debug mode — complete data with optional field redaction

**Redaction**:
- `hash`: Replace with SHA-256 hash
- `omit`: Remove entirely
- `truncate`: Shorten to N characters

**File Structure**:
```
~/.audit/
├── index.jsonl
└── 2026-03-13/
    └── audit-2026-03-13.jsonl
```

**Event Types**: session_start, llm_input, tool_call_before, tool_call_after, subagent_spawned, agent_end

**CLI Commands**:
```bash
audit verify                           # Check integrity
audit trail --run <runId>              # Reconstruct decision chain
audit search --tool bash --from ...   # Filter events
audit report --from 2026-03-01        # Compliance summary
```

**Thread Safety**: Promise queue ensures serial writes per file, prevents race conditions.

### Pattern 2: Blockchain-Anchored Ledger (darkraider01)

**Source**: Enterprise Java/Spring Boot, Ethereum/Web3j

**Architecture**:
- Local database stores full audit events
- Merkle tree batches thousands of hashes
- Only Merkle root anchored to blockchain (gas efficient)
- Zero-knowledge privacy (raw data never on-chain)

**Verification Layers**:
1. **Integrity**: Local DB record matches original hash
2. **Membership**: Record is in the Merkle batch
3. **Finality**: Batch root is on blockchain

**Gas Optimization**: Anchor 10,000 logs in one transaction vs 10,000 separate transactions.

**Use Case**: Regulated industries (finance, healthcare) needing external verifiability.

## Pub-Specific Guidance

For detailed guidance on adapting these audit trail patterns to Cairn's architecture, see `reference.md` in this directory. It covers:
- Current logging state (Pino)
- Cost-benefit analysis for Cairn's use case
- Three integration options (critical ops only, full audit, blockchain anchoring)
- Implementation code examples
- Recommendation: defer for now (single-user, self-hosted)

## Reference Links

- agent-audit-trail: https://github.com/kanson1996/agent-audit-trail
- Blockchain Audit Ledger: https://github.com/darkraider01/Blockchain-Anchored-Audit-Ledger-Service
- Hash Chains: https://en.wikipedia.org/wiki/Hash_chain
- Merkle Trees: https://en.wikipedia.org/wiki/Merkle_tree
