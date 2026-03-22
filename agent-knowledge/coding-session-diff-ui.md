# Learning Guide: Live Diff Viewers, Code Change Streaming UIs, and Real-Time Code Editing Visualization

**Generated**: 2026-03-21
**Sources**: 20 resources analyzed
**Depth**: medium

---

## Prerequisites

- Familiarity with JavaScript/TypeScript and modern web frameworks (React, Svelte, Vue)
- Basic understanding of how git unified diff format works (`--- a/file`, `+++ b/file`, `@@ hunk headers @@`)
- Familiarity with npm ecosystem and bundlers (Vite, Webpack)
- Understanding of Server-Sent Events (SSE) or WebSocket streaming for real-time features

---

## TL;DR

- **Monaco DiffEditor** (via `@monaco-editor/react`) is the richest embeddable diff component — full VS Code diff experience, side-by-side or inline, programmatic model updates for streaming.
- **CodeMirror `@codemirror/merge`** provides both `MergeView` (split-pane) and `unifiedMergeView` (single-pane inline diff) with chunk accept/reject — best for lightweight builds.
- **diff2html** is the go-to library for rendering raw git diff strings as HTML — supports unified or side-by-side, syntax highlighting via highlight.js, synchronized scroll, and a collapsible file list.
- **jsdiff** (`diff` npm package) is the standard for computing diffs client-side: `diffLines`, `diffWords`, `diffChars` all return change objects with `{ value, added, removed }`.
- **Shiki `transformerNotationDiff`** enables syntax-highlighted static diffs via `[!code ++]` / `[!code --]` annotations — ideal for documentation and code review UIs.
- For **streaming AI-generated code changes**, the pattern is: stream text tokens into a buffer, throttle re-renders, update a Monaco/CodeMirror model incrementally rather than replacing the entire string each token.
- AI coding tools (Copilot, Cursor, Windsurf) all converge on **inline streaming + per-file accept/discard** as the dominant UX pattern. The file tree shows change badges; each file has its own diff pane.

---

## Core Concepts

### 1. Diff Computation vs. Diff Rendering

These are two separate concerns:

**Computation**: Transforming two strings into a structured description of changes. Libraries:
- `diff` (jsdiff) — pure computation, returns change object arrays
- `structuredPatch` / `createPatch` — produces unified diff format strings
- Browsers also expose this via `diff` CLI wrapped in a worker

**Rendering**: Taking that change description and displaying it. Libraries:
- `diff2html` — takes unified diff strings → HTML
- `react-diff-viewer` — takes old/new strings → React component
- Monaco DiffEditor — takes two `ITextModel`s → full editor widget
- CodeMirror `@codemirror/merge` — takes two doc strings → editor extension
- Shiki `transformerNotationDiff` — annotated code → syntax-highlighted static HTML

**Key insight**: Separating computation from rendering lets you swap renderers without changing how you detect changes, and lets you stream the "modified" side while keeping the "original" stable.

### 2. Monaco DiffEditor

Monaco's `DiffEditor` (the same engine powering VS Code's diff view) is the highest-fidelity option for embedding a rich diff UI.

**Creating a diff editor (vanilla JS):**
```javascript
const diffEditor = monaco.editor.createDiffEditor(containerElement, {
  renderSideBySide: true,       // false = inline (unified) view
  readOnly: true,               // the modified side
  originalEditable: false,      // the original side
  useInlineViewWhenSpaceIsLimited: true,  // auto-switch when narrow
  ignoreTrimWhitespace: true,
  hideUnchangedRegions: {
    enabled: true,              // collapse unchanged lines
    minimumLineCount: 3,
    contextLineCount: 3,
  },
  diffWordWrap: 'on',
  enableSplitViewResizing: true,
});

// Set the two models
diffEditor.setModel({
  original: monaco.editor.createModel(originalCode, 'typescript'),
  modified: monaco.editor.createModel(modifiedCode, 'typescript'),
});
```

**React wrapper (`@monaco-editor/react`):**
```jsx
import { DiffEditor } from '@monaco-editor/react';

<DiffEditor
  height="600px"
  language="typescript"
  original={originalCode}
  modified={modifiedCode}
  options={{ renderSideBySide: true }}
  onMount={(editor) => {
    editorRef.current = editor;
    // Access both sides:
    editor.getOriginalEditor().getValue();
    editor.getModifiedEditor().getValue();
  }}
/>
```

