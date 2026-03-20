# Pub-Specific Adaptation Guide

This file contains detailed implementation blueprints for self-review patterns in Cairn's architecture. See SKILL.md for general concepts and the recommendation summary.

## Current State in Pub

Cairn's assistant pipeline has **no post-generation quality gate**. The response flow is:

1. `runToolLoop()` streams LLM response via SSE deltas (up to 10 tool-use rounds)
2. Response text accumulates in `fullText` variable
3. `finalizeTask()` emits `emitAssistantEnd`, updates DB, triggers memory extraction

Existing quality mechanisms (none are post-generation review):
- **SOUL.md**: Behavioral principles ("quality over quantity") shape generation
- **Mode addendums**: TALK_MODE_ADDENDUM instructs concise responses, WORK_MODE_ADDENDUM instructs structured output
- **ReAct loop**: Structured Thought/Observation/Conclusion reasoning during tool rounds (disabled by default, ~150 tokens/round)
- **ToolLoopGuard**: Structural gate preventing infinite tool loops (max 50 calls, max 15/tool, max 3 identical repeats)
- **Self-improving-agent skill**: Learning loop for error patterns and corrections, not output quality gating
- **Memory extraction**: Post-response LLM call, but extracts knowledge -- does not evaluate quality

## Existing Patterns as Implementation Models

### Model 1: Memory Extractor (Fire-and-Forget Post-Response LLM)

**Location**: `assistant-runner.ts` `finalizeTask()`, calls `MemoryExtractor.extract()`

**Pattern**: After task completion, a secondary LLM call runs asynchronously. Defaults to Sonnet model (configurable via `config.memory.extractModel`). Structured JSON output with confidence scores. Results feed into the memory proposal pipeline.

**Relevance to Strategy C**: Exact architectural pattern. A post-delivery audit would be implemented identically -- another fire-and-forget LLM call in `finalizeTask()`, alongside memory extraction.

### Model 2: Email Triage Dual-LLM (Two-Pass Architecture)

**Location**: `email-triage-runner.ts`

**Pattern**: LLM1 (quarantine, no tools/memories) produces sanitized 200-char summaries. LLM2 (triage, full tools/memories) makes decisions from summaries only. Output from LLM1 is sanitized (tag stripping, length capping, whitespace normalization).

**Relevance to Strategy A**: Demonstrates two-pass LLM architecture in Cairn. The quarantine LLM is analogous to a review LLM: it evaluates content with restricted context. Key difference: email triage is security-motivated (prevent prompt injection), self-review is quality-motivated.

### Model 3: Chain-of-Density (Quality via Prompt Engineering)

**Location**: `digest-runner.ts` DIGEST_SYSTEM_PROMPT

**Pattern**: Instead of multi-pass refinement, densification rules are embedded directly in the system prompt. Single LLM call produces high-quality output by following explicit quality criteria.

**Relevance to Strategy B**: Exact approach. Quality criteria embedded in the generation prompt are cheaper and faster than post-hoc review. Proven effective in Cairn's digest system.

### Model 4: ToolLoopGuard (Structural Quality Gate)

**Location**: `tools/loop-guard.ts`

**Pattern**: Deterministic checks (call counts, arg hashing, repetition detection) prevent degenerate tool-loop behavior. Zero LLM cost.

**Relevance**: Demonstrates that some quality issues can be caught with heuristics rather than LLM review. For self-review, structural checks (response length, tool result coverage) could supplement or replace LLM-based review.

## Strategy B Blueprint: Inline Self-Critique

**Integration point**: System prompt construction in `assistant-runner.ts` `buildSystemPrompt()` method, or as additions to `SOUL.md`.

**Proposed quality criteria to embed**:

```
Before finalizing your response, mentally verify:
1. Every factual claim is grounded in a tool result from this conversation
2. All parts of the user's question are addressed
3. If a tool call returned an error, the error is acknowledged (not silently ignored)
4. Uncertainty is stated explicitly ("I'm not sure" / "Based on available data")
5. Response length matches the complexity of the question
```

