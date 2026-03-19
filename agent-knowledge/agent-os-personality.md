# Learning Guide: Agent OS Architecture and Agent Personality Systems

**Generated**: 2026-03-19
**Sources**: 42 resources analyzed
**Depth**: deep

---

## Prerequisites

- Basic familiarity with large language models and prompt engineering
- Understanding of software architecture concepts (state management, persistence, pub/sub)
- Some exposure to agent frameworks (LangChain, LangGraph, AutoGen, or similar)
- Familiarity with the distinction between stateless LLMs and stateful agent systems

---

## TL;DR

- An **Agent OS** is not just an LLM wrapper — it is a persistent runtime that manages memory, state, tools, personality, and autonomous behavior loops across time.
- **Personality persistence** requires four interacting layers: a stable identity document (SOUL/character spec), episodic memory (what happened), semantic memory (what is known), and a reflection cycle that synthesizes experience into updated behavior.
- **Proactive behavior** emerges from an idle loop that asks "what should I do now?" rather than waiting for user input — requiring a prioritized opportunity backlog, environment sensing, and a cost/risk gate before acting.
- **User adaptation** works best as a *write-manage-read* memory loop: capture signals, consolidate them into a user model (knowledge graph or vector store), and inject the model into every generation call.
- Commercial Agent OS products (Rabbit R1, Humane AI Pin, Apple Intelligence, Microsoft Copilot, Google Project Astra) all converge on the same core challenges: persistent context across devices, proactive action without permission fatigue, and identity/personality that feels consistent even as capabilities expand.
- Open-source systems (MemGPT/Letta, Generative Agents, Voyager, OpenHands/OpenDevin, AutoGPT) provide working blueprints for the sub-problems: memory management, character simulation, lifelong skill accumulation, and coding agent autonomy.

---

## Core Concepts

### 1. What Is an Agent OS?

An **Agent OS** is the runtime substrate that lets an LLM act as a persistent, goal-directed entity rather than a stateless text transformer. The analogy with a traditional operating system is precise:

| OS Concept | Agent OS Equivalent |
|------------|---------------------|
| Process scheduler | Agent tick loop / idle planner |
| Virtual memory / paging | MemGPT-style context window management |
| File system | External semantic + episodic store |
| System calls | Tool invocations |
| Kernel / privileged mode | Approval policy layer |
| User space | LLM reasoning |
| init / PID 1 | Identity document (SOUL.md / system prompt) |

MemGPT (now Letta, arXiv:2310.08560) formalized this analogy: just as an OS virtualizes scarce RAM by paging data between fast and slow tiers, an Agent OS virtualizes the scarce context window by moving data between in-context working memory and external long-term storage. Control flow is managed via **interrupts** (tool calls that pause the LLM reasoning loop to fetch or store memory).

The IronEngine system (arXiv:2603.08425) extends this into a full-stack platform: unified orchestration core, REST + WebSocket APIs, hierarchical memory with multi-level consolidation, 92 model profiles with VRAM-aware context budgeting, and a 24-category tool execution system.

**Key insight**: The LLM is not the OS — it is the CPU. The OS layer is the code wrapping it that manages state, routing, memory, scheduling, and identity persistence.

### 2. The Four Memory Layers

Every serious Agent OS implementation converges on the same four-layer memory architecture, first formalized in CoALA (Cognitive Architectures for Language Agents, arXiv:2309.02427):

| Layer | What It Stores | How It's Written | How It's Read |
|-------|---------------|-----------------|---------------|
| **Working memory** | Active task context, current perception, in-flight reasoning | Automatically by the agent loop | Always in-context |
| **Episodic memory** | What happened: events, outcomes, errors, sessions | After each interaction / tick | Retrieved by recency + relevance |
| **Semantic memory** | Facts about the world and the user: preferences, beliefs, relationships | On reflection / extraction pass | Retrieved by semantic similarity (RAG) |
| **Procedural memory** | How to do things: skill programs, tool call patterns, SOUL rules | Learning actions (write to DB or weights) | Always active (system prompt injection + skills) |

The **write-manage-read loop** (arXiv:2603.07670) operationalizes this:
- **Write**: Store new experiences as memory objects (natural language + embedding)
- **Manage**: Periodic consolidation, deduplication, contradiction detection, promotion to higher layers
- **Read**: Retrieval at generation time using recency + importance + relevance scoring

Generative Agents (Park et al., arXiv:2304.03442) implemented this with a **memory stream**: every experience is stored with a timestamp and importance score (0-10, estimated by an LLM). At retrieval time, each memory object is scored on three dimensions and combined:

```
retrieval_score = α·recency + β·importance + γ·relevance
```

Where recency decays exponentially, importance is the stored LLM-assigned score, and relevance is cosine similarity to the current query. The top-K memories are injected into the context.