**Streaming updates**: To update the modified side incrementally as tokens arrive, update the model directly rather than replacing props:
```javascript
const modifiedModel = editorRef.current.getModifiedEditor().getModel();
// Replace range with new content (avoids full re-render):
modifiedModel.pushEditOperations([], [{
  range: new monaco.Range(line, col, line, col),
  text: newChunk,
}], () => null);
```

**Key option changes in recent versions:**
- v0.42+: New diff editor widget is the default; old one removed in v0.44
- `useInlineViewWhenSpaceIsLimited` replaces manual responsive handling
- Accessibility: `accessibleDiffViewerNext()` / `accessibleDiffViewerPrev()` (renamed from `diffReviewNext/Prev` in v0.41)

### 3. CodeMirror `@codemirror/merge`

The `@codemirror/merge` package provides two modes:

**Split-pane MergeView** (two separate editors side by side):
```javascript
import { MergeView } from '@codemirror/merge';
import { basicSetup } from 'codemirror';
import { EditorView, EditorState } from '@codemirror/state';

const view = new MergeView({
  a: {
    doc: originalContent,
    extensions: [basicSetup, EditorView.editable.of(false)],
  },
  b: {
    doc: modifiedContent,
    extensions: [basicSetup],
  },
  parent: document.getElementById('editor'),
});
```

**Unified (inline) view** — single pane with diff indicators:
```javascript
import { unifiedMergeView } from '@codemirror/merge';
import { EditorView } from '@codemirror/view';
import { basicSetup } from 'codemirror';

const view = new EditorView({
  parent: document.getElementById('editor'),
  doc: modifiedContent,
  extensions: [
    basicSetup,
    unifiedMergeView({
      original: originalContent,
      // highlightChanges: true,  // default
      // diffConfig: { scanLimit: 500 },
    }),
  ],
});
```

**Updating the original document** (for streaming scenarios):
```javascript
// Use the updateOriginalDoc transaction annotation
import { updateOriginalDoc } from '@codemirror/merge';
view.dispatch({ annotations: updateOriginalDoc.of(newOriginal) });
```

CodeMirror's merge view is ideal when you want a lightweight (no iframe, no VS Code overhead) diff component that fits into a Svelte/vanilla app without a React dependency.

### 4. diff2html — Rendering Raw Git Diffs

diff2html is the most widely adopted library for rendering unified diff strings as HTML. It powers Jenkins diff views, Codacy, Exercism, and many CI tools.

**Basic browser usage:**
```html
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css" />
<script src="https://cdn.jsdelivr.net/npm/diff2html/bundles/js/diff2html-ui.min.js"></script>
```

```javascript
import { Diff2HtmlUI } from 'diff2html/lib/ui/js/diff2html-ui';

const diff2htmlUI = new Diff2HtmlUI(
  document.getElementById('myDiff'),
  unifiedDiffString,
  {
    outputFormat: 'side-by-side',   // or 'line-by-line'
    matching: 'lines',              // 'words', 'none'
    drawFileList: true,
    highlight: true,
    synchronisedScroll: true,
    fileListToggle: true,
    fileContentToggle: true,
    stickyFileHeaders: true,
  }
);

diff2htmlUI.draw();
diff2htmlUI.highlightCode();
diff2htmlUI.synchronisedScroll();
diff2htmlUI.fileListToggle(true);
diff2htmlUI.stickyFileHeaders();
```

**Key insight**: diff2html expects complete unified diff strings as input. It does not compute diffs — feed it output from `git diff`, `createPatch` from jsdiff, or any unified diff string. For real-time updates, regenerate the diff string and call `draw()` again.

**Performance note**: For large diffs, use `matching: 'none'` (skip line-similarity matching) and process in file-level chunks to avoid memory issues.

### 5. jsdiff — Client-Side Diff Computation

The `diff` npm package (jsdiff) is the standard for computing diffs in JavaScript without calling a backend.

```javascript
import { diffLines, diffWords, diffChars, createPatch } from 'diff';

// Line-by-line diff (most useful for code)
const changes = diffLines(originalCode, modifiedCode);
// changes: Array<{ value: string, added?: boolean, removed?: boolean, count: number }>

// Word-level diff (for prose or inline highlights)
const wordChanges = diffWords(oldText, newText);

// Generate unified diff string (for diff2html input)
const patch = createPatch('filename.ts', original, modified, '', '', { context: 3 });
```