**Implementation**: Add to SOUL.md quality principles section or as a skill with `inclusion: always`. No code changes needed.

**Effectiveness measurement**: Track user correction rate (messages containing "no", "wrong", "actually", "that's not right" following an assistant response). Compare before/after adding criteria.

## Strategy C Blueprint: Selective Post-Delivery Audit

**Integration point**: `assistant-runner.ts` `finalizeTask()`, after the existing memory extraction call.

**Trigger criteria** (only audit responses matching any of these):
- Response involved 3+ tool calls (complex multi-step operations)
- Response is in a high-stakes context: coding delegation, deploy, email draft, PR creation
- Response exceeds 1000 characters (long-form output has more surface area for errors)
- User explicitly requested a fact-check or verification

**Review prompt** (for the audit LLM call):

```
Review this assistant response for quality issues.

User question: {userMessage}
Tool results used: {toolResultSummaries}
Assistant response: {fullText}

Evaluate:
1. GROUNDING: Are claims supported by the tool results? Flag any unsupported assertions.
2. COMPLETENESS: Does the response address all aspects of the question?
3. TOOL_ERRORS: Were any tool errors silently ignored?
4. TONE: Is the response appropriately concise/detailed for the question?

Output JSON:
{
  "quality": "good" | "acceptable" | "poor",
  "issues": [{"type": "grounding|completeness|tool_error|tone", "description": "..."}],
  "suggestion": "Brief improvement suggestion if quality != good"
}
```

**Action on findings**:
- `quality: "good"` -- no action, log for metrics
- `quality: "acceptable"` -- propose memory with the improvement suggestion
- `quality: "poor"` -- propose memory + increment a quality_issues counter for monitoring

**Cost**: ~1500 input tokens + ~200 output tokens per audit. At GLM pricing: $0.000085/audit. At 20% trigger rate and 100 responses/day: ~$0.0017/day.

## Selective Trigger Criteria

### Audit (Strategy C) -- worth the cost

| Trigger | Rationale |
|---------|-----------|
| 3+ tool calls in response | Multi-step operations have higher error surface |
| Coding/deploy/email context | High-stakes, hard to undo |
| Artifact creation | User-facing document, quality matters |
| Response > 1000 chars | More content = more potential issues |

### Skip -- not worth auditing

| Skip Pattern | Rationale |
|--------------|-----------|
| Simple acknowledgments | "Done", "Got it" -- no quality risk |
| Clarifying questions | Assistant asking user, not asserting facts |
| Status checks | Short, factual, low-risk |
| Greetings/chat | Conversational, no factual claims |

## Measuring Effectiveness

If self-review is implemented, track these metrics:

| Metric | Measurement | Baseline |
|--------|-------------|----------|
| Correction rate | Messages with correction keywords / total responses | Measure for 2 weeks before implementing |
| Tool error follow-through | Audit findings of type "tool_error" / total audits | Should decrease over time |
| Response quality score | Average audit quality scores | Track trend |
| Memory proposal rate from audits | Audit-generated memories / total audits | Should decrease as quality improves |
| Cost per audit | Actual token usage * provider rate | Compare to budget |

## When to Implement vs Defer

### Implement now (zero cost)
- Strategy B inline criteria via SOUL.md additions
- Structural checks: verify tool results are referenced in response

### Implement when needed
- Strategy C post-delivery audit when:
  - Correction rate data shows a quality problem (>10% of responses corrected)
  - Multi-user deployment is planned (higher cost of error)
  - Response volume exceeds manual review (>500/day)
  - Autonomous operation increases (idle mode doing more unsupervised work)

### Likely never implement
- Strategy A pre-delivery buffer: streaming UX is fundamental to Cairn's interaction model. Only reconsider if Cairn adds a non-streaming API mode (batch processing, scheduled reports).
