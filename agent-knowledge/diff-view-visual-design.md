# Learning Guide: Beautiful Diff Views - Visual Design, CSS, and Configuration

**Generated**: 2026-03-21
**Sources**: 22 resources analyzed
**Depth**: medium

---

## Prerequisites

- Familiarity with diff2html, jsdiff, Monaco, and CodeMirror (see `coding-session-diff-ui.md`)
- Understanding of CSS custom properties and dark mode patterns
- Basic knowledge of syntax highlighters (highlight.js, Shiki, Prism)

---

## TL;DR

- **diff2html's complete CSS variable set** is well-documented and mirrors GitHub's exact dark-mode palette - use it or clone it rather than inventing colors.
- **Color axiom**: use RGBA overlays (not flat colors) for dark mode diff backgrounds so syntax token colors remain visible through the highlight.
- **Line-level vs word-level color**: word/char-level changed segments need a second, more saturated overlay (the "highlight within the highlight") to pop against the line background.
- **Accessible design** requires more than red/green: add `+`/`-` gutter markers and left-border accent lines so colorblind users can distinguish additions from deletions.
- **Virtual scrolling** is unavoidable above ~3,000 diff lines; TanStack Virtual with `measureElement` handles variable-height hunks.
- **Shiki `transformerNotationDiff`** is the cleanest way to get full syntax highlighting AND diff styling simultaneously in static/documentation contexts.

---

## Core Concepts

### 1. Color Palette - Exact Values

The industry-standard palette (used by GitHub, mirrored by diff2html) separates light and dark modes completely. Never use the same hex in both.

**Light mode (GitHub-aligned):**
```css
:root {
  /* Line backgrounds */
  --diff-ins-bg:            #dfd;          /* #dafbe1 on GitHub */
  --diff-del-bg:            #fee8e9;       /* #ffebe9 on GitHub */

  /* Word/char highlight (more saturated) */
  --diff-ins-word-bg:       #97f295;       /* or #acf2bd */
  --diff-del-word-bg:       #ffb6ba;       /* or #fdb8c0 */

  /* Left-border accent */
  --diff-ins-border:        #b4e2b4;       /* #34d058 on GitHub */
  --diff-del-border:        #e9aeae;       /* #f97583 on GitHub */

  /* Gutter (line numbers area) */
  --diff-ins-gutter-bg:     #cfc;
  --diff-del-gutter-bg:     #fcc;

  /* Context / info lines */
  --diff-info-bg:           #f8fafd;
  --diff-info-border:       #d5e4f2;
}
```

**Dark mode (GitHub dark, exact diff2html values):**
```css
@media (prefers-color-scheme: dark) {
  :root {
    /* Line backgrounds - RGBA overlays, not flat colors */
    --diff-ins-bg:           rgba(46, 160, 67, 0.15);
    --diff-del-bg:           rgba(248, 81, 73, 0.10);

    /* Word/char highlight - higher opacity */
    --diff-ins-word-bg:      rgba(46, 160, 67, 0.40);
    --diff-del-word-bg:      rgba(248, 81, 73, 0.40);

    /* Left-border accent (solid) */
    --diff-ins-border:       rgba(46, 160, 67, 0.40);
    --diff-del-border:       rgba(248, 81, 73, 0.40);

    /* Label/gutter text colors */
    --diff-ins-label-color:  #3fb950;
    --diff-del-label-color:  #f85149;

    /* Background for the overall diff container */
    --diff-bg:               #0d1117;
    --diff-file-header-bg:   #161b22;
    --diff-border:           #30363d;

    /* Info/hunk header lines */
    --diff-info-bg:          rgba(56, 139, 253, 0.10);
    --diff-info-border:      rgba(56, 139, 253, 0.40);
  }
}
```