**Change object structure:**
```typescript
interface Change {
  value: string;       // The text of this segment
  added?: boolean;     // true if this segment was inserted
  removed?: boolean;   // true if this segment was deleted
  count: number;       // number of lines/words/chars
}
// Segments with neither added nor removed are unchanged context
```

**Performance options for large files:**
```javascript
// Abort if diff is too complex (prevents UI freeze)
const changes = diffLines(a, b, {
  maxEditLength: 1000,   // give up if edit distance exceeds this
  timeout: 500,          // abort after 500ms
  callback: (changes) => { /* async result */ },
});
```

### 6. Shiki `transformerNotationDiff`

Shiki is a syntax highlighter using VS Code's TextMate grammars. Its `@shikijs/transformers` package includes `transformerNotationDiff` for annotating code as a diff in static HTML.

```typescript
import { codeToHtml } from 'shiki';
import { transformerNotationDiff } from '@shikijs/transformers';

const html = await codeToHtml(code, {
  lang: 'typescript',
  theme: 'github-dark',
  transformers: [transformerNotationDiff()],
});
```

**Annotation syntax** (inside the code string):
```typescript
const x = 1; // [!code --]
const x = 2; // [!code ++]
```

**Output HTML structure:**
```html
<pre class="shiki has-diff">
  <code>
    <span class="line diff remove">...</span>
    <span class="line diff add">...</span>
  </code>
</pre>
```

**CSS to style the diff lines:**
```css
.has-diff .line.diff.add { background-color: rgba(0, 255, 0, 0.1); }
.has-diff .line.diff.remove { background-color: rgba(255, 0, 0, 0.1); }
/* Optional gutter indicators: */
.has-diff .line.diff.add::before { content: '+'; color: green; }
.has-diff .line.diff.remove::before { content: '-'; color: red; }
```

This approach is ideal for **static diff display** in documentation, changelogs, or code review comments where you know the diff at render time and want full syntax highlighting.

### 7. Streaming Code Changes in Real Time

When an LLM streams code, you receive tokens incrementally. The challenge is showing the diff between the "before" state and the streaming "after" state without flickering or thrashing.

**Pattern A: Buffer + throttle approach (recommended)**
```javascript
// Store original before starting
const originalContent = getFileContent(path);
let streamingBuffer = originalContent;

// On each token chunk from SSE:
function onToken(chunk) {
  streamingBuffer += chunk;
  scheduleRender();  // debounced, e.g. 100ms
}

// Render: compute diff and update view
function render() {
  const patch = createPatch('file.ts', originalContent, streamingBuffer);
  diff2htmlUI.draw(patch);  // or update Monaco modified model
}
```

**Pattern B: Monaco model push edits (smooth streaming)**
```javascript
// Keep the model open; push incremental edits
function applyChunk(chunk, position) {
  const model = diffEditor.getModifiedEditor().getModel();
  const endOfDoc = model.getFullModelRange().getEndPosition();
  model.pushEditOperations([], [{
    range: monaco.Range.fromPositions(endOfDoc),
    text: chunk,
  }], () => null);
}
```

**Pattern C: Two-phase display**
1. Phase 1 (streaming): Show the modified file as plain syntax-highlighted code with a "generating..." indicator. No diff yet.
2. Phase 2 (complete): Once streaming finishes, switch to the diff view showing original vs. generated.

This is what GitHub Copilot Agent mode does — it "streams the edits in the editor" during generation, then presents the diff for review after completion.

**Val.town's experience**: They tried unified diff requests to LLMs (asking the model to output diff format directly), got it working inconsistently, and disabled it. The lesson: LLMs are unreliable at generating valid unified diff syntax. Prefer full-file streaming + client-side diff computation.

### 8. Unified vs. Split Diff — When to Use Each

| Scenario | Recommended Mode | Rationale |
|----------|-----------------|-----------|
| AI code review, broad overview | Unified (inline) | Compact; shows context; easy to scan |
| Merge conflict resolution | Split (side-by-side) | Need to see both versions simultaneously |
| Narrow/mobile viewport | Unified (inline) | Split requires horizontal space |
| Multi-file change set | Unified per file in scrollable list | File tree nav + inline diff per file |
| Accepting/rejecting hunks | Either with chunk controls | Per-hunk buttons work in both |
| Documentation/changelog | Shiki static | No JS runtime needed |
| User prefers | Make it configurable | Provide a toggle |

