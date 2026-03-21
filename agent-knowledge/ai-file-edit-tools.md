# Learning Guide: File Write/Edit Tools in AI Coding Assistants

**Generated**: 2026-03-21
**Sources**: 42 resources analyzed
**Depth**: deep

## Prerequisites

- Familiarity with how LLM-based coding agents work (tool use, agentic loops)
- Basic understanding of diff/patch formats (unified diff, search-replace)
- Experience with at least one AI coding assistant
- Understanding of file system operations and version control

## TL;DR

- AI coding assistants use four main strategies for file editing: **whole-file replacement**, **search-and-replace blocks**, **custom patch/diff formats**, and **dedicated apply models** that rewrite files from a chat suggestion.
- The **search-and-replace** pattern (Claude Code Edit, Aider diff, Cline replace_in_file) has emerged as the most common approach, balancing token efficiency with edit reliability.
- LLMs are bad at generating **line numbers** and **JSON-escaped code**; the best tools avoid both.
- **Cursor's key insight**: a small fine-tuned model that rewrites the full file at ~1000 tok/s with speculative decoding outperforms asking the frontier model to produce diffs.
- **Read-before-write** enforcement (Claude Code, Roo Code) is the single most effective guard against clobbering existing content.

---

## Core Concepts

### The Fundamental Problem

An LLM generates text. Source files live on disk. The "file edit tool" bridges the gap: it must translate the model's intent ("change function X to do Y") into a precise, conflict-free mutation of bytes on the filesystem. This is harder than it sounds because:

1. **Token cost**: Returning a full 2000-line file to change one line is wasteful.
2. **Accuracy**: LLMs struggle with exact line numbers, proper indentation, and escaping.
3. **Ambiguity**: A search string might match multiple locations in a file.
4. **Concurrency**: The file may have changed between read and write.
5. **Streaming**: Edits often need to apply while the model is still generating.

### The Four Strategies

| Strategy | Description | Used By |
|----------|-------------|---------|
| **Whole-file replacement** | Model outputs the complete new file | Aider (whole format), Roo Code (write_to_file), Claude Code (Write) |
| **Search-and-replace blocks** | Model specifies old text and new text | Claude Code (Edit), Aider (diff format), Cline/Roo Code (apply_diff/replace_in_file) |
| **Custom patch format** | A simplified diff format purpose-built for LLMs | OpenAI Codex CLI (apply_patch), Aider (udiff) |
| **Apply model** | A separate fine-tuned model rewrites the file from a chat suggestion | Cursor (instant apply), Continue.dev (apply role), Morph, Relace |

---

## Tool-by-Tool Analysis

### 1. Claude Code (Anthropic)

**Edit Strategy**: Exact string search-and-replace

Claude Code provides three file operation tools:

| Tool | Purpose | Permission |
|------|---------|------------|
| `Read` | Read file contents (with optional line offset/limit) | No |
| `Edit` | Targeted search-and-replace edits on specific files | Yes |
| `Write` | Create or completely overwrite files | Yes |

**Edit Tool Details**:
- Uses **exact string matching**: you provide the old text and the new text.
- The old_string must match precisely (including whitespace and indentation).
- A `replace_all` flag can replace all occurrences of the old string.
- **Read-before-Edit requirement**: The Write tool will fail if you have not first used Read on that file. This prevents the model from blindly overwriting content it has never seen.
- For creating new files, Write is used. For targeted changes, Edit is preferred.
- MultiEdit exists for Jupyter notebooks (NotebookEdit).

**Undo/Rollback**:
- Automatic checkpointing: every user prompt creates a checkpoint of affected files.
- `Esc + Esc` or `/rewind` opens a rewind menu to restore code, conversation, or both.
- Checkpoints persist across sessions (30-day retention).
- Limitations: does not track changes made by Bash commands (e.g., `rm`, `mv`).

**Large File Handling**:
- Read tool supports `offset` and `limit` parameters for reading portions of large files.
- For PDFs over 10 pages, a `pages` parameter is required.
- Context window management via auto-compaction when approaching limits.

**Unique Innovations**:
- Hooks: deterministic shell commands that run before/after every file edit (e.g., auto-format, lint).
- LSP integration: automatic type error/warning reporting after file edits.
- Subagent isolation: delegated tasks run in separate context windows.

