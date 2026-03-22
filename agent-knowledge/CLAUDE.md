# Agent Knowledge Base

> Research guides for Cairn development. Read the relevant guide before answering questions on these topics.

## Guides (16)

| # | Topic | File | Sources | Created |
|---|-------|------|---------|---------|
| 1 | Session compaction, context window management | session-compaction.md | 37 | 2026-03-19 |
| 2 | Memory extraction from conversations | auto-memory-extraction.md | 64 | 2026-03-19 |
| 3 | Subagent architectures (agent-as-tool, delegation) | subagents.md | 22 | 2026-03-21 |
| 4 | Orchestrator pattern (thin management layer) | orchestrator-pattern.md | 16 | 2026-03-21 |
| 5 | Autonomous quality gates, self-improvement | autonomous-agent-quality.md | 18 | 2026-03-21 |
| 6 | Telegram Bot API, mini apps | telegram-development.md | 6 | 2026-03-21 |
| 7 | Z.ai / GLM API integration | zai-glm-api.md | 21 | 2026-03-18 |
| 8 | AI thinking/reasoning UI patterns | ai-thinking-ui-patterns.md | 24 | 2026-03-18 |
| 9 | Active learning, user adaptation | active-learning-user-adaptation.md | 42 | 2026-03-19 |
| 10 | Agent OS architecture, personality systems | agent-os-personality.md | 42 | 2026-03-19 |
| 11 | Coding environments for AI agents | coding-env-for-agents.md | 42 | 2026-03-19 |
| 12 | Self-evolving agents | self-evolving-agents.md | 42 | 2026-03-19 |
| 13 | Cron systems, agent resilience | cron-and-resilience.md | 108 | 2026-03-19 |
| 14 | File write/edit tools in AI coding assistants | ai-file-edit-tools.md | 42 | 2026-03-21 |
| 15 | Diff viewer library selection, Monaco, CodeMirror, streaming diffs | coding-session-diff-ui.md | 20 | 2026-03-21 |
| 16 | Diff view visual design, CSS, color schemes, diff2html config, virtual scrolling | diff-view-visual-design.md | 22 | 2026-03-21 |

## Keyword Lookup

| Keywords | Guide |
|----------|-------|
| compaction, context overflow, sliding window, truncate, context rot | session-compaction.md |
| chain of density, Anthropic compaction, pause_after_compaction | session-compaction.md |
| MemGPT, Letta, Zep (both guides) | session-compaction.md, auto-memory-extraction.md |
| memory extraction, auto-extract, contradiction detection, confidence | auto-memory-extraction.md |
| Mem0, HippoRAG, entity extraction, memory decay, Ebbinghaus | auto-memory-extraction.md |
| subagent, spawn, delegate, agent-as-tool, two-level max | subagents.md |
| LangGraph supervisor, CrewAI, AutoGen, OpenAI handoffs, A2A | subagents.md |
| orchestrator, supervisor, thin management, idle tick, AIOS | orchestrator-pattern.md |
| autonomous quality, guardrails, self-improvement, Voyager | autonomous-agent-quality.md |
| property-based testing, PBT, permission allowlist, auto-accept | autonomous-agent-quality.md |
| telegram bot, mini app, webhook, inline keyboard | telegram-development.md |
| GLM, Z.ai, zhipu, vision API, web search API | zai-glm-api.md |
| thinking UI, reasoning trace, streaming indicator | ai-thinking-ui-patterns.md |
| active learning, user adaptation, preference learning | active-learning-user-adaptation.md |
| agent personality, SOUL, agent OS, identity | agent-os-personality.md |
| coding environment, sandbox, worktree, container | coding-env-for-agents.md |
| self-evolving, skill discovery, autonomous learning | self-evolving-agents.md |
| cron, scheduled job, resilience, crash recovery | cron-and-resilience.md |
| file edit tool, writeFile, editFile, search-and-replace, apply model | ai-file-edit-tools.md |
| Claude Code Edit, Codex apply_patch, Aider edit formats, Cursor instant apply | ai-file-edit-tools.md |
| Cline replace_in_file, Roo Code apply_diff, diff format, udiff | ai-file-edit-tools.md |
| read-before-write, checkpoint, rollback, fuzzy matching, speculative edits | ai-file-edit-tools.md |
| JSON code wrapping, line number accuracy, lazy code generation | ai-file-edit-tools.md |
| architect mode, editor model, two-model approach, apply role | ai-file-edit-tools.md |
| diff viewer, Monaco DiffEditor, CodeMirror merge, diff2html, jsdiff, react-diff-viewer | coding-session-diff-ui.md |
| streaming diff, live diff, file tree badges, accept/reject hunk, unified vs split | coding-session-diff-ui.md |
| diff CSS, diff colors, dark mode diff, RGBA overlay, d2h-ins, d2h-del | diff-view-visual-design.md |
| diff2html config, colorScheme, synchronisedScroll, matching, outputFormat | diff-view-visual-design.md |
| syntax highlighting diff, Shiki transformerNotationDiff, hljs diff, Prism diff-highlight | diff-view-visual-design.md |
| virtual scroll diff, TanStack Virtual, large diff performance, lazy hunk rendering | diff-view-visual-design.md |
| accessible diff, colorblind diff, WCAG diff, gutter prefix, left border accent | diff-view-visual-design.md |
| diff color palette, GitHub diff colors, VS Code diff colors, word-level highlight | diff-view-visual-design.md |

## How to Use

1. Match user question to keywords above
2. Read the guide file
3. Check "Best Practices" and "Common Pitfalls" sections for actionable advice
4. Check "Further Reading" for authoritative sources