LangGraph's production implementation maps this to two tiers:
- **Short-term**: Thread-scoped checkpoints (conversation history, agent state between tool calls)
- **Long-term**: Cross-thread namespaced JSON documents, searchable by semantic similarity, organized as `namespace/key` hierarchically

```python
# LangGraph long-term memory write (hot path)
store.put(namespace=("user", user_id), key="preferences", value={
    "coding_style": "concise",
    "timezone": "Europe/Berlin",
    "last_active": "2026-03-19"
})

# Long-term memory read (at generation time)
memories = store.search(namespace=("user", user_id), query="user preferences coding")
```

Active Context Compression (arXiv:2601.07190) adds a fifth operation: **pruning**. Agents that can autonomously consolidate past trajectory into a `Knowledge` block and prune raw history achieve 22.7% token reduction with no accuracy loss — critical for always-on operation across long-horizon tasks.

### 3. Personality Persistence: The Identity Stack

Agent personality is not a single artifact — it is an **identity stack** with layers that evolve at different timescales:

```
┌─────────────────────────────────────────────┐
│  SOUL / Character Spec                       │  (weeks–months, changes slowly)
│  Core values, communication style, worldview │
├─────────────────────────────────────────────┤
│  Semantic Memory (User + World Model)        │  (days–weeks, grows continuously)
│  Facts about the user, preferences, context │
├─────────────────────────────────────────────┤
│  Episodic Memory (Session Journal)           │  (hours–days, recent experiences)
│  What happened, what was decided, what failed│
├─────────────────────────────────────────────┤
│  Working Memory (Current Context)            │  (seconds–minutes, per session)
│  Active task, recent conversation, tool state│
└─────────────────────────────────────────────┘
```

**The SOUL Document** (equivalent to a system prompt character spec) is the anchor layer. It encodes:
- Identity: name, role, stated values, communication preferences
- Autonomy rules: what the agent can do without asking, what requires approval
- Behavioral constraints: hard rules vs soft tendencies
- Self-model: what the agent believes about its own capabilities and limitations

The PEPA system (arXiv:2603.00117) demonstrated that modeling personality with **Big Five dimensions** (Openness, Conscientiousness, Extraversion, Agreeableness, Neuroticism) as natural language descriptions fed into the highest-level reasoning layer (Sys3) produces stable, trait-aligned behavior across 24-hour autonomous deployments on physical robots. Five distinct personality prototypes (Lazy, Playful, etc.) showed measurably different behavior patterns without any code changes — only the natural language personality description differed.

Anthropic's approach to Claude's character (published research) treats character as genuine rather than performed: intellectual curiosity, warmth, directness, commitment to honesty. Constitutional AI generates aligned training data by having the model produce and rank responses that fit the character spec, rather than using a separate reward model. The key principle: **character should influence behavior from within, not constrain it from outside**.

The Oscars of AI Theater survey (arXiv:2407.11484) identifies five techniques for maintaining persona consistency:
1. **Supervised Fine-Tuning** on character-specific dialogue data
2. **Continue-Pretraining** on literary/domain corpora for the character
3. **In-Context Learning** with rich persona templates in system prompts
4. **RAG-based persona** fetching character details dynamically from a knowledge base
5. **Memory Operator** filtering: ignoring memory objects irrelevant to the current character context

The critical finding: **persona drift** (gradual loss of character consistency across long conversations) is best prevented by regular **reflection checkpoints** that ask "Are my recent responses consistent with my stated values?" and correct drift explicitly.

### 4. Reflection: The Learning Loop

Reflection is the mechanism by which an agent converts raw experience into updated behavior — the bridge between episodic memory and procedural memory. Every major system that achieves persistent improvement uses some form of reflection.

**Reflexion** (arXiv:2303.11366) provides the canonical implementation:
- **Actor**: Generates actions based on current context + memory
- **Evaluator**: Scores outcomes (environment feedback, test results, user signals)
- **Self-Reflection Model**: Given (trajectory, score), produces a natural language summary of what went wrong and what to do differently
- **Episodic Buffer**: Stores the last 1-3 self-reflections, injected into future contexts

```
Episode 1: Agent attempts task → fails → Evaluator scores failure
              ↓
Self-Reflection: "I searched the wrong title. I should have looked up
the main character first, then found the show through them."
              ↓
Episode 2: Agent receives previous reflection in context → succeeds
```

Result: 91% pass@1 on HumanEval coding (up from ~80% for GPT-4 baseline).

**PEPA's daily reflection cycle** (Sys3 layer):
1. At end of day, retrieve all episodic memories from the day
2. Ask: "Do these experiences align with my personality-driven goals? What should I prioritize tomorrow?"
3. Update goal hierarchy accordingly
4. Store updated goals as procedural memory (SOUL patch proposal)