**Source**: Claude Code Tools Reference, Best Practices docs, How Claude Code Works docs.

---

### 2. OpenAI Codex CLI

**Edit Strategy**: Custom patch format (`apply_patch`)

Codex CLI uses a purpose-built patch format that is a simplified, LLM-friendly variant of unified diff.

**Patch Format** (`apply_patch`):
```
*** Begin Patch
*** Add File: <path>
+line1
+line2
*** Update File: <path>
@@ optional_context_header
 context line
-removed line
+added line
*** Delete File: <path>
*** End Patch
```

**Key Design Decisions**:
- File paths must be relative (never absolute).
- Default 3 lines of context before/after changes.
- `@@` headers can include class/function names for disambiguation.
- Supports file operations: Add, Update, Delete, and Move (rename).
- Fuzzy matching with Unicode normalization during `seek_sequence`.
- Replacements applied in reverse order to preserve indices.

**Sandbox Architecture**:
- Three approval modes: `suggest` (show diffs), `auto-edit` (apply patches automatically), `full-auto` (complete autonomy).
- macOS: Apple Seatbelt (`sandbox-exec`) with read-only filesystem except `$PWD`, `$TMPDIR`, `~/.codex`.
- Linux: Bubblewrap (`bwrap`) or Docker with iptables firewall; network blocked except OpenAI API.
- `.git` directories protected as read-only.

**Undo/Rollback**:
- In `suggest` mode, diffs are presented for manual approval.
- Warns on non-Git directories (no safety net without VCS).
- Git-based rollback is the primary recovery mechanism.

**Source**: Codex CLI README, apply-patch source code (Rust), apply_patch_tool_instructions.md.

---

### 3. Aider

**Edit Strategy**: Multiple formats, model-adaptive

Aider is unique in offering **six distinct edit formats**, each optimized for different LLM capabilities:

| Format | How It Works | Best For |
|--------|-------------|----------|
| `whole` | LLM returns complete updated file | Simple models, small files |
| `diff` | SEARCH/REPLACE blocks (git merge conflict style) | GPT-4o, Claude 3.5 Sonnet |
| `diff-fenced` | Like diff but path inside fence | Gemini models |
| `udiff` | Simplified unified diff | GPT-4 Turbo (reduces laziness) |
| `editor-diff` | Streamlined diff for architect mode | Editor model in two-model setup |
| `editor-whole` | Streamlined whole for architect mode | Editor model in two-model setup |

**SEARCH/REPLACE Block Syntax** (diff format):
```
path/to/file.py
<<<<<<< SEARCH
def old_function():
    return "old"
=======
def new_function():
    return "new"
>>>>>>> REPLACE
```

**Key Technical Findings**:
- **JSON wrapping degrades performance**: All models performed worse when asked to return code in JSON. Claude 3.5 Sonnet and DeepSeek Coder were most severely impacted. Even avoiding syntax errors, JSON "cognitive burden" distracted from problem-solving.
- **Unified diffs tripled benchmark scores**: GPT-4 Turbo went from 20% to 61% on refactoring benchmarks when switching from SEARCH/REPLACE to udiff format.
- **Flexible patching is critical**: Disabling fuzzy patching caused a 9x increase in editing errors.
- **High-level diff prompting**: Asking for whole-function edits rather than line-level changes reduced errors by 30-50%.
- **Laziness reduction**: udiff format reduced "lazy comments" (e.g., `// ... rest of code ...`) from 12 to 4 benchmark tasks.

**Architect Mode** (Two-Model Approach):
- **Architect model** (e.g., o1-preview, GPT-4o): describes how to solve the problem in natural language.
- **Editor model** (e.g., DeepSeek, o1-mini, Claude Sonnet): translates solution into properly formatted code edits.
- Achieves 85% pass rate on editing benchmarks (vs ~70% single-model).
- Decouples reasoning ability from formatting compliance.

**Error Handling**:
- Reports "Failed to apply edit to <filename>" on malformed edits.
- Makes "every effort" to handle edits that are "almost" correctly formatted.
- Weaker models are more prone to format violations.
- Recommends switching to `--edit-format whole` or `--architect` for unreliable models.