**Key insight**: Monaco auto-switches to inline when `useInlineViewWhenSpaceIsLimited: true` and the editor is narrower than ~700px. CodeMirror requires you to build this switch manually.

### 9. File Tree with Change Indicators

For multi-file diffs (like AI coding agents that modify N files), the canonical UI pattern:

**File tree badge pattern:**
```svelte
<!-- Svelte example -->
<script>
  // fileChanges: Map<string, 'added' | 'modified' | 'deleted'>
  let { fileChanges, selectedFile, onSelect } = $props();
</script>

{#each files as file}
  <div
    class="file-entry"
    class:selected={selectedFile === file.path}
    onclick={() => onSelect(file.path)}
  >
    <span class="file-name">{file.name}</span>
    {#if fileChanges.has(file.path)}
      <span class="change-badge change-{fileChanges.get(file.path)}">
        {fileChanges.get(file.path) === 'added' ? 'A' :
         fileChanges.get(file.path) === 'deleted' ? 'D' : 'M'}
      </span>
    {/if}
  </div>
{/each}
```

**CSS conventions** (same as VS Code and GitHub):
- Added files: green badge `A`
- Modified files: amber/yellow badge `M`
- Deleted files: red badge `D`
- Renamed files: orange badge `R`

**Multi-file accept/discard pattern** (Copilot Edits model):
- Sidebar lists all changed files with badges
- Clicking a file opens its diff in the main pane
- Per-file "Accept" / "Discard" buttons
- Global "Accept All" / "Discard All" buttons
- Files accepted individually are removed from the sidebar list

### 10. Collapsible Hunks and Inline Comments

**Collapsible unchanged regions**: diff2html does not support this natively. Monaco DiffEditor has `hideUnchangedRegions`. For custom implementations:

```javascript
// With diff2html: post-process the DOM
// Unchanged lines have class 'd2h-cntx' (context lines)
// Group consecutive context lines, wrap in collapsible

// With CodeMirror unifiedMergeView:
// Use the collapseUnchanged option (if available in your version)
// Or use @codemirror/language's foldGutter extension on top
```

**Inline comment pattern** (like GitHub PR review):
```typescript
interface DiffComment {
  filePath: string;
  lineNumber: number;          // line in unified diff
  side: 'original' | 'modified';
  body: string;
  id: string;
}

// In a Monaco-based diff viewer, use decorations:
modifiedEditor.deltaDecorations([], [{
  range: new monaco.Range(lineNumber, 1, lineNumber, 1),
  options: {
    isWholeLine: true,
    afterContentClassName: 'inline-comment-widget',
    linesDecorationsClassName: 'comment-gutter-icon',
  },
}]);
// Render actual comment UI in a ViewZone (floating widget below the line)
```

**Accept/reject per hunk**:
```typescript
// With jsdiff: each Change object is a "hunk candidate"
// Build UI: for each { added: true } or { removed: true } block,
// render Accept/Reject buttons that splice the change out of the array
// Then recompose the final modified string

function applyChanges(changes: Change[], selections: Set<number>): string {
  return changes
    .filter((c, i) => !c.removed || !selections.has(i))
    .map(c => c.value)
    .join('');
}
```

### 11. Syntax-Highlighted Diffs in Svelte

For a pure Svelte component (no React, no Monaco overhead):

```svelte
<!-- DiffView.svelte -->
<script lang="ts">
  import { diffLines } from 'diff';
  import { codeToHtml } from 'shiki';

  let { original, modified, language = 'typescript' } = $props();

  const changes = $derived(diffLines(original, modified));

  async function highlight(code: string): Promise<string> {
    return codeToHtml(code, { lang: language, theme: 'github-dark' });
  }
</script>

<div class="diff-view">
  {#each changes as change, i (i)}
    {#if change.added}
      <div class="hunk added">
        {#await highlight(change.value) then html}
          {@html html}
        {/await}
      </div>
    {:else if change.removed}
      <div class="hunk removed">
        {#await highlight(change.value) then html}
          {@html html}
        {/await}
      </div>
    {:else}
      <details class="hunk context">
        <summary>{change.count} unchanged lines</summary>
        {#await highlight(change.value) then html}
          {@html html}
        {/await}
      </details>
    {/if}
  {/each}
</div>

<style>
  .hunk.added { background: rgba(0,255,0,0.08); border-left: 3px solid #3fb950; }
  .hunk.removed { background: rgba(255,0,0,0.08); border-left: 3px solid #f85149; }
  .hunk.context { opacity: 0.7; }
</style>
```

