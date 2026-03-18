# Learning Guide: AI Chat Thinking/Reasoning UI Patterns

**Generated**: 2026-03-18
**Sources**: 24 resources analyzed
**Depth**: medium

## Prerequisites

- Familiarity with modern frontend frameworks (React, Svelte, or Vue)
- Understanding of Server-Sent Events (SSE) and streaming responses
- Basic knowledge of WAI-ARIA accessibility patterns
- Awareness of LLM API response structures (OpenAI, Anthropic, Google, DeepSeek)

## TL;DR

- All major AI providers (OpenAI, Anthropic, Google, DeepSeek) return thinking/reasoning as separate content blocks alongside the main response, each with slightly different API shapes.
- The dominant UI pattern is a **collapsible disclosure widget** -- collapsed by default after completion, expanded during active streaming -- with a header showing a summary label and optional timing/token count.
- Open-source chat UIs (LibreChat, Open WebUI, LobeChat, Vercel AI Chatbot) converge on the same pattern: a `<Reasoning>` or `<Thinking>` component wrapping a `Collapsible`/`<details>` element with `aria-expanded`, `aria-controls`, and screen-reader hiding when collapsed.
- Animation during thinking uses three tiers: (1) a "Thinking..." label with bouncing dots before any tokens arrive, (2) live-streaming reasoning text inside an expanded block, and (3) a smooth height transition on collapse after completion.
- Accessibility requires the WAI-ARIA Disclosure pattern: `role="button"` on the trigger, `aria-expanded` reflecting state, `aria-controls` pointing to the content region, and `aria-hidden` on collapsed content to prevent screen reader clutter.

---

## 1. Provider API Formats for Thinking/Reasoning

Understanding the wire format is essential before designing the UI. Each provider returns thinking content differently.

### 1.1 Anthropic (Claude) -- Extended Thinking

Claude returns thinking as a separate content block with `type: "thinking"` before the `type: "text"` block.

**Response structure:**
```json
{
  "content": [
    {
      "type": "thinking",
      "thinking": "Let me analyze this step by step...",
      "signature": "WaUjzkypQ2mUEVM36O2Txu..."
    },
    {
      "type": "text",
      "text": "Based on my analysis..."
    }
  ]
}
```

**Key details:**
- Enable with `thinking: { type: "enabled", budget_tokens: 10000 }`
- `display: "summarized"` (default) returns a summary; `display: "omitted"` returns empty thinking with signature only
- Streaming emits `thinking_delta` events followed by `text_delta` events
- The `signature` field contains encrypted full thinking for multi-turn continuity
- Billing is based on full thinking tokens, not the summary length

**Streaming event sequence:**
```
content_block_start  -> { type: "thinking" }
content_block_delta  -> { type: "thinking_delta", thinking: "..." }
content_block_delta  -> { type: "signature_delta", signature: "..." }
content_block_stop
content_block_start  -> { type: "text" }
content_block_delta  -> { type: "text_delta", text: "..." }
content_block_stop
```

(Source: Anthropic Extended Thinking Documentation)

### 1.2 OpenAI (o1, o3, o4-mini) -- Reasoning Summaries

OpenAI reasoning models produce internal chain-of-thought that is NOT directly exposed. Instead, the API returns **reasoning summaries**.

**Key details:**
- Reasoning happens internally; raw thinking tokens are not surfaced
- The API returns `reasoning_summary` content alongside the main response
- `reasoning_effort` parameter controls how much reasoning the model does
- Summary tokens are separate from completion tokens in usage counts

(Source: OpenAI Reasoning Documentation)

### 1.3 DeepSeek (deepseek-reasoner) -- reasoning_content Field

DeepSeek returns reasoning in a dedicated `reasoning_content` field at the same level as `content`.