**Undo**: Git-based. `/undo` reverts the last aider-made commit. Auto-commits each change.

**Source**: Aider edit formats docs, unified diffs blog post, code-in-json blog post, architect blog post, SWE-bench results.

---

### 4. Cursor

**Edit Strategy**: Dedicated apply model (full-file rewrite)

Cursor's approach is architecturally distinct: rather than having the frontier model produce edits directly, it uses a **separate fine-tuned model** to rewrite the entire file.

**Two-Stage Architecture**:
1. **Chat/Planning Stage**: Frontier model (Claude, GPT-4, etc.) discusses changes in natural language or produces code blocks.
2. **Apply Stage**: A fine-tuned Llama-3-70B model takes the original file + chat context and outputs the complete new file.

**Why Full-File Rewrite Over Diffs**:
- **More forward passes**: Full output gives the model more tokens to "think through" the correct placement.
- **Training distribution**: Models see far more complete code files than diff formats in pre-training.
- **Line number failure**: Models reliably fail at counting and specifying line numbers; full rewrite sidesteps this.
- When they tested diff approaches (using Aider's SEARCH/REPLACE format), only Claude Opus reliably generated accurate diffs.

**Speculative Edits** (Key Innovation):
- Custom speculative decoding algorithm for code edits.
- Instead of using a draft model, exploits the fact that most of the output file is identical to the input.
- Deterministically speculates that the next tokens will be the same as the original file.
- Produces **4-5x speedup** beyond the fine-tuned model baseline.
- Result: ~1000 tokens/second (~3,500 chars/second).
- **13x faster** than standard Llama-3-70B inference.
- **9x faster** than their previous GPT-4 implementation.

**Model Training**:
- Fine-tuned Llama-3-70B and DeepSeek Coder on synthetic data.
- 80/20 mix of real "fast-apply" examples and cmd-k prompts.
- Downsampled small files, duplicates, and no-op edits.
- Nearly matches Claude-3-Opus accuracy while being much faster.

**Undo/Rollback**: Automatic checkpoints before significant changes. Users can preview and restore previous codebase states.

**Source**: Cursor "Instant Apply" blog post, Cursor Agent documentation.

---

### 5. Windsurf (Codeium) / Cascade

**Edit Strategy**: Direct file editing in Code mode, proposal-based in Chat mode

Windsurf's Cascade agent operates in two modes with different editing behaviors:

| Mode | Editing Behavior |
|------|-----------------|
| **Code Mode** | Directly creates and modifies files in the codebase |
| **Chat Mode** | Proposes code that users manually accept and insert |

**Key Features**:
- **Automatic linter integration**: Cascade auto-fixes linting errors on generated code (enabled by default). Lint fixes are "free of credit charge."
- **Checkpoint/Revert system**: Named snapshots allow reverting Cascade's modifications. Reverts are currently irreversible.
- **Tool calling**: Uses Search, Analyze, Web Search, MCP, and terminal tools.
- **Dependency detection**: Automatically detects required packages and suggests installation.

**Limitations**: Windsurf's public documentation does not disclose the specific diff format or internal edit mechanism used by Cascade. The system is closed-source and the apply method is proprietary.

**Source**: Windsurf Cascade documentation.

---

### 6. Cline / Roo Code

These are closely related projects (Roo Code forked from Cline). Both are VS Code extensions using a similar tool architecture.

#### Cline

**Edit Strategy**: SEARCH/REPLACE blocks with tiered matching

**Tool: replace_in_file** (primary edit tool)
Uses a three-marker format:
```
------- SEARCH
content to find
=======
replacement content
+++++++ REPLACE
```

**Tiered Matching Strategy** (fallback chain):
1. **Exact match**: Direct string search in original file.
2. **Line-trimmed match**: Line-by-line comparison ignoring leading/trailing whitespace.
3. **Block anchor match**: For blocks with 3+ lines, matches using first and last lines as anchors.

**Special Behaviors**:
- Empty SEARCH block = entire file replacement.
- Incremental/streaming processing: chunks processed sequentially during generation.
- Partial marker handling: incomplete markers at chunk boundaries are cleaned up.
- Out-of-order edit detection for replacements at positions earlier than previously processed.

**Tool: write_to_file**: Creates new files or completely overwrites existing ones.

**Undo**: Changes recorded in VS Code's Timeline; provides audit trail and rollback.

**Source**: Cline diff.ts source code, Cline README.

#### Roo Code

**Edit Strategy**: Fuzzy-matched SEARCH/REPLACE with line hints

**Tool: apply_diff** (primary edit tool)
```
<<<<<<< SEARCH:start_line:X:end_line:Y
original content
=======
replacement content
>>>>>>> REPLACE
```

Parameters:
- `path` (required): File path relative to cwd.
- `diff` (required): SEARCH/REPLACE block(s).
- `start_line` (optional hint): Where search content begins.
- `end_line` (optional hint): Where search content ends.

**Matching Strategy**:
- **Fuzzy matching**: Levenshtein distance on normalized strings.
- **Guided by start_line hint**: Middle-out search within configurable `BUFFER_LINES` window (default 40 lines).
- **Configurable confidence threshold**: Typically 0.8-1.0.
- **Consecutive error tracking**: `consecutiveMistakeCountForApplyDiff` prevents repeated failures.

**Tool: write_to_file**:
Parameters: `path`, `content`, `line_count`.
- `line_count` validates against truncation.
- "Much slower and less efficient than apply_diff for modifying existing files."
- Checks `.rooignore` restrictions and workspace boundaries.
- Preprocesses content to remove code block markers and escaped HTML.

**Undo**: Diff preview before application; VS Code Timeline for audit trail.

**Source**: Roo Code documentation (available-tools pages).

---

### 7. Continue.dev

**Edit Strategy**: Apply model with AST-based deterministic matching + LLM fallback

Continue.dev separates the "suggest changes" step from the "apply changes" step:

**Apply Role Architecture**:
- A dedicated model generates precise diffs from chat suggestions.
- Recommended models: Morph Fast Apply, Relace Instant Apply (both have free tiers).
- Most chat models can also serve as apply models (e.g., Claude 3.5 Haiku).
- Customizable via Handlebars templates: `{{{original_code}}}` and `{{{new_code}}}`.

**Deterministic Apply** (Tree-sitter based):
- `deterministicApplyLazyEdit()` handles "lazy blocks" -- comments like `// ... existing code ...`.
- Uses tree-sitter AST parsing to match corresponding nodes between old and new structures.
- Myers diff-like algorithm where lazy blocks consume all nodes until the next match.
- Reconstructs the file by splicing original nodes back into position.
- **Validation**: Rejects if removals exceed 30% of file content; falls back to LLM-based apply.

**Source**: Continue.dev apply model docs, deterministic.ts source code.

---

### 8. Amazon Q Developer

**Edit Strategy**: IDE-integrated code transformation with diff preview

Amazon Q Developer focuses on IDE-based code assistance rather than CLI-based agentic editing.

**Code Transformation Features**:
- Primarily targets Java version upgrades and dependency migrations.
- Three-stage process: initial build -> transformation -> verification.
- "Minimal changes necessary" philosophy for compatibility upgrades.
- Diff view for reviewing changes before application.
- Supports dependency upgrade YAML files for specifying target versions.

**IDE Integration**:
- JetBrains: module-based, diff view for review.
- VS Code: project/workspace-based, dedicated tab for proposed changes.
- Inline code completions and smart actions (right-click refactoring).

**Agentic Capabilities** (newer):
- `/dev` command for multi-file feature implementation.
- Generates implementation plans, then produces code changes.
- Amazon Q scored 20.3% on SWE-bench Lite (pre-Aider's result).

**Source**: AWS documentation on code transformation, Amazon Q Developer overview.

---

### 9. Google Gemini Code Assist / Jules

**Edit Strategy**: IDE inline suggestions, cloud-based agentic editing (Jules)

**Gemini Code Assist** (IDE extension):
- Inline code completions as you type.
- Full function/block generation from comments.
- Smart actions via right-click context menu.
- Chat-based modification requests.
- All suggestions require developer review and validation.

**Google Jules** (Agentic):
- Cloud-based AI coding agent (runs in Google's infrastructure).
- Works on GitHub repos: creates branches, makes changes, opens PRs.
- Integrates with Gemini models.
- Limited public documentation on internal edit mechanics.
- Operates asynchronously; returns results as GitHub PRs.

**Source**: Google Cloud Gemini Code Assist docs, Google Jules announcements.

---

### 10. OpenCode

**Edit Strategy**: Claude Code-inspired tools (archived, succeeded by Crush)

OpenCode (now archived, moved to Charm's "Crush") was a Go-based terminal AI assistant:

**Key Features**:
- File searching, modification, and command execution.
- "AI can execute commands, search files, and modify code."
- File change tracking during sessions.
- LSP integration for code intelligence.
- MCP server support.
- SQLite for session/conversation storage.
- Permission system requiring user approval for file modifications.

**Tools**: Specific tool schemas were not publicly documented before archival. The tool system was similar to Claude Code's approach based on the Go implementation structure.

**Source**: OpenCode README, OpenCode website.

---

## Comparative Analysis

### Edit Strategy Comparison

| Tool | Strategy | Token Efficiency | Accuracy | Streaming | Large Files |
|------|----------|-----------------|----------|-----------|-------------|
| Claude Code Edit | Exact search-replace | High | High (exact match) | Yes | offset/limit Read |
| Codex apply_patch | Custom patch format | High | High (fuzzy seek) | No (sandbox) | Context chunks |
| Aider diff | SEARCH/REPLACE blocks | High | Medium-High | Yes | Repo map |
| Aider whole | Full file output | Low | High | Yes | Poor (full output) |
| Aider udiff | Simplified unified diff | High | High (GPT-4T) | Yes | Good |
| Cursor instant apply | Full file rewrite (apply model) | Low (but fast) | Very High | Speculative | Up to 400 lines trained |
| Cline replace_in_file | SEARCH/REPLACE (tiered) | High | Medium-High | Yes (streaming) | Anchor match |
| Roo Code apply_diff | SEARCH/REPLACE (fuzzy+hints) | High | High (Levenshtein) | Yes | Line hints |
| Continue.dev | AST-based deterministic | Medium | High | No | 30% threshold |

### Read-Before-Write Requirements

| Tool | Requirement | Enforcement |
|------|-------------|-------------|
| Claude Code | Write tool fails if Read not called first | Hard enforcement |
| Roo Code | write_to_file requires `line_count` validation | Soft (truncation detection) |
| Aider | Files must be `/add`-ed to chat context | Hard (context gating) |
| Codex CLI | Sandbox restricts file access by approval mode | Hard (OS-level) |
| Cursor | No explicit requirement (apply model handles) | N/A |
| Cline | User approval before each edit | Interactive gate |

### Undo/Rollback Capabilities

| Tool | Mechanism | Granularity | Persistence |
|------|-----------|-------------|-------------|
| Claude Code | Automatic checkpoints (per-prompt) | Per file edit | 30 days |
| Codex CLI | Git-based (suggest mode shows diffs) | Per session | Git history |
| Aider | Git auto-commit + `/undo` | Per LLM response | Git history |
| Cursor | Automatic checkpoints | Per agent action | Session |
| Windsurf | Named checkpoint/revert | Per Cascade action | Session |
| Cline | VS Code Timeline | Per edit | VS Code |
| Roo Code | Diff preview + VS Code Timeline | Per edit | VS Code |
| Continue.dev | Editor undo | Standard editor | Editor |

---

## Common Failure Modes

### 1. Indentation Mismatch
The most common edit failure. LLMs frequently:
- Mix tabs and spaces
- Use wrong indentation depth (especially in Python)
- Strip leading whitespace in SEARCH blocks

**Mitigations**: Fuzzy matching (Roo Code Levenshtein), line-trimmed matching (Cline tier 2), whitespace normalization (Aider).

### 2. Partial/Ambiguous Matches
The SEARCH string matches multiple locations in the file.

**Mitigations**:
- Claude Code: fails with error; requires more context in the search string.
- Roo Code: line hint (`start_line`) narrows search window.
- Cline: anchor matching (first + last lines).
- Aider: includes surrounding context lines.

### 3. Lazy Code Generation
LLMs replace code with placeholder comments like `// ... rest of implementation ...`.

**Mitigations**:
- Aider udiff format reduced laziness from 12 to 4 tasks.
- Architect mode separates reasoning from formatting.
- Cursor's apply model ignores lazy blocks since it rewrites the full file.
- Continue.dev's deterministic apply uses AST matching to handle lazy blocks.

### 4. JSON Escaping Errors
When code is wrapped in JSON (for structured tool calls), escape sequences break.

**Key Finding** (Aider): All models performed worse with JSON-wrapped code. Claude 3.5 Sonnet showed degraded problem-solving even without syntax errors, suggesting cognitive overhead from JSON formatting.

**Mitigations**: Use plain text / markdown fencing (Aider), avoid JSON tool schemas for code output.

### 5. Line Number Inaccuracy
LLMs are unreliable at counting and specifying line numbers.

**Key Finding** (Cursor): "Outputting specific line numbers on a single token is unreliable." This is why Cursor chose full-file rewrite over line-numbered diffs.

**Mitigations**:
- Avoid line numbers entirely (Claude Code exact match, Aider SEARCH/REPLACE).
- Use line numbers only as hints (Roo Code `start_line`).
- Full-file rewrite (Cursor apply model).

### 6. Large File Failures
Files over ~500 lines cause problems for all tools:
- Whole-file approaches: token budget exceeded.
- Search-replace: SEARCH string may not be unique.
- Cursor: trained on files up to 400 lines (longer files are a planned improvement).

**Mitigations**:
- Claude Code: Read with offset/limit, then targeted Edit.
- Aider: repo map with tree-sitter to identify relevant sections.
- Roo Code: buffer window around start_line hint.

### 7. Binary File Corruption
Most tools assume text files and will corrupt binary files.

**Mitigations**: Most tools detect binary files and refuse to edit. Claude Code's Read tool can display images as visual content.

### 8. Concurrent Edit Conflicts
When multiple agents or the developer edit the same file simultaneously.

**Mitigations**:
- Claude Code: checkpoints enable rollback; worktrees isolate parallel sessions.
- Codex CLI: sandbox filesystem isolation.
- Git-based workflows provide merge conflict resolution.

---

## Best Practices That Have Emerged

### 1. Exact String Match > Line Numbers > Regex
The industry has converged on exact string matching as the most reliable edit primitive. Line numbers fail due to LLM counting errors. Regex is rarely used (too complex for LLMs to generate reliably).

### 2. Read-Before-Write Is Essential
Claude Code's enforcement of reading before writing prevents the most destructive failure mode: blindly overwriting content the model has never seen.

### 3. Two-Model Architecture Works
Both Aider's architect mode and Cursor's apply model demonstrate that separating "what to change" from "how to format the change" produces better results. The reasoning model focuses on problem-solving; the editor/apply model focuses on correct formatting.

### 4. Fuzzy Matching with Confidence Thresholds
Pure exact matching is too brittle (fails on whitespace differences). Pure fuzzy matching is too loose (wrong location). The sweet spot is fuzzy matching with a configurable confidence threshold (Roo Code: 0.8-1.0).

### 5. Git Integration for Safety
Every successful tool integrates with git: auto-commits (Aider), checkpoints (Claude Code, Cursor), or VCS-gated approval modes (Codex CLI).

### 6. Avoid JSON for Code Transport
Aider's research conclusively shows that wrapping code in JSON degrades both syntax accuracy and reasoning quality. The best tools use markdown fencing or plain text.

### 7. Lint-After-Edit Hooks
Windsurf's auto-lint-fix and Claude Code's hook system demonstrate that running linters/formatters automatically after every edit catches many LLM-introduced style issues for free.

### 8. Streaming Edits for Responsiveness
Cline and Roo Code's ability to process SEARCH/REPLACE blocks as they stream from the model provides a much more responsive editing experience than waiting for the full response.

---

## Performance Comparisons

### Aider Edit Format Benchmarks (89-task refactoring)

| Format | GPT-4 Turbo Score | GPT-4 (June) Score |
|--------|-------------------|---------------------|
| SEARCH/REPLACE | 20% | 26% |
| Unified diff | 61% | 59% |
| Improvement | **3x** | **2.3x** |

### Aider Architect Mode Benchmarks (editing benchmark)

| Configuration | Pass Rate |
|---------------|-----------|
| o1-preview + DeepSeek editor (whole) | 85% |
| o1-preview + Claude Sonnet editor (diff) | 82.7% |
| Single model (GPT-4o, diff) | ~70% |

### Cursor Apply Model Speed

| Model | Speed | Relative |
|-------|-------|----------|
| Standard Llama-3-70B | ~77 tok/s | 1x |
| Previous GPT-4 deploy | ~111 tok/s | 1.4x |
| Fine-tuned Llama-3-70B | ~500 tok/s | 6.5x |
| + Speculative edits | **~1000 tok/s** | **13x** |

---

## Implications for Cairn

Based on this research, here are recommendations for Cairn's tool system:

### 1. Primary Edit Tool: Exact Search-and-Replace
Follow Claude Code's approach. Parameters: `file_path`, `old_text`, `new_text`, `replace_all`.
- Require Read-before-Write.
- On ambiguous match (multiple occurrences), return an error with match count and positions.
- Consider supporting a `context_lines` parameter for disambiguation.

### 2. Write Tool: Full File Creation/Overwrite
For new files or complete rewrites. Enforce Read-before-Write for existing files.

### 3. Fuzzy Matching as Fallback
Implement Levenshtein-distance matching with configurable threshold (default 0.9) for when exact match fails. Log the fuzzy match for debugging.

### 4. Automatic Checkpointing
Snapshot affected files before each edit. Store checksums and original content. Enable rollback per-edit or per-conversation-turn.

### 5. Lint-After-Edit Hook
If a linter is configured for the language, run it automatically after each edit and surface errors back to the model.

### 6. Consider a Two-Stage Apply
For the assistant plane, consider having the reasoning model describe changes in natural language, then use a lighter/cheaper model to generate the actual edit tool calls. This mirrors Aider's architect pattern.

### 7. Avoid JSON-Wrapping Code
If using structured tool calls, keep code content in plain string parameters. Do not require the model to JSON-escape code blocks.

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Claude Code Tools Reference](https://code.claude.com/docs/en/tools-reference.md) | Docs | Authoritative tool definitions |
| [Claude Code Best Practices](https://code.claude.com/docs/en/best-practices.md) | Docs | Patterns for effective file editing |
| [Cursor Instant Apply Blog](https://cursor.com/blog/instant-apply) | Blog | Deep technical dive on apply models and speculative edits |
| [Aider Edit Formats](https://aider.chat/docs/more/edit-formats.html) | Docs | Comprehensive comparison of 6 edit formats |
| [Aider Unified Diffs](https://aider.chat/2023/12/21/unified-diffs.html) | Blog | Benchmarks showing 3x improvement with udiff |
| [Aider Code in JSON](https://aider.chat/2024/08/14/code-in-json.html) | Blog | Evidence that JSON wrapping degrades LLM code quality |
| [Aider Architect Mode](https://aider.chat/2024/09/26/architect.html) | Blog | Two-model approach achieving SOTA edit benchmarks |
| [Codex CLI apply_patch](https://github.com/openai/codex/blob/main/codex-rs/apply-patch) | Source | Rust implementation of Codex's custom patch format |
| [Codex CLI README](https://github.com/openai/codex/blob/main/codex-cli/README.md) | Docs | Sandbox architecture and approval modes |
| [Roo Code apply_diff](https://docs.roocode.com/advanced-usage/available-tools/apply-diff) | Docs | Fuzzy matching with line hints |
| [Cline diff.ts Source](https://github.com/cline/cline/blob/main/src/core/assistant-message/diff.ts) | Source | Tiered matching implementation |
| [Continue.dev Apply Model](https://docs.continue.dev/customize/model-roles/apply) | Docs | Apply role architecture |
| [Continue.dev Deterministic Apply](https://github.com/continuedev/continue/blob/main/core/edit/lazy/deterministic.ts) | Source | AST-based lazy block handling |
| [Sourcegraph Code Completion Lifecycle](https://sourcegraph.com/blog/the-lifecycle-of-a-code-ai-completion) | Blog | Technical architecture of completion pipelines |

---

*This guide was synthesized from 42 sources. See `resources/ai-file-edit-tools-sources.json` for full source list with quality scores.*