**Why RGBA in dark mode**: dark backgrounds are typically `#0d1117` (GitHub) or `#1e1e1e` (VS Code). Flat `#044B53` (react-diff-viewer's approach) creates jarring saturated blocks that destroy readability of syntax tokens. RGBA overlays let `color: #ce9178` (orange string token) remain readable on top of the green addition tint.

### 2. diff2html - Complete Configuration Reference

diff2html uses CSS custom properties prefixed `--d2h-*`. The library maps color scheme classes to these variables.

**Full constructor options:**
```javascript
import { Diff2HtmlUI } from 'diff2html/lib/ui/js/diff2html-ui';

const ui = new Diff2HtmlUI(targetElement, diffString, {
  // Layout
  outputFormat: 'side-by-side',   // 'line-by-line' | 'side-by-side'
  matching: 'lines',              // 'lines' | 'words' | 'none'

  // Color scheme
  colorScheme: 'auto',            // 'light' | 'dark' | 'auto'
  // 'auto' maps to .d2h-auto-color-scheme class,
  // which switches via @media (prefers-color-scheme: dark)

  // Features
  drawFileList: true,             // file summary at top
  fileListToggle: true,           // collapsible file summary
  fileListStartVisible: false,    // file list hidden by default
  fileContentToggle: true,        // individual file sections collapsible
  stickyFileHeaders: true,        // file headers stay on screen while scrolling
  synchronisedScroll: true,       // link scroll between side-by-side panes
  highlight: true,                // syntax highlighting via highlight.js

  // Performance
  renderNothingWhenEmpty: false,
  matchingMaxComparisons: 2500,   // max line comparisons for word-level matching
  maxLineSizeInBlockForComparison: 200,  // skip word-diff for very long lines
});

ui.draw();
ui.highlightCode();       // Must call separately after draw()
ui.synchronisedScroll();  // Must call separately
ui.fileListToggle(false); // false = start hidden
ui.stickyFileHeaders();
```

**CSS class hierarchy produced by diff2html:**
```
.d2h-wrapper                          ← outermost
  .d2h-file-list-wrapper              ← file summary
    .d2h-file-list
      .d2h-file-list-item (added|deleted|changed|moved)
  .d2h-file-wrapper                   ← per-file diff
    .d2h-file-header                  ← sticky header
      .d2h-file-name
      .d2h-tag (d2h-added|d2h-deleted|d2h-changed|d2h-moved|d2h-renamed)
    .d2h-diff-table
      .d2h-code-linenumber            ← gutter cell
      .d2h-code-line                  ← code content cell
        .d2h-code-line-prefix         ← '+' or '-' character
        .d2h-code-line-ctn            ← actual code (syntax highlighted)
          ins                         ← word-level addition highlight
          del                         ← word-level deletion highlight
```

**Line state classes:**
- `.d2h-ins` - added line row
- `.d2h-del` - deleted line row
- `.d2h-cntx` - context (unchanged) line row
- `.d2h-info` - hunk header row (`@@ -12,7 +12,9 @@`)
- `.d2h-ins.d2h-change` - modified line (added side of word diff)
- `.d2h-del.d2h-change` - modified line (deleted side of word diff)

**Overriding the color scheme with custom CSS:**
```css
/* Override just dark mode additions to use a different green */
.d2h-dark-color-scheme .d2h-ins,
.d2h-auto-color-scheme .d2h-ins {
  background-color: rgba(63, 185, 80, 0.15);
}

.d2h-dark-color-scheme .d2h-ins ins,
.d2h-auto-color-scheme .d2h-ins ins {
  background-color: rgba(63, 185, 80, 0.40);
  border-radius: 2px;
}
```

**Theming with highlight.js dual themes:**
```html
<!-- Light highlight.js theme -->
<link id="hljs-light" rel="stylesheet"
  href="https://cdn.jsdelivr.net/npm/highlight.js@11/styles/github.min.css"
  media="(prefers-color-scheme: light)">
<!-- Dark highlight.js theme -->
<link id="hljs-dark" rel="stylesheet"
  href="https://cdn.jsdelivr.net/npm/highlight.js@11/styles/github-dark.min.css"
  media="(prefers-color-scheme: dark)">
<!-- diff2html CSS (must load AFTER hljs to win specificity) -->
<link rel="stylesheet"
  href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css">
```

**Slim bundle (no hljs bundled, specify languages manually):**
```javascript
import { Diff2HtmlUI } from 'diff2html/lib/ui/js/diff2html-ui-slim';
import hljs from 'highlight.js/lib/core';
import javascript from 'highlight.js/lib/languages/javascript';
import typescript from 'highlight.js/lib/languages/typescript';
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('typescript', typescript);

const ui = new Diff2HtmlUI(el, diffString, config, hljs);
```

### 3. Syntax Highlighting in Diffs

Three approaches depending on the rendering library:

**A. diff2html + highlight.js (automatic language detection)**
diff2html calls `hljs.highlightAuto()` on each code block after extracting the file extension from the diff header (`--- a/foo.ts`). Works out of the box.

To force a language (when extension is ambiguous):
```javascript
// Not directly supported - diff2html infers from file header
// Workaround: use the highlightLanguages config map
const ui = new Diff2HtmlUI(el, diff, {
  highlight: true,
  // highlightLanguages is a Map<fileExtension, languageAlias>
  highlightLanguages: new Map([['tsx', 'typescript'], ['mjs', 'javascript']]),
});
```

**B. Shiki transformerNotationDiff (static, best quality)**
```typescript
import { codeToHtml } from 'shiki';
import { transformerNotationDiff } from '@shikijs/transformers';

// Annotate the code string with markers:
const code = `
const x = 1; // [!code --]
const x = 2; // [!code ++]
`;

const html = await codeToHtml(code, {
  lang: 'typescript',
  themes: {
    light: 'github-light',
    dark: 'github-dark',
  },
  transformers: [transformerNotationDiff({ matchAlgorithm: 'v3' })],
});

// Produces: <span class="line diff remove"> and <span class="line diff add">
// Pre element gets class "has-diff"
```

CSS for Shiki diff lines:
```css
/* Container */
.shiki.has-diff { overflow-x: auto; }

/* Line backgrounds */
.shiki .line.diff.add  { background-color: rgba(46, 160, 67, 0.15); }
.shiki .line.diff.remove { background-color: rgba(248, 81, 73, 0.10); }

/* Gutter +/- indicators */
.shiki .line.diff::before {
  content: ' ';
  display: inline-block;
  width: 1.2em;
  margin-right: 0.5em;
}
.shiki .line.diff.add::before    { content: '+'; color: #3fb950; }
.shiki .line.diff.remove::before { content: '-'; color: #f85149; }
```

**C. Prism.js diff-highlight plugin**
Prism has a dedicated `diff-highlight` plugin for code blocks with `language-diff-typescript` class.

```html
<pre><code class="language-diff-typescript">
- const x = 1;
+ const x = 2;
</code></pre>
```
Uses `prism-diff-highlight.js` plugin + base language grammar. Works well for documentation.

### 4. Visual Design Patterns from GitHub/VS Code

**Left-border accent line** (universal pattern):
```css
.diff-line-added {
  background: var(--diff-ins-bg);
  border-left: 3px solid var(--diff-ins-border);
}
.diff-line-removed {
  background: var(--diff-del-bg);
  border-left: 3px solid var(--diff-del-border);
}
```

**Gutter column structure** (GitHub pattern - 2-column gutter for unified view):
```css
.diff-gutter {
  min-width: 80px;       /* two 40px line number columns */
  padding: 0 8px;
  color: rgba(0,0,0,0.3);
  user-select: none;
  font-size: 12px;
  text-align: right;
  vertical-align: top;
}

/* In side-by-side: single column per side */
.diff-gutter-side { min-width: 40px; }
```

**Inline `+`/`-` prefix marker** (instead of only color):
```css
.d2h-code-line-prefix {
  font-weight: bold;
  padding-right: 4px;
  user-select: none;
}
.d2h-ins .d2h-code-line-prefix { color: #3fb950; }
.d2h-del .d2h-code-line-prefix { color: #f85149; }
```

**Sticky file headers** (critical for long multi-file diffs):
```css
.d2h-file-header {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--diff-file-header-bg);
  border-bottom: 1px solid var(--diff-border);
  display: flex;
  align-items: center;
  padding: 6px 12px;
  font-size: 13px;
  font-family: ui-monospace, 'SFMono-Regular', 'SF Mono', Menlo, monospace;
}
```

**Monospace font stack** (matches VS Code/GitHub):
```css
.diff-view {
  font-family: ui-monospace, 'SFMono-Regular', 'SF Mono', Consolas,
               'Liberation Mono', Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
  tab-size: 4;
}
```

**Hunk header styling** (the `@@ -12,7 +12,9 @@` line):
```css
.d2h-info {
  background: rgba(56, 139, 253, 0.10);
  border-top: 1px solid rgba(56, 139, 253, 0.40);
  border-bottom: 1px solid rgba(56, 139, 253, 0.40);
  color: #8b949e;
  font-style: italic;
  padding: 4px 0;
}
```

**Collapse unchanged regions** (`<details>` pattern for non-Monaco):
```css
details.diff-context > summary {
  cursor: pointer;
  background: #f1f8ff;
  color: #0366d6;
  padding: 4px 12px;
  font-size: 12px;
  list-style: none;
  border-top: 1px solid #e1e4e8;
  border-bottom: 1px solid #e1e4e8;
}
details.diff-context > summary:hover { background: #dbedff; }
details.diff-context > summary::marker { display: none; }
details.diff-context > summary::before { content: '...'; margin-right: 8px; }

/* Dark mode */
@media (prefers-color-scheme: dark) {
  details.diff-context > summary {
    background: rgba(56, 139, 253, 0.10);
    color: #58a6ff;
    border-color: rgba(56, 139, 253, 0.30);
  }
}
```

**File status badges** (A/M/D/R):
```css
.file-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px; height: 16px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 700;
  font-family: monospace;
  margin-left: 8px;
}
.file-badge-added    { background: #196c2e; color: #56d364; }
.file-badge-modified { background: #5a3e1b; color: #d29922; }
.file-badge-deleted  { background: #6d1f1f; color: #f85149; }
.file-badge-renamed  { background: #0d419d; color: #79c0ff; }

/* Light mode */
@media (prefers-color-scheme: light) {
  .file-badge-added    { background: #dafbe1; color: #1a7f37; }
  .file-badge-modified { background: #fff8c5; color: #9a6700; }
  .file-badge-deleted  { background: #ffebe9; color: #cf222e; }
  .file-badge-renamed  { background: #ddf4ff; color: #0969da; }
}
```

### 5. Accessible Color Design

Red/green alone fails WCAG 2.1 AA for color-blind users (deuteranopia affects ~8% of men). Three mitigations:

**1. Add the `+`/`-` gutter prefix** - ensures the distinction is not color-only.

**2. Use blue/orange as an alternative palette** (accessible):
```css
/* Accessible alternative to red/green */
.diff-accessible-ins { background: rgba(0, 100, 255, 0.12); border-left: 3px solid #0064ff; }
.diff-accessible-del { background: rgba(200, 80, 0, 0.12);  border-left: 3px solid #c85000; }
```

**3. Contrast ratios for the standard palette** (check with a tool like polished or colorable):
- Light mode `#399839` label on `#fff` = 4.5:1 (passes AA)
- Dark mode `#3fb950` label on `#0d1117` = 5.2:1 (passes AA)
- Light mode `#c33` label on `#fff` = 7.1:1 (passes AAA)
- Dark mode `#f85149` label on `#0d1117` = 4.8:1 (passes AA)

The word-level highlights (RGBA overlays) inherently have low contrast on their own. They are accent layers on top of the +/- prefix and border - they should not be the sole signal.

### 6. Side-by-Side vs Unified - CSS Differences

**Unified view** uses a single-table layout with line numbers in a 2-column gutter:
```css
.diff-unified .diff-line {
  display: table-row;
  width: 100%;
}
.diff-unified .diff-gutter {
  display: table-cell;
  width: 90px;   /* "before" + "after" line numbers */
  padding-right: 12px;
}
```

**Side-by-side view** uses either CSS Grid or two adjacent tables:
```css
/* Grid approach (cleaner, no scroll sync needed for purely visual alignment) */
.diff-side-by-side {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 1px;
  background: var(--diff-border); /* 1px gap shows as border line */
}
.diff-side-by-side .diff-pane {
  overflow-x: auto;
  min-width: 0;
}
```

diff2html uses two `<table>` elements with synchronized scroll via JavaScript. The table approach is simpler but requires `synchronisedScroll()` to keep panes in sync.

**Auto-switching to unified on narrow viewports:**
```css
@media (max-width: 700px) {
  .diff-side-by-side { grid-template-columns: 1fr; }
  .diff-side-by-side .diff-pane-right { display: none; }
  /* Or switch diff2html outputFormat programmatically */
}
```

```javascript
// Monaco approach (automatic):
monaco.editor.createDiffEditor(el, {
  useInlineViewWhenSpaceIsLimited: true,  // switches automatically below ~700px
});
```

### 7. Virtual Scrolling for Large Diffs

Diffs above ~3,000 lines will cause rendering lag. Browsers can handle ~1,000 DOM rows without noticing; beyond that, virtualize.

**Strategy**: virtual-scroll at the hunk level, not the line level. Each hunk is a variable-height block. This is simpler than line-level virtualization and still provides the needed performance.

**TanStack Virtual pattern for hunks:**
```typescript
import { useVirtualizer } from '@tanstack/react-virtual';

function VirtualDiffView({ hunks }: { hunks: Hunk[] }) {
  const parentRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: hunks.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (i) => hunks[i].lines.length * 20 + 32, // 20px/line + 32px header
    overscan: 3,           // render 3 extra hunks above/below viewport
    measureElement: (el) => el.getBoundingClientRect().height,  // actual height after render
  });

  return (
    <div ref={parentRef} style={{ height: '100%', overflow: 'auto' }}>
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((vi) => (
          <div
            key={vi.key}
            data-index={vi.index}
            ref={virtualizer.measureElement}
            style={{ position: 'absolute', top: vi.start, width: '100%' }}
          >
            <HunkBlock hunk={hunks[vi.index]} />
          </div>
        ))}
      </div>
    </div>
  );
}
```

**Alternative: lazy rendering with Intersection Observer** (simpler, no library needed):
```javascript
// Render a placeholder for each hunk; upgrade to real content when visible
const observer = new IntersectionObserver((entries) => {
  entries.forEach(entry => {
    if (entry.isIntersecting) {
      const index = +entry.target.dataset.hunkIndex;
      renderHunk(entry.target, hunks[index]);
      observer.unobserve(entry.target);
    }
  });
}, { rootMargin: '200px' });

hunkPlaceholders.forEach(el => observer.observe(el));
```

**diff2html with large diffs - configuration to reduce cost:**
```javascript
const ui = new Diff2HtmlUI(el, diffString, {
  matching: 'none',                    // Skip word-level matching (expensive)
  maxLineSizeInBlockForComparison: 100, // Skip word-diff for long lines
  matchingMaxComparisons: 1000,        // Reduce max comparisons
  renderNothingWhenEmpty: true,
});
```

### 8. Copy Button and Line Linking

**Copy button on hover** (CSS + JS):
```css
.diff-file-header { position: relative; }
.diff-copy-btn {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0;
  transition: opacity 0.15s;
  padding: 4px 8px;
  border-radius: 6px;
  border: 1px solid var(--diff-border);
  background: var(--diff-file-header-bg);
  cursor: pointer;
  font-size: 11px;
}
.diff-file-header:hover .diff-copy-btn { opacity: 1; }
```

**Line anchor links** (GitHub-style `#L42`):
```css
.diff-line-number:hover::after {
  content: '#';
  color: #0969da;
  margin-left: 4px;
  text-decoration: none;
}
tr:target { background: #fff8c5 !important; }  /* highlight linked line */
```

### 9. Polished CSS Tricks

**Smooth scroll into view on file navigation:**
```javascript
document.querySelector(`[data-file="${fileName}"]`)
  ?.scrollIntoView({ behavior: 'smooth', block: 'start' });
```

**Prevent text-select from capturing gutter clicks:**
```css
.diff-gutter { user-select: none; }
```

**Horizontal scroll only on code columns, not gutter:**
```css
.diff-table {
  display: grid;
  grid-template-columns: 80px 1fr;  /* fixed gutter + scrollable code */
}
.diff-code { overflow-x: auto; }
```

**Word wrapping toggle** (long lines):
```css
.diff-view.wrap .diff-code { white-space: pre-wrap; word-break: break-all; }
.diff-view:not(.wrap) .diff-code { white-space: pre; }
```

**Subtle box shadow for file blocks** (depth without weight):
```css
.d2h-file-wrapper {
  border: 1px solid var(--diff-border);
  border-radius: 6px;
  overflow: hidden;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
}
```

**Empty line placeholders** in side-by-side (for unmatched hunks):
```css
.d2h-emptyplaceholder {
  background-color: var(--diff-empty-bg, #f1f1f1);
  border-left: 3px solid transparent;  /* invisible border to match spacing */
}
.d2h-dark-color-scheme .d2h-emptyplaceholder {
  background-color: hsla(215, 8%, 47%, 0.1);
}
```

**Focus outline for keyboard navigation:**
```css
.diff-line:focus { outline: 2px solid #0969da; outline-offset: -2px; }
.diff-line { tabindex: 0; }
```

**Loading skeleton for large diffs** (while processing):
```css
.diff-skeleton {
  background: linear-gradient(
    90deg,
    var(--diff-file-header-bg) 25%,
    rgba(255,255,255,0.05) 50%,
    var(--diff-file-header-bg) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  height: 20px;
  border-radius: 3px;
}
@keyframes shimmer { to { background-position: -200% 0; } }
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Flat colors in dark mode destroy syntax visibility | Using `background: #044B53` instead of RGBA | Use `rgba(46,160,67,0.15)` so syntax token colors show through |
| highlight.js CSS overrides diff2html colors | CSS load order | Load diff2html CSS after hljs CSS |
| diff2html `colorScheme: 'auto'` not working | Missing `d2h-auto-color-scheme` class on wrapper | diff2html applies it automatically; check wrapper element has it |
| Sticky file headers disappear | Parent container has `overflow: hidden` | Use `overflow: clip` instead, or restructure container layout |
| Side-by-side panes out of sync after resize | Scroll positions diverge | Call `synchronisedScroll()` on resize events |
| Word-level diff on minified files freezes browser | O(n^2) comparison on single very long line | Set `maxLineSizeInBlockForComparison: 100` to skip word-diff for long lines |
| Text selection picks up `+`/`-` prefixes | Prefix not excluded from selection | Add `user-select: none` to `.d2h-code-line-prefix` |
| Shiki `has-diff` class not on `pre` element | `transformerNotationDiff` only added in `@shikijs/transformers` | Ensure `@shikijs/transformers` is imported, not just shiki core |
| Virtual scroll heights jump when hunks expand | `estimateSize` too inaccurate | Use `measureElement` callback to let virtualizer self-correct |

---

## Best Practices

1. **Use diff2html's `colorScheme: 'auto'`** with both light and dark hljs themes loaded via `media` attribute. This is the simplest way to get correct dark mode support.

2. **Always add left-border accents** (`border-left: 3px solid`) alongside background colors. This provides a color-blind-accessible secondary signal without requiring extra markup.

3. **Use RGBA overlays for dark mode backgrounds** - not flat colors. The diff background must be translucent so syntax highlighting tokens remain readable.

4. **Font choice matters**: always use a monospace font stack. `ui-monospace` resolves to the OS's default monospace (SF Mono on Mac, Cascadia Code on Windows) before any specified fallback.

5. **Keep gutter non-selectable** (`user-select: none` on `.d2h-code-linenumber`). Selecting line numbers is frustrating when copying code.

6. **Add sticky file headers** for multi-file diffs. Users scroll past dozens of files; knowing which file they are in is essential.

7. **Collapse context lines** to 3-5 lines per hunk. Showing 100 unchanged lines between two changed lines is noise. Use Monaco's `hideUnchangedRegions` or `<details>` elements.

8. **Virtualize above 500 hunks** (~3,000 lines). The browser stalls hard at ~10,000 DOM rows. TanStack Virtual with `measureElement` handles variable-height hunks correctly.

9. **Test hljs language detection** for your file types. diff2html infers language from the file extension in the diff header. Extensions like `.svelte`, `.astro`, or `.mts` may need the `highlightLanguages` map override.

10. **Provide a word-wrap toggle** in the UI. Long SQL queries, minified JSON, and generated files all need wrapping. Default to off (better for code), let users enable it.

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [diff2html CSS source](https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css) | Source | Full CSS variable reference with exact values |
| [diff2html GitHub](https://github.com/rtfpessoa/diff2html) | Docs | Full configuration API and renderer source |
| [diff2html live demo](https://diff2html.xyz/demo.html) | Demo | Interactive explorer: switch format, matching, color scheme |
| [Shiki transformers docs](https://shiki.style/packages/transformers) | Docs | transformerNotationDiff configuration and CSS classes |
| [Shiki dual themes](https://shiki.style/guide/dual-themes) | Docs | CSS variable approach for light/dark syntax highlighting |
| [react-diff-viewer styles](https://github.com/praneshr/react-diff-viewer/blob/master/src/styles.ts) | Source | Full dark/light theme variable reference for React projects |
| [git-diff-view](https://github.com/MrWangJustToDo/git-diff-view) | Library | React/Vue/Svelte diff viewer with Shiki, FastDiff char-level, SSR support |
| [TanStack Virtual docs](https://tanstack.com/virtual/latest) | Docs | Virtualizer API for variable-height rows |
| [CodeMirror merge README](https://github.com/codemirror/merge) | Docs | MergeView + unifiedMergeView configuration |
| [WCAG color contrast checker](https://webaim.org/resources/contrastchecker/) | Tool | Verify your palette meets AA (4.5:1) requirements |
| [jsdiff GitHub](https://github.com/kpdecker/jsdiff) | Docs | Diff computation API including maxEditLength, timeout |

---

*This guide was synthesized from 22 sources. See `resources/diff-view-visual-design-sources.json` for full source list.*
*Companion to `coding-session-diff-ui.md` which covers library selection, Monaco/CodeMirror API, and streaming patterns.*