**Key details:**
- Output has two fields: `reasoning_content` (CoT) and `content` (final answer)
- In streaming, reasoning tokens and content tokens arrive in separate chunks
- CoT from previous rounds must NOT be concatenated into context (causes 400 errors)
- `max_tokens` controls total output including CoT
- Temperature, top_p, and other sampling parameters are not supported

(Source: DeepSeek API Documentation)

### 1.4 Google Gemini -- Thought Summaries

Gemini returns summarized thought content alongside the response when `includeThoughts: true`.

**Key details:**
- Response parts include a `thought: true` boolean property on thinking parts
- Streaming returns "rolling, incremental summaries" rather than raw thought tokens
- Gemini 3 models use `thinkingLevel` (minimal/low/medium/high)
- Gemini 2.5 models use `thinkingBudget` (0 to disable, -1 for dynamic)
- Token usage includes `thoughtsTokenCount` in usage metadata

(Source: Google AI Gemini Thinking Documentation)

### 1.5 Open-Source Models (DeepSeek-R1, Qwen QwQ)

Open-weight reasoning models use `<think>...</think>` XML tags inline in the response text.

**Key details:**
- Thinking content is wrapped in `<think>` tags within the regular content stream
- Applications must parse these tags client-side to separate thinking from response
- No separate API field -- everything comes in one text stream
- Open WebUI and LobeChat both parse these tags for display

(Source: Open WebUI issues, LobeChat issues)

---

## 2. UI Patterns Across Major Platforms

### 2.1 ChatGPT (OpenAI)

**Pattern: Collapsed summary with "Thought for X seconds" header**

- During processing: Shows "Thinking..." with animated indicator
- After completion: Displays a collapsed block with header text like "Thought for 12 seconds"
- Clicking the header expands to show the reasoning summary
- The summary is a condensed version, not raw thinking tokens
- Collapsed by default after generation completes
- No raw chain-of-thought is ever shown to users

### 2.2 Claude.ai (Anthropic)

**Pattern: Foldable thinking block with summary text**

- During streaming: Thinking block is expanded, showing summarized reasoning as it streams
- After completion: Block collapses automatically, showing a clickable header
- Thinking content appears "less polished" and "more detached" than the main response (by design)
- Harmful reasoning content is encrypted and shows "the rest of the thought process is not available for this response"
- Available to Pro, Team, Enterprise, and API users

(Source: Anthropic Visible Extended Thinking announcement)

### 2.3 DeepSeek Chat

**Pattern: Collapsible "Thought for X seconds" block**

