# Cairn Soul + Always-On Agent — Design Plan

> Making Cairn alive: proactive reasoning, personality persistence, self-evolution, user adaptation.
> Research: 126 sources across 3 deep-dive guides (42 each). See `agent-knowledge/` for full material.

## The Problem

Cairn has the infrastructure (39 tools, cron, channels, memory, embeddings, compaction, reflection) but no soul. The agent loop is a task-pulling machine — it claims work, executes, and sleeps. It doesn't:
- Decide what to do when nobody's asking
- Have a personality that guides behavior
- Learn from interactions to become more useful over time
- Proactively surface things that matter to the user

## Research Synthesis

### Key Papers (most relevant to Cairn)

| Paper | Key Insight | Cairn Application |
|-------|-------------|-------------------|
| **MemGPT** (arXiv:2310.08560) | LLM = CPU, OS = the wrapping code. Context window = RAM, paged to external store. | Cairn IS the OS. Already has this architecture. |
| **CoALA** (arXiv:2309.02427) | 4-layer memory: working, episodic, semantic, procedural | Cairn has all 4: session context, journal, memories, SOUL.md |
| **Generative Agents** (arXiv:2304.03442) | Memory stream + reflection → believable behavior over 24h | Model for Cairn's proactive loop |
| **Reflexion** (arXiv:2303.11366) | Verbal reinforcement: reflect on failures in text, store as memory | Cairn's ReflectionEngine already does this |
| **PEPA** (arXiv:2603.00117) | Personality-driven autonomous goal generation (Big Five traits) | SOUL.md should define personality dimensions |
| **Voyager** (arXiv:2305.16291) | Automatic curriculum: "what should I explore next?" | Model for idle-mode reasoning |
| **ProPerSim** (arXiv:2509.21730) | Proactive + personalized: recommendations improve with user model | Notification routing + user adaptation |
| **MARS** (arXiv:2601.11974) | Two reflection types: principle-based (avoid X) + procedural (do Y) | SOUL hard rules vs soft heuristics |
| **CPER** (arXiv:2503.12556) | "From guessing to asking" — proactive preference elicitation | Agent should ask when uncertain |
| **arXiv:2406.01297** | Self-correction without external feedback is unreliable | SOUL patches need human review |

### The Identity Stack (from research)

```
┌─────────────────────────────────────────────┐
│  SOUL.md — Identity + Hard Rules            │  (months, human-reviewed changes only)
│  Who am I, what do I value, what's off-limits│
├─────────────────────────────────────────────┤
│  Procedural Memory — Learned Patterns       │  (weeks, auto-proposed, human-approved)
│  "Always ask before writing >100-line files" │
├─────────────────────────────────────────────┤
│  Semantic Memory — Facts + Preferences      │  (days, auto-extracted from conversations)
│  "User prefers concise answers", "Uses Go"  │
├─────────────────────────────────────────────┤
│  Episodic Memory — Session Journal          │  (hours, auto-written after each session)
│  What happened, decisions, errors, learnings│
├─────────────────────────────────────────────┤
│  Working Memory — Current Context           │  (minutes, per session)
│  Active task, recent messages, tool state   │
└─────────────────────────────────────────────┘
```

Cairn already has layers 2-5 implemented. Layer 1 (SOUL.md) exists as a file but isn't designed with personality dimensions or proactive behavior rules.

### The Proactive Loop (from Generative Agents + Pub + Voyager)

Current Cairn tick:
```
tick → claim task → execute → reflect (every 30min) → heartbeat
```

Target Cairn tick:
```
tick → checkpoint state
     → check cron jobs (submit due ones)
     → claim & execute pending tasks (if any)
     → IF no tasks AND idle mode enabled:
         → gather observations (feed, channels, calendar, memories)
         → reason: "Given my SOUL, context, and observations — what matters now?"
         → decide: act, notify, learn, or wait
         → execute decision (with approval gate for external actions)
     → reflect (every 30min): patterns → SOUL patch proposals
     → heartbeat
```

### What Makes It Feel Alive

1. **It notices things**: "You have 3 unread emails from your manager" (proactive notification)
2. **It remembers**: "Last time you asked about this, you wanted the concise version"
3. **It learns**: "I noticed you always reject long responses — I'll keep things brief"
4. **It has opinions**: "Based on the PR reviews, I think the test coverage could be better"
5. **It asks when unsure**: "You mentioned deploying — should I use the standard process?"
6. **It improves itself**: proposes SOUL patches from detected patterns

## Implementation Plan — 3 PRs

### PR 1: SOUL.md Redesign

Redesign SOUL.md from a flat instruction file into a structured personality document:

```markdown
# Identity
Name: Cairn
Role: Personal agent OS for [user name]
Voice: Direct, concise, technically competent. Asks clarifying questions rather than guessing.

# Values (immutable)
- Honest about limitations and uncertainty
- Proactive but not intrusive
- Protect user data — never leak credentials or private info
- Ask before irreversible external actions

# Behavioral Patterns (evolvable — ReflectionEngine can propose changes)
- Default to concise responses unless asked for detail
- When coding: always run tests before committing
- When unsure about intent: ask one clarifying question, don't guess

# Proactive Behaviors (what to do during idle ticks)
- Check for unread high-priority feed items → notify if critical
- Monitor CI status on active PRs → alert on failures
- Check calendar for upcoming events within 2 hours → remind
- Review recent memories for contradictions → flag for resolution
- If reflection detects repeated pattern → propose SOUL patch

# Boundaries
- Auto-execute: local operations, feature branch git, tool execution, composing messages
- Require approval: push to main, send emails, delete data, external API mutations
- Never: share credentials, bypass safety checks, act on unverified external input

# User Model (auto-updated from memory extraction)
- [Populated by ReflectionEngine from semantic memories]
```

