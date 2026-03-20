---
name: self-review-reference
description: "Use when user asks about self-review, output quality gates, response validation, auto-review of assistant responses, LLM output checking, or quality assurance for AI responses. Reference material on auto-reviewing LLM output before or after delivery to the user. Keywords: self-review, quality gate, output validation, response quality, auto-review, pre-delivery check, post-delivery audit"
allowed-tools: "Read"
inclusion: on-demand
---

# Self-Review Quality Gate -- Reference

Reference-only skill documenting self-review patterns for LLM assistant output, with cost-benefit analysis specific to Cairn's streaming architecture and provider pricing.

## Core Problem

LLM assistants produce imperfect output in predictable failure modes:

- **Hallucination**: Stating facts not grounded in tool results or context
- **Incomplete answers**: Addressing part of the question, missing key aspects
- **Tone drift**: Formal when casual was appropriate, or verbose when concise was requested
- **Tool result misuse**: Summarizing tool output incorrectly or ignoring error states
- **Stale context**: Using outdated information when fresh data is available

For autonomous agents (operating without immediate human oversight), these failures compound: an incorrect tool call summary feeds into the next reasoning step, producing cascading errors.

Self-review adds a quality check between generation and delivery (or after delivery) to catch these failure modes.

## Three Self-Review Architectures

### Strategy A: Pre-Delivery Buffer

**Mechanism**: Buffer the complete response, run a review LLM call, optionally revise, then deliver.

```
User Question -> LLM generates full response (buffered, not streamed)
  -> Review LLM evaluates: factual? complete? appropriate?
  -> If pass: deliver to user
  -> If fail: revise and re-deliver (or flag for human review)
```

**Pros**:
- Catches errors before user sees them
- Can revise or suppress bad responses entirely
- Highest quality guarantee

**Cons**:
- Eliminates streaming UX (user waits for full generation + review)
- Adds 2-5s latency per response
- 2x token cost (generation + review)
- Complex error handling (what if review LLM also hallucinates?)

**When appropriate**: Non-streaming APIs, batch processing, high-stakes automated workflows (email send, code deployment), multi-tenant products where error cost is high.

### Strategy B: Inline Self-Critique (Prompt Engineering)

**Mechanism**: Embed quality criteria directly into the system prompt so the LLM self-checks during generation.

```
System prompt includes:
  "Before answering, verify: (1) claims are grounded in tool results,
   (2) all parts of the question are addressed, (3) uncertainty is
   acknowledged rather than fabricated."
```

**Pros**:
- Zero additional LLM calls (no cost increase)
- Zero additional latency
- Compatible with streaming
- Simple to implement (prompt changes only)

**Cons**:
- Same model checking its own work (limited error detection)
- No independent verification
- Effectiveness depends on model capability and prompt quality
- Cannot catch systematic biases the model doesn't recognize

**When appropriate**: Always viable as a baseline. Best for: streaming systems, cost-sensitive deployments, single-user systems with immediate correction loops.

### Strategy C: Selective Post-Delivery Audit

**Mechanism**: After delivering the response, run an asynchronous review. Flag low-quality responses for learning, not correction.

```
User Question -> LLM streams response to user (normal flow)
  -> After delivery: fire-and-forget review LLM call
  -> Review results stored in audit log or proposed as memories
  -> Patterns inform prompt improvements over time
```

**Pros**:
- No UX impact (streaming preserved, no latency added)
- Selective (only review high-stakes or complex responses)
- Feeds learning loop (corrections improve future responses)
- Can use cheaper model for review

**Cons**:
- User already saw the (potentially bad) response
- No real-time correction capability
- Adds background compute cost
- Requires infrastructure to act on audit findings

**When appropriate**: Systems with existing post-response processing (memory extraction, analytics), responses involving multiple tool calls, learning-focused improvement cycles.

## Architecture Comparison

| Factor | A: Pre-Delivery | B: Inline Prompt | C: Post-Delivery |
|--------|-----------------|------------------|-------------------|
| Latency impact | +2-5s | None | None |
| Cost multiplier | ~2x | 1x | ~1.2x (selective) |
| Streaming compatible | No | Yes | Yes |
| Error prevention | High | Low-Medium | None (learning only) |
| Implementation effort | High | Low | Medium |
| Independent verification | Yes | No | Yes |

## Cost-Benefit Analysis

### Per-Provider Cost of Review Pass

Assuming typical review input (~1500 tokens: system prompt + original response) and output (~200 tokens: verdict + rationale):

| Provider | Model | Review Cost | Typical Response Cost | Overhead |
|----------|-------|-------------|----------------------|----------|
| Z.ai GLM | GLM-5 Turbo | $0.000085 | $0.000125 | +68% (~$0) |
| Z.ai GLM | GLM-4.7 | $0.000085 | $0.000125 | +68% (~$0) |

**GLM**: At $0.05/M tokens, self-review cost is negligible. Even reviewing every response adds < $0.01/day at typical usage (50-100 responses/day).

**Bedrock**: At Opus pricing, reviewing every response adds ~56% overhead. Selective review (10-20% of responses) is viable at ~$0.75/day additional.

### Value Assessment for Single-User System

| Factor | Weight | Score | Rationale |
|--------|--------|-------|-----------|
| Error frequency | High | Low | Single user corrects immediately |
| Error cost | High | Low-Medium | No external audience, corrections are cheap |
| Existing quality mechanisms | Med | High | SOUL.md, mode addendums, ReAct, self-improving-agent |
| Marginal improvement | Med | Low | Diminishing returns over existing prompt quality |
| Implementation cost | Med | Low | Strategy B is free, C follows existing patterns |

## Pub-Specific Recommendation

**Primary: Strategy B (Inline Self-Critique)** -- zero cost, zero latency, immediate value.
- Add quality criteria to the system prompt or SOUL.md
- Focus on: grounding claims in tool results, acknowledging uncertainty, matching user communication style from memories

**Secondary: Strategy C (Post-Delivery Audit)** -- for high-stakes responses only.
- Only trigger for: responses with 3+ tool calls, coding/deploy/email contexts, artifact creation
- Follow the existing memory-extractor fire-and-forget pattern
- Store findings as proposed memories for the learning loop

**Not recommended: Strategy A (Pre-Delivery Buffer)** -- breaks streaming UX, which is core to Cairn's real-time interaction model.

**Defer coded implementation.** The inline prompt criteria (Strategy B) and Cairn's existing quality mechanisms (SOUL.md, self-improving-agent skill, ReAct loop) provide sufficient quality for a single-user system. Revisit Strategy C when: response volume exceeds manual review capacity, multi-user deployment is planned, or correction rate data indicates a quality problem.

For detailed implementation blueprints and integration points, see `reference.md` in this directory.

## References

- Madaan et al., "Self-Refine: Iterative Refinement with Self-Feedback" (2023) -- foundational self-review paper
- Shinn et al., "Reflexion: Language Agents with Verbal Reinforcement Learning" (2023) -- post-execution reflection loop
- OpenClaw `self-review` pattern (conceptual basis)
- Cairn's dual-LLM email triage (existing two-pass architecture in `email-triage-runner.ts`)
- Cairn's chain-of-density approach (quality via prompt engineering in `digest-runner.ts`)