- Uses a similar pattern to ChatGPT with a timing header
- During streaming: Shows reasoning tokens in real-time within an expanded block
- After completion: Collapses with a summary header showing duration
- Raw chain-of-thought is visible (unlike OpenAI's summaries)

### 2.4 Gemini (Google)

**Pattern: "Thinking..." indicator with expandable summary**

- During processing: Shows an animated "Thinking" indicator
- After completion: Provides a collapsible thought summary
- Rolling incremental summaries during streaming give progressive insight

---

## 3. Open-Source Chat UI Implementations

### 3.1 LibreChat

**Architecture:**
- `Part.tsx` component detects `ContentTypes.THINK` and routes to `<Reasoning>` component
- `MessageContent.tsx` uses `parseThinkingContent()` to extract `:::thinking...:::` blocks
- Separate `Thinking` component (from `./Parts/Thinking`) handles the display

**Parsing logic:**
```typescript
const parseThinkingContent = (text: string) => {
  const thinkingMatch = text.match(/:::thinking([\s\S]*?):::/);
  return {
    thinkingContent: thinkingMatch ? thinkingMatch[1].trim() : '',
    regularContent: thinkingMatch
      ? text.replace(/:::thinking[\s\S]*?:::/, '').trim()
      : text,
  };
};
```

**Accessibility (PR #11927):**
- `aria-hidden={!isExpanded}` dynamically hides collapsed content from screen readers
- `role="group"` with unique `id` via `useId()` for semantic grouping
- `aria-controls` on toggle button linking to content region
- `aria-expanded` reflects current state
- `aria-label` on content group for screen reader announcement

**UI polish (PR #11142):**
- `FloatingThinkingBar` appears on hover/focus with absolute positioning
- Dynamic icons for expand/collapse state
- "Copy Thoughts" button with proper `aria-label`
- Prevents layout expansion during message streaming
- Full keyboard navigation support

(Sources: LibreChat GitHub PRs #11142, #11927)

### 3.2 Open WebUI

**Architecture:**
- Uses Svelte with `<details>` HTML elements for collapsible thinking
- Parses `<think>` tags from model output in the Markdown rendering pipeline
- `ContentRenderer.svelte` delegates to `Markdown.svelte` for actual parsing

**Known issues and fixes:**
- Issue #21348: Reasoning traces were "visually split to many parts" during streaming, causing browser slowdowns -- fixed by restoring proper `<think>` tag interpretation
- Issue #22267: HTML preview was rendering code blocks inside thinking blocks incorrectly
- Issue #22237: TTS was reading thinking content aloud when reasoning contained code blocks -- fixed
- Issue #20286: Responses were being duplicated for thinking models during streaming -- fixed

(Sources: Open WebUI GitHub issues)

### 3.3 LobeChat

**Architecture:**
- React-based with dedicated thinking component
- Handles `reasoning_content` from providers and `<think>` tags from open models
- PR #13067: Filters internal thinking content in OpenAI-compatible payloads
- Issue #12829: Working on displaying reasoning summaries in non-streaming responses

**Known considerations:**
- Combining thinking models with web search causes issues (Issue #12589)
- Provider-specific thinking toggles needed (e.g., `enable_thinking` for Qwen)

(Source: LobeChat GitHub issues)

### 3.4 Vercel AI Chatbot Template

**Architecture:**
- `MessageReasoning` component wraps `<Reasoning>`, `<ReasoningTrigger>`, `<ReasoningContent>` primitives
- Uses `hasBeenStreaming` state to track whether to show reasoning expanded
- `defaultOpen={hasBeenStreaming}` ensures reasoning is expanded during streaming
- `isStreaming={isLoading}` prop enables animation during active generation

**ThinkingMessage loading state:**
```jsx
// Animated "Thinking" label with bouncing dots
<ThinkingMessage>
  Thinking
  <span class="animate-bounce" style="animation-delay: 0ms">.</span>
  <span class="animate-bounce" style="animation-delay: 150ms">.</span>
  <span class="animate-bounce" style="animation-delay: 300ms">.</span>
</ThinkingMessage>
```

**Vercel AI SDK integration:**
```typescript
// Server-side: forward reasoning to client
return result.toUIMessageStreamResponse({ sendReasoning: true });

// Client-side: render reasoning parts
message.parts.map((part, index) => {
  if (part.type === 'reasoning') {
    return <pre key={index}>{part.text}</pre>;
  }
  if (part.type === 'text') {
    return <div key={index}>{part.text}</div>;
  }
});
```

(Sources: Vercel AI SDK documentation, Vercel AI Chatbot GitHub)

---

## 4. The Collapsible Thinking Block Pattern (Reference Implementation)

### 4.1 Component Structure

The consensus pattern across all implementations:

```
+--------------------------------------------------+
| [icon] Thinking...  (during streaming)            |
|                                                   |
|   Reasoning text streams here in real-time...     |
|   Each token appended as it arrives...            |
+--------------------------------------------------+

          |  (after completion, auto-collapses to)
          v

+--------------------------------------------------+
| [v] Thought for 12s                    [copy] [^] |
+--------------------------------------------------+
```

### 4.2 State Machine

```
IDLE -> THINKING -> STREAMING_REASONING -> STREAMING_RESPONSE -> COMPLETE
  |                    |                       |                    |
  |                    | (expanded, animated)  | (expanded)         | (collapsed)
  |                    |                       |                    |
  show nothing        show "Thinking..."      show reasoning       show header only
                      with animation          text streaming        user can toggle
```

### 4.3 React Reference Implementation

```tsx
import { Collapsible, CollapsibleContent, CollapsibleTrigger }
  from "@/components/ui/collapsible";

interface ThinkingBlockProps {
  reasoning: string;
  isStreaming: boolean;
  thinkingDuration?: number;
}

function ThinkingBlock({ reasoning, isStreaming, thinkingDuration }: ThinkingBlockProps) {
  const [isOpen, setIsOpen] = useState(isStreaming);
  const contentId = useId();

  // Auto-expand when streaming starts, auto-collapse when done
  useEffect(() => {
    if (isStreaming) setIsOpen(true);
    else setIsOpen(false);
  }, [isStreaming]);

  if (!reasoning && !isStreaming) return null;

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger
        aria-controls={contentId}
        aria-expanded={isOpen}
        className="flex items-center gap-2 text-sm text-muted-foreground"
      >
        <ChevronIcon className={cn(
          "h-4 w-4 transition-transform",
          isOpen && "rotate-90"
        )} />

        {isStreaming ? (
          <span className="flex items-center gap-1">
            <span>Thinking</span>
            <ThinkingDots />
          </span>
        ) : (
          <span>Thought for {thinkingDuration}s</span>
        )}
      </CollapsibleTrigger>

      <CollapsibleContent
        id={contentId}
        role="group"
        aria-label="Model reasoning"
        aria-hidden={!isOpen}
      >
        <div className={cn(
          "mt-2 rounded-md border p-3 text-sm",
          "bg-muted/50 text-muted-foreground",
          "font-mono leading-relaxed",
          isStreaming && "animate-pulse-subtle"
        )}>
          {reasoning || <ThinkingSkeleton />}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
```

### 4.4 Svelte Reference Implementation

```svelte
<script lang="ts">
  import { slide } from 'svelte/transition';

  export let reasoning: string = '';
  export let isStreaming: boolean = false;
  export let duration: number | null = null;

  let isOpen = isStreaming;

  $: if (isStreaming) isOpen = true;
  $: if (!isStreaming && reasoning) isOpen = false;
</script>

{#if reasoning || isStreaming}
  <div class="thinking-block">
    <button
      class="thinking-trigger"
      aria-expanded={isOpen}
      on:click={() => isOpen = !isOpen}
    >
      <svg class="chevron" class:rotated={isOpen}><!-- chevron icon --></svg>

      {#if isStreaming}
        <span class="thinking-label">
          Thinking<span class="dots"><span>.</span><span>.</span><span>.</span></span>
        </span>
      {:else}
        <span>Thought for {duration}s</span>
      {/if}
    </button>

    {#if isOpen}
      <div
        transition:slide={{ duration: 200 }}
        class="thinking-content"
        role="group"
        aria-label="Model reasoning"
      >
        {reasoning}
      </div>
    {/if}
  </div>
{/if}
```

---

## 5. Animation Patterns During Thinking

### 5.1 Pre-Token Phase: "Thinking..." Indicator

Before any reasoning tokens arrive, show a lightweight animation.

**Pattern A: Bouncing Dots (most common)**
```css
.thinking-dots span {
  display: inline-block;
  animation: bounce 1.4s ease-in-out infinite;
}
.thinking-dots span:nth-child(2) { animation-delay: 0.15s; }
.thinking-dots span:nth-child(3) { animation-delay: 0.3s; }

@keyframes bounce {
  0%, 80%, 100% { transform: translateY(0); }
  40% { transform: translateY(-6px); }
}
```

**Pattern B: Pulsing Label**
```css
.thinking-label {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
```

**Pattern C: Shimmer Bar (skeleton loading)**
```css
.thinking-skeleton {
  height: 1em;
  background: linear-gradient(
    90deg,
    hsl(var(--muted)) 25%,
    hsl(var(--muted-foreground) / 0.1) 50%,
    hsl(var(--muted)) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s ease-in-out infinite;
  border-radius: 4px;
}

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
```

### 5.2 Streaming Phase: Live Reasoning Text

During token streaming, the expanded thinking block shows text appearing progressively.

**Best practices:**
- Use `overflow-y: auto` with a `max-height` to prevent the thinking block from dominating the viewport
- Apply a subtle cursor blink at the end of streaming text
- Scroll to bottom of thinking block as new tokens arrive
- Consider a muted/monospace font to differentiate from the final response

```css
.thinking-content {
  max-height: 300px;
  overflow-y: auto;
  scroll-behavior: smooth;
  font-family: var(--font-mono);
  color: hsl(var(--muted-foreground));
}

.thinking-content.streaming::after {
  content: '|';
  animation: cursor-blink 1s step-end infinite;
}

@keyframes cursor-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}
```

### 5.3 Collapse Phase: Smooth Transition

When thinking completes and the block auto-collapses.

**Modern CSS approach (Chrome 129+):**
```css
:root {
  interpolate-size: allow-keywords;
}

.thinking-content {
  overflow: hidden;
  transition: height 300ms ease-out, opacity 200ms ease-out;
}

.thinking-content[aria-hidden="true"] {
  height: 0;
  opacity: 0;
}
```

**Fallback approach (all browsers):**
```css
.thinking-content {
  display: grid;
  grid-template-rows: 1fr;
  transition: grid-template-rows 300ms ease-out;
}

.thinking-content.collapsed {
  grid-template-rows: 0fr;
}

.thinking-content > div {
  overflow: hidden;
}
```

### 5.4 Timing Guidelines

| Phase | Duration | Indicator Type |
|-------|----------|----------------|
| Pre-token (< 2s) | No indicator needed | None |
| Pre-token (2-10s) | Bouncing dots or pulse | Indeterminate |
| Pre-token (> 10s) | Consider showing elapsed time | Semi-determinate |
| Streaming | Real-time text | Streaming text |
| Collapse transition | 200-300ms | CSS transition |

(Source: NNGroup progress indicator guidelines)

---

## 6. When to Show Thinking Expanded vs Collapsed

### 6.1 Expand by Default When:

- **Currently streaming**: User should see reasoning arrive in real-time (consensus across all implementations)
- **User explicitly requested reasoning**: If the user asked "show your thinking" or "explain your reasoning"
- **Debugging/developer mode**: In development tools or API playgrounds
- **Short reasoning**: Under ~100 tokens, expansion has minimal visual cost

### 6.2 Collapse by Default When:

- **Generation is complete**: All major platforms collapse after thinking finishes
- **Long reasoning traces**: Especially for deep reasoning (10K+ tokens) that would overwhelm the viewport
- **Mobile viewports**: Screen real estate is precious
- **Subsequent views**: When revisiting a conversation, collapsed is the expected state

### 6.3 Vercel AI Chatbot Pattern (Recommended)

```typescript
// Track whether streaming has occurred for this message
const [hasBeenStreaming, setHasBeenStreaming] = useState(isLoading);

useEffect(() => {
  if (isLoading) setHasBeenStreaming(true);
}, [isLoading]);

// Expanded during streaming, collapsed after
// But if user opened it manually, respect that
<Reasoning defaultOpen={hasBeenStreaming} isStreaming={isLoading}>
```

This pattern:
- Shows thinking expanded during generation
- Auto-collapses when generation completes
- Lets the user manually re-expand at any time
- Does not auto-collapse if the user manually expanded

---

## 7. Accessibility Considerations

### 7.1 WAI-ARIA Disclosure Pattern (Required)

The thinking block is a disclosure widget and MUST follow the WAI-ARIA disclosure pattern.

**Required attributes:**
```html
<!-- Toggle button -->
<button
  role="button"
  aria-expanded="true|false"
  aria-controls="thinking-content-id"
>
  Thought for 12 seconds
</button>

<!-- Collapsible content -->
<div
  id="thinking-content-id"
  role="group"
  aria-label="Model reasoning"
  aria-hidden="true|false"
>
  ... reasoning text ...
</div>
```

**Keyboard interactions:**
- `Enter`: Toggle expand/collapse
- `Space`: Toggle expand/collapse

(Source: W3C WAI-ARIA Disclosure Pattern)

### 7.2 Screen Reader Hiding (LibreChat Pattern)

When collapsed, thinking content MUST be hidden from screen readers to prevent cluttered reading flows.

```tsx
<div
  role="group"
  id={contentId}
  aria-label="Model reasoning"
  aria-hidden={!isExpanded}  // Key: hide from AT when collapsed
>
  {reasoningContent}
</div>
```

(Source: LibreChat PR #11927)

### 7.3 TTS (Text-to-Speech) Considerations

- By default, TTS should NOT read thinking content (it's internal reasoning, not the response)
- LibreChat offers a user toggle for including thinking in TTS (Issue #11381)
- When thinking contains code blocks, TTS readers can produce garbled output

### 7.4 Motion Sensitivity

- Provide `prefers-reduced-motion` support for all animations
- Bouncing dots, pulse, and shimmer should be disabled or simplified

```css
@media (prefers-reduced-motion: reduce) {
  .thinking-dots span,
  .thinking-label,
  .thinking-skeleton {
    animation: none;
  }
  .thinking-content {
    transition: none;
  }
}
```

### 7.5 Native HTML Alternative

The `<details>` and `<summary>` elements provide built-in accessibility:
- Screen reader compatible with no extra ARIA needed
- Keyboard navigation (Space/Enter to toggle)
- `toggle` event for state tracking
- `name` attribute for accordion grouping

```html
<details name="thinking" open>
  <summary>Thought for 12 seconds</summary>
  <div class="thinking-content">
    ... reasoning text ...
  </div>
</details>
```

(Source: MDN Web Docs, W3C WAI-ARIA Disclosure Pattern)

---

## 8. Streaming Architecture Best Practices

### 8.1 Separate Thinking from Response Tokens

All providers send thinking tokens before response tokens. Your stream handler must:

1. Detect when thinking starts (first `thinking_delta` or `<think>` tag)
2. Accumulate thinking tokens into a separate buffer
3. Detect transition to response content (first `text_delta` or `</think>` tag)
4. Accumulate response tokens into the main buffer
5. Trigger UI state transition from "thinking" to "responding"

### 8.2 Non-Blocking Rendering

For React, use `useTransition` to prevent thinking token updates from blocking UI:

```tsx
const [isPending, startTransition] = useTransition();

function onThinkingDelta(token: string) {
  startTransition(() => {
    setReasoning(prev => prev + token);
  });
}
```

This keeps the UI responsive during rapid token streaming.

(Source: React useTransition documentation)

### 8.3 Token Parsing for Open Models

For models that use `<think>` tags inline:

```typescript
function parseThinkingTags(text: string): { thinking: string; content: string } {
  const thinkMatch = text.match(/<think>([\s\S]*?)<\/think>/);
  if (thinkMatch) {
    return {
      thinking: thinkMatch[1].trim(),
      content: text.replace(/<think>[\s\S]*?<\/think>/, '').trim(),
    };
  }
  // Handle streaming case where </think> hasn't arrived yet
  const openThinkMatch = text.match(/<think>([\s\S]*)/);
  if (openThinkMatch) {
    return {
      thinking: openThinkMatch[1].trim(),
      content: '',
    };
  }
  return { thinking: '', content: text };
}
```

### 8.4 Avoiding Fragmentation (Open WebUI Lesson)

Open WebUI v0.8.0 had a critical bug where reasoning traces were "visually split to many parts" causing browser hangs. The root cause: each streaming token created a new DOM node instead of appending to the existing thinking block.

**Fix pattern:**
- Accumulate tokens into a single string buffer
- Re-render the entire thinking block content on each update (virtual DOM diffing handles this efficiently)
- Never create a new DOM element per token

(Source: Open WebUI Issue #21348)

---

## 9. Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Thinking block stays expanded after completion | Missing auto-collapse logic | Use `useEffect` watching `isStreaming` to trigger collapse |
| Browser hangs during long reasoning streams | Creating new DOM nodes per token | Accumulate into string buffer, re-render single element |
| Screen readers read collapsed thinking content | Missing `aria-hidden` on collapsed state | Set `aria-hidden={!isExpanded}` dynamically |
| TTS reads thinking aloud | Thinking content not excluded from TTS pipeline | Filter thinking blocks before TTS processing |
| Thinking block flickers on short reasoning | Expand/collapse happens too quickly | Add a minimum display time (e.g., 500ms) before auto-collapse |
| Raw `<think>` tags shown to user | Not parsing open-model thinking tags | Always parse/strip thinking tags before rendering |
| Layout shifts during streaming | Thinking block height changes push content | Use `max-height` with overflow scroll, or reserve space |
| Duplicate messages with thinking models | Streaming handler counts thinking and response as separate messages | Ensure single message accumulates both thinking and text parts |
| Animations cause accessibility issues | No `prefers-reduced-motion` support | Always include reduced motion media query |
| Multi-turn context breaks | Sending `reasoning_content` back in DeepSeek context | Strip reasoning from context; for Claude, preserve `signature` field |

---

## 10. Design System Integration

### 10.1 Visual Hierarchy

Thinking content should be visually subordinate to the main response:

```
Response text (primary):  font-size: 1rem,   color: foreground,        font: sans-serif
Thinking text (secondary): font-size: 0.875rem, color: muted-foreground, font: monospace
Thinking header (tertiary): font-size: 0.8rem,  color: muted-foreground, font: sans-serif
```

### 10.2 Color Coding

Common approach: use a subtle background tint to differentiate thinking from response.

```css
.thinking-content {
  background: hsl(var(--muted) / 0.5);
  border-left: 2px solid hsl(var(--muted-foreground) / 0.3);
}
```

### 10.3 Icon Patterns

| State | Icon | Animation |
|-------|------|-----------|
| Pre-thinking | Brain / Sparkle | Pulse |
| Actively thinking | Brain / Sparkle | Spin or pulse |
| Collapsed (complete) | Chevron right | None |
| Expanded (complete) | Chevron down | None |

---

## 11. Framework-Specific Component Libraries

### 11.1 React (shadcn/ui + Radix)

Use the Collapsible primitive from Radix UI via shadcn/ui:

```bash
pnpm dlx shadcn@latest add collapsible
```

```tsx
import { Collapsible, CollapsibleContent, CollapsibleTrigger }
  from "@/components/ui/collapsible";
```

Radix Collapsible provides:
- `defaultOpen`, `open`, `onOpenChange` props
- `forceMount` for animation control
- `[data-state]` attribute ("open" | "closed") for CSS styling
- CSS variables `--radix-collapsible-content-width/height` for animations
- Full WAI-ARIA Disclosure pattern compliance

(Source: shadcn/ui and Radix UI documentation)

### 11.2 Svelte

Use native `<details>` element with Svelte transitions:

```svelte
{#if isOpen}
  <div transition:slide={{ duration: 200 }}>
    {reasoning}
  </div>
{/if}
```

Svelte transitions run off the main thread via web animations for optimal performance.

(Source: Svelte transition documentation)

### 11.3 Vercel AI SDK

The SDK provides first-class reasoning support:

```typescript
// Server: enable reasoning forwarding
result.toUIMessageStreamResponse({ sendReasoning: true });

// Client: access reasoning parts
message.parts.filter(p => p.type === 'reasoning');
```

(Source: Vercel AI SDK chatbot documentation)

---

## 12. Summary: The Canonical Thinking Block

Based on analysis of all major platforms and open-source implementations, the canonical thinking block pattern is:

1. **Before tokens arrive**: Show "Thinking" with animated dots (bouncing or pulsing)
2. **During reasoning streaming**: Expand the thinking block, stream reasoning text with monospace font, muted color, max-height scroll area
3. **Transition to response**: Keep thinking block visible but start rendering the main response below it
4. **After generation completes**: Auto-collapse the thinking block to a single-line header showing "Thought for Xs" with expand/collapse toggle
5. **User interaction**: Allow manual expand/collapse at any time with full keyboard and screen reader support
6. **Copy support**: Provide a "Copy thoughts" button (visible on hover/focus) for the reasoning content

This pattern is implemented with minor variations in ChatGPT, Claude.ai, DeepSeek Chat, Gemini, LibreChat, Open WebUI, LobeChat, and the Vercel AI Chatbot template.

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Anthropic Extended Thinking Docs](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking) | API Docs | Definitive reference for Claude thinking API |
| [DeepSeek Reasoning Model API](https://api-docs.deepseek.com/guides/reasoning_model) | API Docs | DeepSeek reasoning_content format |
| [Google Gemini Thinking Docs](https://ai.google.dev/gemini-api/docs/thinking) | API Docs | Gemini thought summaries and thinkingLevel |
| [Vercel AI SDK Chatbot Docs](https://ai-sdk.dev/docs/ai-sdk-ui/chatbot) | Framework Docs | First-class reasoning parts support |
| [LibreChat PR #11927](https://github.com/danny-avila/LibreChat/pull/11927) | Implementation | Screen reader accessibility for thinking blocks |
| [LibreChat PR #11142](https://github.com/danny-avila/LibreChat/pull/11142) | Implementation | Thinking block UI polish and FloatingThinkingBar |
| [Open WebUI Issue #21348](https://github.com/open-webui/open-webui/issues/21348) | Bug Report | Reasoning trace fragmentation and fix |
| [WAI-ARIA Disclosure Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/disclosure/) | Spec | Accessibility requirements for collapsible widgets |
| [MDN `<details>` Element](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/details) | Reference | Native HTML disclosure widget |
| [Radix UI Collapsible](https://www.radix-ui.com/primitives/docs/components/collapsible) | Component Library | React collapsible primitive with full a11y |
| [shadcn/ui Collapsible](https://ui.shadcn.com/docs/components/collapsible) | Component Library | shadcn wrapper around Radix Collapsible |
| [NNGroup Progress Indicators](https://www.nngroup.com/articles/progress-indicators/) | UX Research | Timing guidelines for loading indicators |
| [NNGroup Skeleton Screens](https://www.nngroup.com/articles/skeleton-screens/) | UX Research | Shimmer and skeleton loading patterns |
| [CSS Animate to height:auto](https://developer.chrome.com/docs/css-ui/animate-to-height-auto) | CSS Guide | Modern CSS collapse animations |
| [CSS Exclusive Accordion](https://developer.chrome.com/docs/css-ui/exclusive-accordion) | CSS Guide | Native HTML accordion with details[name] |
| [React useTransition](https://react.dev/reference/react/useTransition) | React Docs | Non-blocking UI updates for streaming |
| [Svelte Transitions](https://svelte.dev/docs/svelte/transition) | Svelte Docs | Off-thread animations for collapsible blocks |
| [Anthropic Visible Thinking](https://www.anthropic.com/news/visible-extended-thinking) | Announcement | Design philosophy behind showing thinking |
| [DeepSeek-R1 Paper](https://arxiv.org/abs/2501.12948) | Research Paper | Chain-of-thought reasoning emergence |
| [web.dev Loading Bar](https://web.dev/articles/building/a-loading-bar-component) | CSS Guide | Indeterminate loading animation patterns |

---

*This guide was synthesized from 24 sources. See `resources/ai-thinking-ui-patterns-sources.json` for full source list.*