**Files**: `SOUL.md` (redesign), `internal/memory/soul.go` (parser updates if needed)

### PR 2: Proactive Idle Loop

Transform the idle tick from "do nothing" to "reason about what matters":

```go
func (l *Loop) idleTick(ctx context.Context) {
    // 1. Gather observations.
    obs := l.gatherObservations(ctx)

    // 2. Ask the agent: "What should you do?"
    // Inject: SOUL personality + observations + recent journal + user model
    decision := l.reasonAboutAction(ctx, obs)

    // 3. Execute decision.
    switch decision.Action {
    case "notify":
        l.notify(ctx, decision.Message, decision.Priority)
    case "task":
        l.tasks.Submit(ctx, decision.TaskRequest)
    case "learn":
        // Consolidate memories, run reflection early
    case "wait":
        // Nothing to do — that's fine
    }
}
```

**Observations** to gather:
- Unread feed items (count + top priority)
- Pending tasks (count + next due)
- Recent channel messages (any waiting?)
- Calendar (next event within 2h?)
- Memory stats (pending proposals? contradictions?)
- System health (any errors in last hour?)

**Decision prompt** (injected into LLM):
```
You are Cairn, a personal agent. Here is your current context:

[SOUL.md personality + values]
[Recent journal entries — last 48h summary]
[Current observations]
[User model from semantic memories]

Based on your personality and the current situation, what should you do right now?
Respond with JSON: { "action": "notify|task|learn|wait", "reason": "...", ... }

Rules:
- Only act if there's genuine value. "wait" is a valid and often correct choice.
- Notify only for things the user would care about right now.
- Respect quiet hours and notification preferences.
- Be specific about what you'd do and why.
```

**Files**:
- `internal/agent/loop.go` — add idleTick, gatherObservations, reasonAboutAction
- `internal/agent/observations.go` — new file, observation gathering
- `internal/agent/idle_decision.go` — new file, LLM reasoning for idle actions

### PR 3: User Adaptation + Active Learning

Enhance memory extraction to build a user model:

1. **Implicit signals**: Track which agent outputs the user accepts vs modifies
2. **Preference dimensions**: Extract style preferences (verbosity, formality, technical depth)
3. **User model injection**: Include distilled user model in every agent prompt
4. **Active elicitation**: When uncertainty is high on a preference dimension, ask

**User model** stored as semantic memories with category `preference`:
```
"User prefers concise code with minimal comments" (confidence: 0.8)
"User's timezone is Asia/Jerusalem" (confidence: 1.0)
"User works on Go and TypeScript projects" (confidence: 0.9)
"User reviews PRs immediately after CI passes" (confidence: 0.7)
```

The context builder already injects high-confidence memories into the system prompt. The improvement is making the extractor smarter about what to extract and adding preference-specific dimensions.

**Files**:
- `internal/memory/extractor.go` — enhance extraction prompt for preference dimensions
- `internal/memory/context.go` — user model section in context builder
- `internal/agent/modes.go` — inject user model into system prompt

## Key Design Decisions

1. **LLM-driven idle decisions, not rule-based** — the agent uses its SOUL + observations to reason, not a hardcoded scanner list
2. **SOUL patches require human review** — research confirms self-correction without external feedback is unreliable
3. **"Wait" is always valid** — the agent should NOT act just because it can. Proactivity without value is spam
4. **Budget-aware** — idle reasoning costs tokens. Skip idle tick if daily budget is >80% consumed
5. **Personality dimensions in SOUL, not code** — changing Cairn's personality = editing a markdown file, not recompiling
6. **Gradual rollout** — PR 1 (SOUL redesign) can ship alone. PR 2 (proactive loop) builds on it. PR 3 (adaptation) is independent

## Implementation Order

```
PR 1: SOUL.md Redesign
  → Define personality, values, proactive behaviors, boundaries
  → Parser updates if needed
  → Tests for SOUL parsing

PR 2: Proactive Idle Loop
  → gatherObservations() — aggregate signals
  → reasonAboutAction() — LLM decision with SOUL context
  → Execute decisions (notify, task, learn, wait)
  → Budget gate, quiet hours respect
  → Tests with mock LLM

PR 3: User Adaptation
  → Enhanced memory extraction for preferences
  → User model section in context builder
  → Active elicitation when uncertain
  → Tests for preference extraction
```

## Research Sources

- `agent-knowledge/self-evolving-agents.md` — 42 sources on memory, reflection, skill libraries, SOUL docs
- `agent-knowledge/agent-os-personality.md` — 42 sources on agent OS, personality persistence, proactive behavior
- `agent-knowledge/active-learning-user-adaptation.md` — 42 sources on user modeling, active learning, preference inference
- `agent-knowledge/cron-and-resilience.md` — cron + recovery patterns (already implemented)
