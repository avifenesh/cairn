---
name: chain-of-density-reference
description: "Reference material on chain-of-density summarization. Use when user asks how to make summaries denser, improve digest quality, apply chain-of-density prompting, or write more concise notification summaries. Keywords: chain of density, densification, summary quality, entity density, digest improvement, concise summaries"
inclusion: on-demand
allowed-tools: "Read"
---


# Chain-of-Density Summarization — Reference

Reference-only skill documenting the chain-of-density technique and its application to Cairn's digest system. Based on the Adams et al. 2023 paper ("From Sparse to Dense: GPT-4 Summarization with Chain of Density Prompting").

## Core Algorithm

Chain-of-density (CoD) generates increasingly entity-dense summaries through iterative refinement:

1. **Pass 1**: Produce a verbose, entity-sparse summary covering main topics
2. **Pass 2-5**: Each pass identifies 1-3 missing salient entities, rewrites the summary to include them without increasing length
3. **Result**: A summary that packs maximum information into minimal space

Key properties of each refinement round:
- Same or shorter length than previous pass
- Adds new entities (names, numbers, dates, events) from the source
- Replaces filler and generic phrases with specific facts
- Maintains readability and grammatical flow

## Densification Rules (for single-pass prompting)

Rather than running multiple LLM passes, embed these rules into a system prompt:

1. **Entity-first**: Lead each item with the most specific identifier (repo name, PR number, person, score)
2. **No filler phrases**: Remove "there was", "it appears that", "it should be noted", "various updates"
3. **Merge co-referent items**: If two events share an entity, combine into one line with a count
4. **Quantify over qualify**: "3 comments by alice, bob" not "several comments from multiple users"
5. **Scale formatting to volume**: <10 groups = full detail per item; 10-30 = grouped with counts; 30+ = category summaries with top items only

## Adaptation for Cairn Digest

Cairn's DigestRunner uses a single LLM pass (Strategy A) with densification principles embedded in the system prompt rather than multi-pass refinement (Strategy B), keeping cost constant.

**Prompt integration points:**
- DIGEST_SYSTEM_PROMPT: Add densification rules section after existing priority/rules sections
- buildDigestPrompt: Inject metadata (group count, event count) so the LLM calibrates detail level

**Metrics to watch:**
- Output token count (should decrease or stay flat for same input volume)
- Entity density (named items per sentence — target: 2-3)
- User satisfaction (qualitative — is the digest scannable and actionable?)

## References

- Adams et al., "From Sparse to Dense: GPT-4 Summarization with Chain of Density Prompting" (2023)
- Applicable patterns: news aggregation, changelog summarization, alert triage