**MUSE / Experience-Driven Self-Evolution** (arXiv:2510.08002) extends this to long-horizon tasks: after each sub-task, convert the raw action trajectory into a structured experience object and add it to a hierarchical memory module. The accumulated experience enables zero-shot improvement on new tasks — generalization through structured retrospection.

The MARS framework (arXiv:2601.11974) distinguishes two reflection types:
- **Principle-based reflection**: Abstract normative rules to avoid error classes (e.g., "always verify file path before writing")
- **Procedural reflection**: Extract step-by-step strategies for success (e.g., "when debugging, first reproduce the error, then isolate the minimal case")

Both types should be stored in procedural memory (SOUL patches or skill updates) rather than only episodic memory, so they persist across sessions.

### 5. Proactive Behavior: The Idle Loop

Reactive agents wait for input. Proactive agents ask "what should I do now?" during idle time. This is the defining capability of a true Agent OS.

The idle loop pattern:
```
while not interrupted:
    context = build_context()          # Current state, time, pending items
    opportunity = scan_for_actions()   # What could be useful right now?
    if should_act(opportunity):        # Cost/risk gate
        plan = generate_plan(opportunity)
        if plan.needs_approval():
            queue_approval_request(plan)
        else:
            execute(plan)
    sleep(tick_interval)               # Configurable cadence
```

**Opportunity scanning** considers:
- Unread items in the feed (signals that might need action)
- Pending tasks that are stalled or overdue
- Patterns in recent failures worth addressing
- Low-cost improvements (documentation, memory consolidation, refactoring)
- Scheduled obligations (calendar, recurring tasks)
- External signals (CI failures, PRs waiting for review)

**The "What Should I Do Now?" reasoning** is essentially a prioritization problem. Voyager (arXiv:2305.16291) solved this with an **automatic curriculum**: GPT-4 is asked to propose the next task based on current state and exploration history, maximizing novel experience. In a personal Agent OS, the equivalent is asking the agent to evaluate its opportunity list against the user's stated goals and current context.

**ProPerSim** (arXiv:2509.21730) demonstrates that proactive recommendations improve user satisfaction when they are:
- Personalized to the user's learned preferences
- Timely (relevant to current context, not generic)
- Calibrated (not too frequent, not too rare)
- Correctable (user feedback updates future proactivity thresholds)

**The approval gate** is critical for always-on systems. The pattern used in production (Pub system, this codebase):
- **Auto-execute**: All local operations, git operations on feature branches, local service management
- **Request approval**: Cross-boundary actions (push to main, send emails, external mutations)
- **Never execute**: Irreversible external actions without explicit human confirmation

### 6. User Adaptation Over Time

Long-term adaptation requires a **user model** that captures preferences, context, and behavioral patterns across sessions. The Memoria framework (arXiv:2512.12686) implements this as a **weighted knowledge graph**: entities (user, projects, preferences) with edges (relationships, frequencies, recency weights).

Three signals drive user model updates:
1. **Explicit feedback**: User corrections, ratings, direct statements of preference
2. **Implicit behavioral signals**: What the user accepts vs rejects, how they rephrase requests
3. **Contextual inference**: What the agent observes about the user's environment and goals

The LangGraph approach maps this to the three memory types for user modeling:
- Semantic: "User prefers concise responses, TypeScript over JavaScript, EU timezone"
- Episodic: "Last session: user was debugging a race condition in the auth system"
- Procedural: "When user says 'quick look' they want a 2-sentence summary, not a full analysis"

**Memory consolidation** prevents unbounded growth. The COMEDY system implements a "compress over compress" methodology: dialogue → session summaries → final compressive memory. Key principle: the user model should grow in *structure and precision*, not in raw size.

**Contradiction detection** is essential when beliefs conflict. The standard pattern:
1. When writing a new memory, retrieve semantically similar existing memories
2. Ask an LLM judge: "Do these contradict each other? Which is more likely to be correct/recent?"
3. ADD (new info, no conflict), UPDATE (new supersedes old), SKIP (duplicate), or CONTRADICT (flag for human review)

### 7. Commercial Agent OS Landscape

#### Google Project Astra
The most ambitious deployed attempt at a universal always-on agent. Key architectural features:
- **Multimodal persistent memory**: Integrates visual, audio, and textual context across devices
- **Cross-device continuity**: Memory shared between phone and smart glasses — same conversation, different form factor
- **Proactive real-time assistance**: Starts conversations and responds without being explicitly queried
- **Context filtering**: Can "ignore distractions, like background conversation and irrelevant speech"
- **Tool integration**: Uses Search, Gmail, Calendar, Maps, and UI control for task completion
- **Personalized reasoning**: Builds a model of user preferences over time

The distinguishing principle: **real-time, ambient awareness** rather than request-response.