This pattern uses `jsdiff` for computation, `shiki` for per-segment syntax highlighting, and Svelte's `#each` for rendering.

**Performance consideration**: Calling `codeToHtml` per change segment can be slow for large diffs. Cache results with a `Map<string, string>` keyed on the code string, and highlight on demand (when hunk is expanded).

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Re-creating Monaco DiffEditor on every prop change | React lifecycle mismatch with Monaco's imperative API | Use `useRef` + `editor.getModifiedEditor().getModel().setValue()` instead of destroying/recreating |
| diff2html fails on combined diff format | Library only supports unified and git diff, not `--cc` (combined) | Always use `git diff` or `createPatch` (unified format) as input |
| Shiki `transformerNotationDiff` applied to wrong lines | `v1` vs `v3` matching algorithm difference | Use `matchAlgorithm: 'v3'` for predictable behavior; test with multi-line annotations |
| LLM asked to output unified diff format | Models inconsistently generate valid diff syntax (wrong hunk headers, off-by-one) | Have the LLM output full file content; compute diff client-side |
| Monaco diff editor blank in SSR/Svelte apps | Monaco uses browser APIs; can't render server-side | Dynamic import with `browser` check: `if (typeof window !== 'undefined')` |
| Huge diff freezes the browser | Full diff computation on every keystroke | Debounce re-computation; use `maxEditLength` option; virtualize unchanged regions |
| Split view unusable on mobile | Fixed 50/50 split doesn't work on small screens | Switch to unified mode below a breakpoint; Monaco's `useInlineViewWhenSpaceIsLimited` helps |
| Syntax highlighting + diff classes conflict | highlight.js and diff2html CSS specificity collisions | Load diff2html CSS after highlight.js; use the `auto` color scheme option |
| jsdiff `callback` parameter ignored | Async option requires explicit handling | Pass a callback function or use the promise wrapper; synchronous fallback blocks the UI |

---

## Best Practices

1. **Separate diff computation from rendering.** Use jsdiff (client-side) or `git diff` (server-side) to get structured change data; pass that to a renderer. Never ask an LLM to output diff format.

2. **Default to unified (inline) diff for AI-generated changes.** Split view implies the user needs to compare two states carefully; for AI code review the primary task is approving a proposal, not authoring changes.

3. **Show a file tree with change badges alongside the diff pane.** This is the universal pattern from VS Code, Cursor, Copilot Edits, and Windsurf. A flat list of changed files with `A/M/D` badges dramatically improves navigation in multi-file changesets.

4. **Stream first, diff second.** When an LLM generates code, stream the modified file content directly into the editor. Only compute and show the diff after streaming completes. Users find "watching code appear" more reassuring than watching a diff animate.

5. **Use Monaco DiffEditor when you need the full VS Code experience.** `@monaco-editor/react` wraps it cleanly. Budget ~2MB for the Monaco runtime; use the `@monaco-editor/loader` to load workers asynchronously.

6. **Use CodeMirror `@codemirror/merge` for lightweight builds.** CodeMirror is ~600KB (vs Monaco ~2MB), loads synchronously, and integrates cleanly with Svelte/vanilla. The `unifiedMergeView` extension is especially clean for inline diff display.

7. **Use diff2html when you have git diff strings.** If your backend runs `git diff` and sends the output to the frontend, diff2html is the simplest and best-documented renderer. Combine with `synchronisedScroll()` and `stickyFileHeaders()` for a polished experience.

8. **Collapse unchanged regions.** For code review, show 3-5 context lines around each hunk and collapse the rest. Monaco's `hideUnchangedRegions` does this automatically. For custom renderers, use `<details>` elements or an accordion pattern.

9. **Provide per-hunk accept/reject.** The granularity of accept/reject significantly affects usability. File-level is the minimum; hunk-level is preferred. Line-level is rarely needed and complex to implement.

10. **Cache syntax highlighting results.** Shiki and highlight.js are CPU-intensive. Cache highlighted HTML by content hash. In React/Svelte, `useMemo`/`$derived` on the code string prevents redundant highlighting passes.

---

## Library Comparison

