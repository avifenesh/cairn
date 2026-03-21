# Agent Knowledge Base

> Learning guides synthesized from local research repos and Cairn internals.
> Reference these when answering questions about listed topics.

## Available Topics

| Topic | File | Sources | Depth | Created |
|-------|------|---------|-------|---------|
| LLM Session Compaction and Conversation Summarization | session-compaction.md | 7 local + 30 web | deep | 2026-03-19 |
| Automatic Memory Extraction from LLM Conversations | auto-memory-extraction.md | 22 local + 42 web | deep | 2026-03-19 |
| Telegram Bot API & Mini Apps Development | telegram-development.md | 6 official | deep | 2026-03-21 |
| Subagent Architectures for AI Agents | subagents.md | 22 web | deep | 2026-03-21 |
| Orchestrator Agent Pattern for Autonomous Systems | orchestrator-pattern.md | 16 web | deep | 2026-03-21 |
| Autonomous Agent Self-Improvement with Quality Gates | autonomous-agent-quality.md | 18 web | medium | 2026-03-21 |

## Trigger Phrases

Use this knowledge when the user asks about:

- "session compaction" → session-compaction.md
- "context window overflow" → session-compaction.md
- "conversation summarization" → session-compaction.md
- "context limit" → session-compaction.md
- "token budget" or "token limit" for conversations → session-compaction.md
- "truncate tool output" → session-compaction.md
- "sliding window" for messages → session-compaction.md
- "History() grows too large" → session-compaction.md
- "compress messages" → session-compaction.md
- "prune conversation" → session-compaction.md
- "Plandex summarization" → session-compaction.md
- "Eino summarization" → session-compaction.md
- "Gollem autocontext" → session-compaction.md
- "orphaned tool results" → session-compaction.md
- "context rot" → session-compaction.md
- "chain of density" → session-compaction.md
- "ConversationSummaryBufferMemory" → session-compaction.md
- "MemGPT" or "Letta memory" → session-compaction.md AND auto-memory-extraction.md
- "Zep memory" → session-compaction.md AND auto-memory-extraction.md
- "Anthropic compaction API" → session-compaction.md
- "compact_20260112" → session-compaction.md
- "pause_after_compaction" → session-compaction.md
- "LangChain memory types" → session-compaction.md AND auto-memory-extraction.md
- "LlamaIndex ChatMemoryBuffer" → session-compaction.md
- "temporal knowledge graph memory" → session-compaction.md AND auto-memory-extraction.md
- "structured distillation agent memory" → session-compaction.md
- "adaptive focus memory" → session-compaction.md
- "How does memory extraction work?" → auto-memory-extraction.md
- "How do I implement memory for an LLM agent?" → auto-memory-extraction.md
- "MemGPT architecture" → auto-memory-extraction.md
- "Zep memory system" → auto-memory-extraction.md
- "Mem0 how it works" → auto-memory-extraction.md
- "LangGraph memory patterns" → auto-memory-extraction.md
- "contradiction detection in memory" → auto-memory-extraction.md
- "memory confidence scoring" → auto-memory-extraction.md
- "Ebbinghaus forgetting curve LLM" → auto-memory-extraction.md
- "entity extraction from conversations" → auto-memory-extraction.md
- "memory prompt engineering" → auto-memory-extraction.md
- "generative agents memory stream" → auto-memory-extraction.md
- "how ChatGPT memory works" → auto-memory-extraction.md
- "Cairn memory gaps" → auto-memory-extraction.md
- "ADD/UPDATE/DELETE memory actions" → auto-memory-extraction.md
- "HippoRAG knowledge graph retrieval" → auto-memory-extraction.md
- "subagent" or "sub-agent" → subagents.md
- "spawn agent" or "delegate to agent" → subagents.md
- "agent-as-tool" → subagents.md
- "two-level max" or "no grandchildren" → subagents.md
- "context isolation" for agents → subagents.md
- "orchestrator" or "supervisor agent" → orchestrator-pattern.md
- "thin management layer" → orchestrator-pattern.md
- "idle tick replacement" → orchestrator-pattern.md
- "AIOS" or "agent operating system kernel" → orchestrator-pattern.md
- "autonomous quality" or "self-improvement" → autonomous-agent-quality.md
- "guardrails for agents" → autonomous-agent-quality.md
- "property-based testing LLM" → autonomous-agent-quality.md
- "memory auto-accept" → autonomous-agent-quality.md
- "permission allowlist" → autonomous-agent-quality.md
- "Voyager skill library" → autonomous-agent-quality.md

## Quick Lookup

| Keyword | Guide |
|---------|-------|
| compaction, compact | session-compaction.md |
| summarize conversation | session-compaction.md |
| context overflow | session-compaction.md |
| truncate | session-compaction.md |
| sliding window | session-compaction.md |
| token budget messages | session-compaction.md |
| stripOrphanedToolResults | session-compaction.md |
| context rot | session-compaction.md |
| chain of density | session-compaction.md |
| MemGPT hierarchical memory | session-compaction.md, auto-memory-extraction.md |
| Zep entity extraction | session-compaction.md, auto-memory-extraction.md |
| Anthropic compaction beta | session-compaction.md |
| memory extraction, LLM memory, agent memory | auto-memory-extraction.md |
| MemGPT, Letta, core memory, archival memory | auto-memory-extraction.md |
| Zep, Graphiti, temporal knowledge graph | auto-memory-extraction.md |
| Mem0, ADD/UPDATE/DELETE/NONE | auto-memory-extraction.md |
| LangGraph store, cross-thread memory | auto-memory-extraction.md |
| Ebbinghaus, memory decay, forgetting curve | auto-memory-extraction.md |
| HippoRAG, entity extraction | auto-memory-extraction.md |
| generative agents, memory stream, reflection | auto-memory-extraction.md |
| contradiction resolution, fact invalidation | auto-memory-extraction.md |
| memory confidence, poignancy score | auto-memory-extraction.md |
| proposition extraction, atomic facts | auto-memory-extraction.md |
| subagent, spawn, delegate, child agent | subagents.md |
| agent-as-tool, two-level max, worktree isolation | subagents.md |
| LangGraph supervisor, CrewAI, AutoGen, OpenAI handoffs | subagents.md |
| orchestrator, supervisor, management layer | orchestrator-pattern.md |
| idle tick, thin orchestrator, system state scan | orchestrator-pattern.md |
| AIOS, agent kernel, agent scheduler | orchestrator-pattern.md |
| autonomous quality, guardrails, self-improvement | autonomous-agent-quality.md |
| property-based testing, PBT, self-verification | autonomous-agent-quality.md |
| Voyager, skill library, auto-accept, permission model | autonomous-agent-quality.md |

## How to Use

1. Check if user question matches a topic above
2. Read the relevant guide file
3. For Cairn-specific implementation questions, check "Implementation Roadmap" and "Gap Analysis" sections in auto-memory-extraction.md
4. For theoretical/research questions, check the "Web Research" section in the relevant guide
5. Cite specific source files when the user asks for implementation details
