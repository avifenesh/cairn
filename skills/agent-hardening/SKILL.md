---
name: agent-hardening
description: "Use when reviewing agent security, prompt injection defenses, input sanitization, LLM security hardening, or SSRF protection. Keywords: prompt injection, agent hardening, input sanitization, LLM security, SSRF, defense in depth"
inclusion: on-demand
---

# Agent Hardening -- Reference

Reference-only skill documenting security hardening patterns for LLM-powered agent systems.

## Core Problem

LLM-powered agents face a unique threat surface that traditional web applications do not:

- **Untrusted input reaches LLM context** -- emails, webhooks, RSS, agent messages all become prompt content
- **Tools grant real-world side effects** -- shell, git, email, deploy are one tool call away
- **Multi-hop data flows create indirect injection paths** -- external content enters memory, memory enters future prompts

## OWASP LLM Top 10 Mapping

| ID | Threat | Agent Relevance |
|----|--------|-----------------|
| LLM01 | Prompt Injection (direct + indirect) | Untrusted data reaches prompt via feed, email, webhooks, agent messages |
| LLM02 | Insecure Output Handling | Tool results re-injected into LLM context without validation |
| LLM05 | Insecure Plugin Design | Tool arguments not schema-validated; uncapped tool loop iterations |
| LLM07 | SSRF | Web-fetch/URL tools used to probe internal network or cloud metadata |

## Defense-in-Depth Strategies

| Layer | Strategy | What It Guards |
|-------|----------|----------------|
| 1 | Input sanitization at trust boundaries | Strip XML/HTML tags, escape special chars from external content to prevent prompt injection |
| 2 | Privilege separation (dual-LLM quarantine) | Process untrusted content with stripped-down LLM (no tools, no memories), pass only capped factual summaries |
| 3 | Output validation | Validate tool results against schema before re-injecting into LLM context |
| 4 | Schema validation (Zod/JSON Schema) | Reject malformed tool arguments, enforce field types and size limits |
| 5 | Rate limiting and loop guards | Cap tool calls per session, detect repetitive tool invocations, prevent runaway execution |
| 6 | SSRF protection | Block private IPs, loopback, cloud metadata, DNS rebinding; redirect blocking |
| 7 | Environment isolation | Subprocess gets only safe env vars, workspace path restricted, timeout enforced |

## Pattern A: Dual-LLM Quarantine

A quarantine LLM processes untrusted content with NO tools and NO memory context. Its system prompt constrains the model to produce only factual summaries and reject instruction-like content.

- Output is capped (e.g., 200 chars), tag-stripped, and field-validated
- Only the sanitized summary reaches the privileged LLM that has tool access
- Prevents indirect injection from external content (emails, RSS, agent messages)
- The quarantine boundary is the single most important defense against indirect prompt injection

## Pattern B: Tag-Stripping at Trust Boundaries

All external content is tag-stripped before entering prompt context:

- **XML escaping**: converts angle brackets and ampersands to entities -- preserves content structure while neutralizing XML injection
- **Full tag stripping**: removes all `<...>` patterns -- used for feed titles, memory content, LLM input/output
- **Applied at**: memory injection, feed title rendering, skill name/description escaping, conversation context, memory extraction
- **Defends against**: role override attempts, delimiter escapes, system prompt manipulation

## Attack Categories

These are abstract categories -- no working payloads are included:

- **Direct injection**: user crafts input to override system prompt instructions
- **Indirect injection**: adversarial content embedded in external data (email, webpage, RSS) reaches LLM via data pipeline
- **Data exfiltration**: injection causes LLM to leak system prompt, memories, or API keys via tool calls
- **Goal hijacking**: injection redirects agent from assigned task to attacker's objective
- **Privilege escalation**: injection causes agent to use tools beyond intended scope

## References

- OWASP LLM Top 10 (2025 edition)
- Simon Willison's prompt injection research (simonwillison.net)
- Anthropic's responsible AI documentation
- NIST AI 100-2e2023 (Adversarial ML taxonomy)
- Greshake et al. (2023) "Not What You've Signed Up For" -- indirect prompt injection

See `reference.md` for Cairn's defense inventory, gap analysis, and remediation recommendations.