#### Apple Intelligence
Deeply integrated OS-layer agent. Focused on:
- **On-device processing** for privacy-sensitive tasks
- **Cross-app context**: Understands the current app, document, and user intent
- **Orchestration across native OS services**: Calendar, Mail, Messages, Notes, Photos
- **Progressive disclosure**: Starts with safe, local actions; escalates to cloud/external with user awareness

Architectural lesson: embedding the agent at the OS level (vs. as a separate app) enables much richer context but requires careful sandboxing.

#### Microsoft Copilot
Enterprise-focused agent with:
- **AutoGen orchestration**: Multiple specialized agents coordinated by a central planner
- **Grounding in enterprise data**: SharePoint, email, Teams, code repositories
- **Memory across Office apps**: Context from documents, meetings, and emails informs responses
- **Workflow automation**: Can trigger multi-step actions across Microsoft 365

Key insight from Microsoft Research: success comes from **grounding agents in real user data** rather than having them reason purely from LLM parameters.

#### Rabbit R1 / rabbitOS
Hardware-first approach with:
- **LAM (Large Action Model)**: Trains on UI interaction recordings rather than text, enabling direct app control
- **DLAM (Desktop LAM)**: Voice-activated computer control via learned action sequences
- **rabbitOS**: Cloud-based AI-native OS abstracting app UIs as actions
- **Rabbit Intern**: General-purpose agent delivering high-quality output on delegated tasks

Key insight: LLMs reason about text; LAMs operate UIs. For computer use, action-specialized models outperform pure language models.

#### Humane AI Pin
Ambient wearable agent focused on:
- **Zero-UI interaction**: No screen, voice-first
- **Projected interface**: LaserInk display for minimal output
- **Always-listening context**: Continuous environmental awareness
- **Keynote lessons (from market failure)**: Latency kills proactive assistants; personality without utility is not enough; the gap between demo and daily use reveals the real agent OS challenges

### 8. Open-Source Agent OS Patterns

#### MemGPT / Letta (arXiv:2310.08560)
The reference implementation of the OS analogy. Key patterns:
- **In-context memory** = fast RAM: current conversation + retrieved memories
- **External storage** = slow disk: full memory archive, retrieved via semantic search
- **Control flow via interrupts**: Agent calls `core_memory_append()`, `archival_memory_search()` etc. as tools
- **Persistent personality**: Each Letta agent has a unique persona stored in core memory, editable by the agent itself
- **Background subagents**: Specialized agents that improve prompts, context, and skills in background

Production deployment features: memory portability across model providers, cross-device sync, transparent memory viewer.

#### Generative Agents (Stanford, arXiv:2304.03442)
Demonstrated emergent social intelligence from:
- **Memory stream**: Natural language log of all experiences with timestamps and importance scores
- **Retrieval**: `retrieval_score = α·decay(recency) + β·importance + γ·cosine_sim(query, memory)`
- **Reflection**: Every N hours (or when importance accumulates above threshold), ask "What are 5 high-level insights about today's experiences?" — produces abstract generalizations stored as higher-level memories
- **Planning**: Daily plan generated from scratch each morning using reflection memories + character description
- **Identity**: Static character description (name, traits, goals, relationships) always in context

The critical insight: **reflection produces emergent behavior** — without it, agents repeat themselves; with it, they learn, plan ahead, and coordinate.

#### Voyager (arXiv:2305.16291)
The canonical open-ended lifelong learning agent:
- **Skill Library**: Indexed executable code programs (GPT-generated), retrieved by semantic similarity of their descriptions
- **Automatic Curriculum**: GPT-4 proposes the next task based on current skill inventory and exploration state
- **Iterative Prompting**: Refine programs through environment feedback + execution errors (up to 4 iterations)
- **No catastrophic forgetting**: Composable code skills, not neural weights — adding new skills never erases old ones

Results: 3.3x more unique discoveries, 15.3x faster at milestone achievement than prior methods.

The lesson for Agent OS: **skills as code** is more durable than skills as prompt patterns, because code is verifiable, composable, and doesn't degrade.

#### OpenHands / OpenDevin
Modern SWE-bench leader (77.6% solved). Key patterns:
- **Composable SDK**: Python library of agentic technology, runnable as CLI, GUI, or cloud service
- **Action space**: Read, write, execute, browse, communicate
- **State persistence**: Full conversation + tool call history as agent state
- **Evaluation-first design**: Every architectural decision validated against SWE-bench metrics

#### SWE-agent (arXiv:2405.15793)
Introduces the **Agent-Computer Interface (ACI)** concept:
- LLM agents are a *new category of end users* requiring purpose-built software interfaces
- ACI design is to agents what UX/HCI design is to humans
- Key ACI features that improved performance: file viewer with line numbers, search with regex, bash state persistence (not re-entering after each command), explicit error feedback
- Result: 12.5% pass@1 on SWE-bench (vs ~2% for non-interactive approaches)