| Library | Bundle Size | Mode | Syntax Highlight | Streaming | Svelte-Compatible |
|---------|-------------|------|-----------------|-----------|-------------------|
| Monaco DiffEditor | ~2MB | Split + Inline | Built-in | Via model push edits | Yes (dynamic import) |
| @codemirror/merge | ~600KB | Split + Unified | Via extensions | Via transactions | Yes, native |
| diff2html | ~150KB | Side-by-side + Line-by-line | Via highlight.js | Re-draw on update | Yes (vanilla JS) |
| react-diff-viewer | ~50KB | Split + Unified | Via Prism (render prop) | Re-render on prop change | No (React only) |
| Shiki transformerNotationDiff | ~5MB (grammar data) | Static HTML only | Built-in (full TextMate) | No | Yes |
| jsdiff | ~30KB | N/A (compute only) | N/A | Via streaming concat | Yes |

---

## Code Architecture for an AI Coding Agent Diff UI

The following pattern combines the pieces above into a coherent architecture for a Cairn-style coding session UI:

```
┌─────────────────────────────────────────────────────────┐
│  File Tree (Svelte sidebar)                             │
│  ├── src/main.ts  [M]  amber badge                      │
│  ├── src/api.ts   [A]  green badge                      │
│  └── test/...     [D]  red badge                        │
├─────────────────────────────────────────────────────────┤
│  Diff Pane (main area)                                  │
│                                                         │
│  [Unified | Split]  [Accept File] [Discard File]        │
│                                                         │
│  @@ -12,7 +12,9 @@                                      │
│  - const old = getData();           ← red, removed      │
│  + const data = await fetchData();  ← green, added      │
│  + if (!data) throw new Error(...); ← green, added      │
│    return process(data);            ← context           │
│                                                         │
│  [Accept Hunk ✓]  [Discard Hunk ✗]  ← per-hunk buttons │
├─────────────────────────────────────────────────────────┤
│  [Accept All Changes]  [Discard All]  [Apply & Deploy]  │
└─────────────────────────────────────────────────────────┘
```

**State model:**
```typescript
interface DiffSession {
  files: Map<string, {
    original: string;
    modified: string;
    status: 'streaming' | 'complete' | 'accepted' | 'discarded';
    hunks: HunkDecision[];
  }>;
  selectedFile: string | null;
  viewMode: 'unified' | 'split';
}

interface HunkDecision {
  changeIndex: number;
  decision: 'pending' | 'accepted' | 'discarded';
}
```

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Monaco Editor Playground](https://microsoft.github.io/monaco-editor/playground.html) | Interactive Docs | Live examples of DiffEditor API |
| [@monaco-editor/react README](https://github.com/suren-atoyan/monaco-react/blob/master/README.md) | Docs | DiffEditor component props and patterns |
| [diff2html GitHub](https://github.com/rtfpessoa/diff2html) | Docs | Full API, configuration, and integration examples |
| [diff2html Live Demo](https://diff2html.xyz/demo.html) | Demo | Interactive config explorer (unified/split, matching, themes) |
| [@codemirror/merge GitHub](https://github.com/codemirror/merge) | Docs | MergeView and unifiedMergeView API |
| [jsdiff GitHub](https://github.com/kpdecker/jsdiff) | Docs | Complete diff computation API |
| [Shiki Transformers](https://shiki.style/packages/transformers) | Docs | transformerNotationDiff and other code annotation transformers |
| [GitHub Copilot Edits docs](https://docs.github.com/en/copilot/using-github-copilot/asking-github-copilot-questions-in-your-ide) | Reference | Accept/discard per file UX pattern |
| [GitHub Next: Copilot Workspace](https://githubnext.com/projects/copilot-workspace) | Blog | Multi-file AI diff UX: plan → spec → editable diff |
| [Val.town blog: Fast Follow](https://blog.val.town/blog/fast-follow/) | Engineering blog | Honest account of LLM diff output unreliability |
| [Vercel AI SDK streaming](https://ai-sdk.dev/docs/foundations/streaming) | Docs | streamText + UI rendering for AI responses |
| [Simon Willison: Streaming LLM APIs](https://til.simonwillison.net/llms/streaming-llm-apis) | Technical blog | SSE/streaming implementation patterns |

---

*This guide was synthesized from 20 sources. See `resources/coding-session-diff-ui-sources.json` for full source list.*