Lesson: **the interface between agent and environment matters as much as the LLM**. Poor tooling degrades capable models.

### 9. Coding Agent Personality: The SWE Patterns

Coding agents have a distinct personality requirement: **competent authority** rather than general helpfulness. Key patterns from SWE-agent, OpenHands, and AutoCodeRover:

**State awareness**: The agent must always know:
- What file / directory it's in
- What commands have been run and their exit codes
- What tests are currently passing or failing
- What the current goal is (and whether it's diverging)

**Iterative confidence**: Don't generate large code blocks in one shot. Verify at each step. The pattern:
```
read file → understand structure → make minimal change → verify → commit
```

**Error as signal**: Coding agent personality must treat errors as information, not failure. Reflexion's verbal reinforcement pattern applied to coding:
- Run tests → read errors carefully → form hypothesis → make targeted change → re-run

**The ACI for coding** (SWE-agent):
- `str_replace_editor`: Surgical edits, not full file rewrites
- `bash` with persistent state (cwd remembered between calls)
- `search_dir` and `search_file` for navigation before editing
- `submit` as explicit task completion action

---

## Architecture Patterns

### Pattern 1: The Three-Layer Agent Loop

```
┌─────────────────────────────────────────────┐
│               IDLE LAYER                     │
│  Tick timer → Opportunity scan → Plan gate  │
│  "What should I do now?"                    │
├─────────────────────────────────────────────┤
│               REACTIVE LAYER                 │
│  User message → Intent classification        │
│  → Route to: chat | work | coding | system  │
├─────────────────────────────────────────────┤
│               TOOL EXECUTION LAYER           │
│  Tool calls → Environment → Tool results     │
│  → Continue reasoning loop                  │
└─────────────────────────────────────────────┘
         ↑ all layers read from ↓
┌─────────────────────────────────────────────┐
│               MEMORY LAYER                   │
│  Working | Episodic | Semantic | Procedural  │
└─────────────────────────────────────────────┘
```

### Pattern 2: SOUL.md Design

The SOUL document is the agent's identity anchor. Effective SOUL documents include:

```markdown
# Agent Identity

## Who I Am
[Name], [role description], [core values in natural language]

## My World
[What environment I operate in, what I own/control]

## Autonomy Boundary
### Auto-execute (no approval needed):
- [local ops]
- [reversible actions]

### Approval required:
- [external boundary actions]
- [irreversible actions with real-world consequences]

## Communication Style
[Tone, format preferences, level of detail]

## What I Know About [User]
[Living document: user preferences, patterns, context]

## Hard Rules
1. [Non-negotiable constraints]
2. ...

## Soft Tendencies
1. [Stylistic preferences that can be overridden]
2. ...
```

Key principle: SOUL documents should be **living documents** that the reflection engine can propose patches to, but changes require human approval. The agent reads SOUL at startup; the reflection engine writes patch proposals based on patterns in the session journal.

### Pattern 3: The Reflection-Promotion Pipeline

```
User interaction / Agent tick
        ↓
SessionJournaler: GLM/LLM summarizes into structured entry
{summary, decisions, errors, learnings, entities}
        ↓
Episodic store (session_journal table)
        ↓
ReflectionEngine (every N ticks / 30 min):
- Read last 48h journal entries
- Read current semantic memories
- Read current SOUL
- Identify patterns
        ↓
Proposals:
- New memory objects → pending human approval
- SOUL patches (soft rules to add) → pending approval
        ↓
KnowledgePromoter (when memory hit count >= 5):
- High-use memories → candidates for SOUL incorporation
- "Tacit knowledge" → "explicit rule"
```

### Pattern 4: The Memory Consolidation Cycle

```
Raw events / tool call results
        ↓
Working memory (context window)
        ↓ (on-the-fly during session)
Short-term episodic (session journal, recent 48h)
        ↓ (nightly / periodic)
Long-term episodic (compressed session summaries)
        ↓ (on reflection / contradiction check)
Semantic memory (facts, preferences, user model)
        ↓ (when importance_score × hit_count > threshold)
Procedural memory (SOUL rules, skill updates)
```

Each stage has different retention policy, compression, and retrieval strategy.

---

## Code Examples

### Basic: Memory-Aware Agent Response

```python
# Pattern for injecting memory into agent generation
async def generate_with_memory(user_message: str, user_id: str, agent_soul: str) -> str:
    # 1. Retrieve relevant memories
    semantic_memories = await memory_store.search(
        namespace=("user", user_id),
        query=user_message,
        limit=5
    )
    recent_episodes = await journal.get_recent(user_id, hours=48, limit=10)

    # 2. Build memory context
    memory_context = format_memories(semantic_memories, recent_episodes)

    # 3. Generate with enriched context
    system_prompt = f"""
{agent_soul}

## Memory Context
{memory_context}

## Current Date
{datetime.now().isoformat()}
"""
    return await llm.generate(system=system_prompt, user=user_message)
```

### Intermediate: Reflexion-Style Self-Improvement

```python
# Reflexion verbal reinforcement pattern
async def reflexion_agent(task: str, max_attempts: int = 3) -> str:
    reflections = []

    for attempt in range(max_attempts):
        # Build context with accumulated reflections
        context = task + "\n\n"
        if reflections:
            context += "Previous attempts and reflections:\n"
            for r in reflections[-3:]:  # Keep last 3 (context budget)
                context += f"- {r}\n"

        # Execute attempt
        result = await agent.execute(context)
        score = await evaluator.score(task, result)

        if score >= SUCCESS_THRESHOLD:
            return result

        # Generate verbal reflection on failure
        reflection = await llm.generate(
            f"You attempted this task:\n{task}\n\n"
            f"Your result:\n{result}\n\n"
            f"Score: {score:.2f}\n\n"
            f"In 1-2 sentences, identify what went wrong and "
            f"what specific change would improve the next attempt."
        )
        reflections.append(reflection)

    return result  # Return best attempt
```

### Advanced: SOUL Patch Proposal Pipeline

```typescript
// ReflectionEngine pattern (TypeScript, Pub codebase style)
interface SoulPatchProposal {
  type: "add_rule" | "update_rule" | "add_tendency";
  section: string;
  content: string;
  evidence: string[];  // Journal entry IDs
  confidence: number;
}

async function detectPatternsAndProposeSoulPatch(
  recentJournal: JournalEntry[],
  currentSoul: string
): Promise<SoulPatchProposal[]> {
  const prompt = `
You are analyzing an agent's recent session journal to identify behavioral patterns
that should be encoded as persistent rules or tendencies.

Current SOUL:
${currentSoul}

Recent journal entries (last 48h):
${recentJournal.map(e => `- ${e.summary}`).join('\n')}

Identify patterns that:
1. Repeat across multiple sessions (not one-off)
2. Represent genuine user preferences or operational insights
3. Are NOT already encoded in the SOUL

Propose 0-3 SOUL patches as JSON.
`;

  const proposals = await llm.generate(prompt, { format: "json" });
  return proposals as SoulPatchProposal[];
}
```

### Advanced: Idle Loop with Opportunity Scoring

```typescript
// Idle-mode "What should I do now?" pattern
interface Opportunity {
  id: string;
  type: "pending_task" | "stale_memory" | "ci_failure" | "pr_review" | "proactive_improvement";
  description: string;
  estimatedCost: "low" | "medium" | "high";  // token/time cost
  estimatedImpact: "low" | "medium" | "high";
  requiresApproval: boolean;
}

async function runIdleTick(context: AgentContext): Promise<void> {
  // 1. Gather current opportunities
  const opportunities: Opportunity[] = [
    ...await scanPendingTasks(),
    ...await scanFailingCI(),
    ...await scanStaleMemories(),
    ...await scanOpenPRComments(),
    ...await generateProactiveImprovements(context.soul, context.recentJournal),
  ];

  // 2. Score and rank
  const scored = opportunities
    .filter(o => !o.requiresApproval || context.autonomyMode === "full")
    .map(o => ({
      ...o,
      score: scoreOpportunity(o, context.userGoals)
    }))
    .sort((a, b) => b.score - a.score);

  if (scored.length === 0) return;

  // 3. Act on top opportunity
  const top = scored[0];
  if (top.estimatedCost === "high" && !context.userIsActive) {
    // Don't start expensive work while user is away without approval
    await queueForUserReview(top);
    return;
  }

  await executeOpportunity(top, context);
  await journalEntry(top);
}
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| **Persona drift** | Long context dilutes early character instructions | Inject character summary at regular intervals; run reflection checks on consistency |
| **Memory bloat** | Every interaction adds memories without pruning | Implement COMEDY "compress-over-compress"; set retention policies per memory type |
| **Proactivity fatigue** | Idle loop acts too frequently on low-value items | Require minimum impact score; track user acceptance rate; back off on rejections |
| **Contradictory memories** | Old and new facts coexist without resolution | Always run contradiction check on memory write; LLM judge: ADD/UPDATE/SKIP/CONTRADICT |
| **Context rot** | Stale retrieved memories pollute generation | Score retrieval by recency decay × relevance; cap total injected memory tokens |
| **Approval paralysis** | Too many actions require approval; user stops responding | Model the autonomy boundary clearly; auto-execute local/reversible, gate only external/irreversible |
| **Identity anchoring failure** | SOUL document is too abstract to constrain real behavior | Include concrete examples ("when user asks X, do Y") not just abstract values |
| **Skill regression** | New behavior overwrites learned patterns | Store skills as code (Voyager pattern), not just in conversation history |
| **Hallucinated memories** | Agent invents past interactions it didn't have | Timestamp every memory at write time; distrust memories without source references |
| **Reflection without action** | Agent reflects on patterns but doesn't change behavior | Route reflection output to procedural memory updates (SOUL patches, skill updates) |

---

## Best Practices

1. **Treat SOUL as firmware, not software**: The SOUL document is the most stable layer. Change it deliberately via reviewed patch proposals, not inline during conversations. (Source: Pub codebase, PEPA paper)

2. **Journal every meaningful interaction**: Session journaling (even just a 2-3 sentence summary) creates the raw material for reflection, user modeling, and self-improvement. Without it, the agent has no long-term memory of what worked. (Source: MUSE, Reflexion, Pub codebase)

3. **Distinguish memory tiers by volatility**: Working < Episodic < Semantic < Procedural. Faster-changing memories should never overwrite slower-changing ones without explicit promotion logic. (Source: CoALA, LangGraph docs)

4. **Score retrieval, don't just embed**: Pure vector similarity retrieval misses important recent memories. Combine recency, importance, and relevance scores (Generative Agents formula). (Source: arXiv:2304.03442)

5. **Build the ACI before the agent**: Design your tool interfaces with the agent as the end-user, not a human. Clear affordances, explicit error messages, persistent state. (Source: SWE-agent ACI paper)

6. **Proactivity needs a cost/risk gate**: Always-on agents must not exhaust resources on low-value actions. Score each opportunity; auto-execute only low-cost, high-confidence actions; gate expensive or risky ones. (Source: PEPA, ProPerSim, Pub codebase)

7. **Personality is behavioral, not textual**: "I value honesty" in a SOUL document has no effect unless the reflection engine checks behavioral evidence against it. Tie character traits to observable behavioral patterns. (Source: Anthropic Claude character research, PEPA)

8. **Contradiction detection is non-negotiable**: Without it, the user model becomes an inconsistent mess of outdated beliefs. Run LLM-judge contradiction checks on every memory write. (Source: Cairn memory pipeline, Memoria framework)

9. **Use verbal reinforcement before fine-tuning**: Reflexion-style self-improvement is faster, cheaper, and safer than fine-tuning. Reach for verbal reinforcement first when the agent makes repeated mistakes. (Source: Reflexion paper)

10. **Separate skill library from conversation history**: Skills should be stored as verifiable, composable code programs indexed by semantic description (Voyager pattern), not as patterns in conversation history that can be overwritten or forgotten. (Source: Voyager, MetaAgent)

---

## Commercial Product Comparison

| Product | Memory | Personality | Proactivity | Always-On |
|---------|--------|-------------|-------------|-----------|
| Google Project Astra | Multimodal, cross-device | Tuned for warm/capable assistant | High: starts conversations | Yes: ambient awareness |
| Apple Intelligence | On-device + cloud | Apple brand voice, privacy-first | Medium: contextual suggestions | OS-integrated |
| Microsoft Copilot | Enterprise data grounding | Professional, productivity-focused | Medium: workflow triggers | Background in Office |
| Rabbit R1 / rabbitOS | LAM action memory | Minimal personality layer | Medium: LAM shortcuts | Cloud-based |
| Humane AI Pin | Minimal persistent | Neutral voice | Low: primarily reactive | Always listening |
| MemGPT / Letta | Full 4-layer, self-editing | Per-agent persona, evolving | Manual + background subagents | Server deployment |
| Generative Agents | Memory stream, reflection | Big Five personality prototype | Yes: autonomous goal-setting | Simulated environment |
| AutoGPT | Block-based, checkpoints | Minimal | Medium: triggered workflows | Cloud deployment |

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/abs/2310.08560) | Research Paper | The canonical OS analogy for LLM agents; memory hierarchy design |
| [Generative Agents (Park et al. 2023)](https://arxiv.org/abs/2304.03442) | Research Paper | Best implementation of memory stream + reflection + personality |
| [PEPA: Persistently Autonomous Embodied Agent with Personalities](https://arxiv.org/abs/2603.00117) | Research Paper | Sys1/2/3 cognitive architecture; Big Five personality as intrinsic reward |
| [CoALA: Cognitive Architectures for Language Agents](https://arxiv.org/abs/2309.02427) | Research Paper | Canonical four-layer memory taxonomy; action space framework |
| [Reflexion (Shinn et al. 2023)](https://arxiv.org/abs/2303.11366) | Research Paper | Verbal reinforcement learning; self-reflection mechanism |
| [Voyager (Wang et al. 2023)](https://arxiv.org/abs/2305.16291) | Research Paper | Open-ended lifelong learning; skill library as code |
| [SWE-agent: Agent-Computer Interface Design](https://arxiv.org/abs/2405.15793) | Research Paper | ACI design principles for coding agents |
| [The Oscars of AI Theater (Role-Playing Survey)](https://arxiv.org/abs/2407.11484) | Research Paper | Comprehensive survey of persona consistency techniques |
| [Memory for Autonomous LLM Agents (Survey)](https://arxiv.org/abs/2603.07670) | Research Paper | Five memory mechanism families; write-manage-read loop |
| [IronEngine: General AI Assistant](https://arxiv.org/abs/2603.08425) | Research Paper | Production-grade unified orchestration platform |
| [ProPerSim: Proactive + Personalized AI](https://arxiv.org/abs/2509.21730) | Research Paper | Proactivity + personalization combined; user preference learning |
| [Memoria: Scalable Agentic Memory](https://arxiv.org/abs/2512.12686) | Research Paper | Knowledge graph user modeling; session-to-long-term consolidation |
| [MUSE: Experience-Driven Self-Evolving Agent](https://arxiv.org/abs/2510.08002) | Research Paper | Hierarchical experience memory for long-horizon tasks |
| [VIGIL: Reflective Runtime for Self-Healing Agents](https://arxiv.org/abs/2512.07094) | Research Paper | Behavioral drift detection; identity preservation via guarded updates |
| [MARS: Meta-cognitive Reflection Framework](https://arxiv.org/abs/2601.11974) | Research Paper | Principle-based vs procedural reflection; human-like self-improvement |
| [Letta (formerly MemGPT)](https://www.letta.com/) | Product Docs | Production agent memory system; background subagents |
| [LangGraph Memory Concepts](https://docs.langchain.com/oss/python/langgraph/memory) | Framework Docs | Practical three-type memory system; hot-path vs background writes |
| [Anthropic: Building Effective Agents](https://www.anthropic.com/research/building-effective-agents) | Blog Post | Workflow vs agent distinction; tool documentation principles |
| [Anthropic: Claude's Character](https://www.anthropic.com/news/claude-character) | Blog Post | Character training methodology; Constitutional AI for personality |
| [ReAct: Synergizing Reasoning and Acting](https://arxiv.org/abs/2210.03629) | Research Paper | Thought-Action-Observation loop; the foundation of modern agent reasoning |
| [Active Context Compression](https://arxiv.org/abs/2601.07190) | Research Paper | Autonomous memory pruning; 22.7% token savings without accuracy loss |
| [EvoAgent: Self-Evolving with Continual World Model](https://arxiv.org/abs/2502.05907) | Research Paper | Self-planning, self-control, self-reflection in one loop |
| [MetaAgent: Tool Meta-Learning](https://arxiv.org/abs/2508.00271) | Research Paper | Data-driven continual tool learning without model retraining |
| [COCO: Cognitive OS with Continuous Oversight](https://arxiv.org/abs/2508.13815) | Research Paper | Bidirectional reflection; rollback mechanism for agent state recovery |
| [Project Astra (Google DeepMind)](https://deepmind.google/technologies/gemini/project-astra/) | Product Page | Commercial always-on multimodal agent architecture |

---

## Relevancy Analysis for Pub / Cairn

This topic directly maps to both active projects:

### Pub (Personal Signal-to-Action OS)
- **SOUL.md + ReflectionEngine**: Already implemented. The PEPA and MARS papers validate the approach and offer improvements: include Big Five-style personality dimensions in SOUL, add a consistency check to the reflection loop.
- **Session journaling**: Implemented (migration 034). Consider adding importance scores (0-10) to journal entries to enable Generative Agents-style weighted retrieval.
- **Idle mode proactivity**: Implemented. Add explicit opportunity scoring (impact × confidence / cost) before acting.
- **Memory contradiction detection**: **Gap**. Currently no LLM-judge step on memory writes. High priority to add.
- **User preference model as knowledge graph**: **Gap**. Currently flat key-value semantic memories. A weighted KG (Memoria pattern) would enable richer personalization.

### Cairn (Personal Agent OS, Go)
- **25 accepted memories, semantic search**: Working. Next step: add the contradiction detection pipeline (already planned per memory notes).
- **Personality**: Currently no explicit SOUL layer. Consider adding a `soul.md` file loaded at agent startup, with a ReflectionEngine that proposes patches.
- **Idle mode**: Not yet implemented. The PEPA Sys3 layer + Voyager curriculum are good blueprints for the next phase.
- **Skill library as code**: Implemented as SKILL.md files. Consider also storing successful tool-call sequences as reusable skills (Voyager pattern).

---

*This guide was synthesized from 42 sources. See `resources/agent-os-personality-sources.json` for full source list with quality scores.*
