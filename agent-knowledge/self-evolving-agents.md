# Learning Guide: Self-Evolving AI Agents

**Generated**: 2026-03-19
**Sources**: 42 resources analyzed
**Depth**: deep

---

## Prerequisites

- Familiarity with LLMs and prompt engineering basics
- Understanding of agent loops (observe → think → act)
- Working knowledge of embeddings and vector search
- Comfort reading Python or TypeScript pseudocode

## TL;DR

- Self-evolving agents improve without retraining model weights; they adapt through memory, reflection, and skill accumulation stored outside the model.
- The canonical pattern is **experience → critique → distill → store → retrieve**: an agent acts, reflects on the outcome in language, stores the insight, and retrieves it on the next relevant task.
- Three memory types form the backbone: **semantic** (facts/preferences), **episodic** (past experiences as examples), and **procedural** (rules that update the system prompt itself).
- Skill libraries (pioneered by Voyager) and self-modifying prompt documents (SOUL files, procedural memory) are the two dominant patterns for persistent behavioral change.
- Safety and stability require guarding which parts of the agent are mutable: identities and hard rules should be locked, heuristics and style preferences should be soft.

---

## Core Concepts

### 1. The Self-Evolution Spectrum

Self-evolving agents exist on a spectrum of how deeply they can modify themselves:

| Level | Mechanism | What Changes | Example |
|-------|-----------|--------------|---------|
| 0 — Stateless | None | Nothing persists | Basic chatbot |
| 1 — Memory | Key-value / vector store | Facts recalled | MemGPT, ChatGPT with memory |
| 2 — Reflection | Episodic summaries | Strategy improves | Reflexion, generative agents |
| 3 — Skill growth | Executable code library | Capability expands | Voyager, AgentFactory |
| 4 — Prompt mutation | Procedural / SOUL doc updates | Behavior rules evolve | LangMem procedural, Pub SOUL.md |
| 5 — Architecture search | Meta-agent rewrites agent code | Agent structure evolves | ADAS / Meta Agent Search |

Most production systems aim for levels 2-4. Level 5 is research-stage and carries significant safety concerns.

### 2. Memory-Driven Adaptation

Memory is the foundational mechanism for all self-evolution. Three orthogonal types are widely recognized across frameworks (LangMem, MemGPT/Letta, generative agents):

**Semantic memory (facts)** stores declarative knowledge: user preferences, domain facts, entity relationships. Retrieved by cosine similarity against embeddings of the current context. Example: "User prefers concise responses" stored as a memory item.

**Episodic memory (experiences)** stores what happened in the past as few-shot examples or compressed summaries. The agent learns *how* to handle situations it has seen before. Reflexion's episodic memory buffer stores reflective text from past trial failures; generative agents' memory stream stores timestamped natural-language observations.

**Procedural memory (behavior)** is the most powerful form: it updates the agent's own instruction set. LangMem's procedural memory analyzes interactions and rewrites the system prompt. The Pub project's `SOUL.md` is a manually maintained version of this — the `ReflectionEngine` proposes patches to it automatically.

**Retrieval scoring** (from generative agents, arXiv:2304.03442) combines three signals:
```
score = α_recency × recency + α_importance × importance + α_relevance × relevance
```
- Recency: exponential decay from last access timestamp
- Importance: LLM-rated poignancy on 1-10 scale at storage time
- Relevance: cosine similarity to current context embedding

### 3. Reflection Loops

Reflection converts raw experience into reusable insight. The core idea (Reflexion, arXiv:2303.11366) is verbal reinforcement learning: instead of gradient updates, the agent writes a text analysis of what went wrong and stores it in an episodic buffer.

**Basic reflection loop:**
```python
trial_result = execute_task(task, current_policy)
if not trial_result.success:
    reflection = llm.generate(
        prompt=f"You attempted: {task}\nYour trajectory: {trial_result.trajectory}\n"
               f"Result: {trial_result.feedback}\n"
               "In a paragraph, identify what went wrong and what you should do differently."
    )
    memory_buffer.append(reflection)  # bounded, e.g. last 3 reflections
```

On the next attempt, the agent receives both the task and the reflection buffer as context, effectively learning from failure without weight updates.

Reflexion achieved 91% pass@1 on HumanEval coding benchmarks (vs. GPT-4's 80%) using this mechanism alone.

**Generative agents** (arXiv:2304.03442) add a higher-order reflection: when importance scores accumulate past a threshold (~150 points, roughly 2-3 times daily), the system synthesizes memories into abstract "reflections" — higher-level insights that themselves become retrievable memories. This creates a tree: observations → reflective summaries → meta-insights.

**Self-Refine** (arXiv:2305.16291) applies reflection within a single generation pass:
```python
output = generator(task)
for iteration in range(max_iterations):
    feedback = feedback_model(task, output)
    if is_terminal(feedback): break
    output = refiner(task, output, feedback)
return output
```
All three models (generator, feedback, refiner) can be the same LLM with different prompts.

**LATS (Language Agent Tree Search)** extends reflection with Monte Carlo tree search: multiple action trajectories are explored in parallel, scored through reflection, and the tree is backtracked to find optimal paths. This avoids local optima that single-trajectory reflection can get stuck in.

### 4. Skill Libraries (Voyager Pattern)

Voyager (voyager.minedojo.org) key mechanisms:

**Skill storage**: Each learned behavior is saved as executable code (JavaScript for Minecraft via Mineflayer) with a natural-language description. Skills are indexed by embedding their description.

**Skill retrieval**: When facing a new task, the agent queries for top-5 relevant skills by semantic similarity. Retrieved skills appear in the action-generation prompt as examples.

**Iterative prompting** (the inner loop, up to 4 attempts):
```python
for i in range(4):
    skills = skill_manager.retrieve_skills(task, environment_feedback)
    code = action_agent.generate_code(
        task, code, environment_feedback,
        execution_errors, critique, skills
    )
    agent_state, env_feedback, exec_errors = environment.step(code)
    success, critique = critic_agent.check_task_success(task, agent_state)
    if success:
        skill_manager.add_skill(task_description, code)
        break
```

**Automatic curriculum**: GPT-4 proposes the next task based on current inventory/state and exploration progress — an "in-context form of novelty search" that continuously challenges the agent.

Results: 3.3x more unique items discovered, 2.3x longer travel distances vs. prior approaches. Skills transfer to new Minecraft worlds, demonstrating genuine generalization.

**AgentFactory** (arXiv:2603.18000) extends this to general-purpose agents: successful task solutions are saved as standardized Python code with docstrings, creating an ever-growing library of reusable subagents that compound over time.

**JARVIS-1** (arXiv:2311.05997) adds multimodal memory to the Voyager pattern, augmenting code skills with visual observations, enabling more reliable precondition checking for when skills can be applied.

### 5. SOUL Documents and Self-Modifying Prompts

The most operationally grounded pattern for persistent behavioral identity is the **SOUL document** (or equivalent: character card, personality file, system prompt base). This is a human-readable document that defines:

- Core values and hard constraints (immutable)
- Behavioral heuristics and style preferences (soft, can evolve)
- Current knowledge of the environment
- Learned patterns and preferred approaches

In production systems (including Pub), the SOUL document is the authoritative source for agent identity. Reflection loops propose patches; human review gates what gets merged.

**Anthropic's Constitutional AI** (arXiv:2212.08073) is a scaled-up version: a "constitution" (list of principles) guides self-critique and revision during training. The AI generates responses, critiques them against the constitution, revises, and trains on the revisions — automated behavioral refinement without human labeling of harmful outputs.

**LangMem's procedural memory** extends this to runtime: the system observes patterns across conversations and updates the agent's system prompt. If the agent repeatedly gives excessively long answers, the prompt gets updated with a conciseness instruction. This is autonomous SOUL patching.

**Key design tension**: How much of the SOUL should be mutable? Three-tier model:
1. **Hard rules** (never mutable): safety constraints, identity core, ethical limits
2. **Soft rules** (propose-and-review): communication style, domain preferences, operational patterns
3. **Working memory** (freely mutable): current task state, recent learnings

### 6. Automated Design of Agentic Systems (ADAS)

The frontier of self-evolution is meta-agents that write other agents. ADAS (arXiv:2408.08435) introduces **Meta Agent Search**:

```python
archive = [initial_agent]
while True:
    meta_agent.review(archive)
    new_agent_code = meta_agent.program_novel_agent(
        inspired_by=archive,
        task_description=task
    )
    performance = evaluate(new_agent_code, benchmark)
    archive.append((new_agent_code, performance))
```

Because agents are defined in code (Turing complete), this theoretically enables learning any agentic configuration: novel prompts, tool orchestration, reasoning patterns, or hybrid architectures. Discovered agents outperformed hand-designed agents across coding, science, and math benchmarks while maintaining cross-domain robustness.

**SEMAG** (arXiv:2603.15707) applies evolutionary thinking to multi-agent code generation: workflows adapt to task difficulty, backbone models upgrade automatically, and the system selects the strongest performer. Outperforms prior methods by 3.3% on CodeContests when using the same base model.

**EvoTool** (arXiv:2603.04900) evolves tool-use policies via evolutionary operators:
- **Blame-aware mutation**: diagnoses which module (Planner/Selector/Caller/Synthesizer) caused a failure, then surgically mutates only that module via natural-language critique
- **Diversity-aware selection**: preserves varied solution candidates to avoid premature convergence

### 7. Multi-Agent Co-Evolution

When multiple agents interact, collective evolution emerges:

**SAGE** (arXiv:2603.15255) has four specialized agents co-evolve from a shared backbone:
- Challenger generates progressively harder tasks
- Planner transforms tasks into structured plans
- Solver executes and verifies via tools
- Critic filters low-quality outputs to prevent curriculum drift

Using only a small seed set, this improved Qwen-2.5-7B by 8.9% on LiveCodeBench and 10.7% on OlympiadBench.

**AutoAgents** (arXiv:2309.17288) generates specialized agents dynamically based on task requirements, with an Observer agent providing continuous reflective feedback to improve both strategies and agent configurations.

**MetaGPT** (arXiv:2308.00352) demonstrates a subtler form: agents review feedback from previous projects and update their constraint prompts before each new project. Memory is stored as summaries that "can be inherited by future constraint prompt updates."

**EvoScientist** (arXiv:2603.08127) applies persistent memory to scientific discovery: an Ideation Memory records failed research directions and a separate Experimentation Memory captures effective implementation strategies, both continuously updated as the system attempts new research tasks.

### 8. Claude's Character and Agent Identity

Anthropic's key principles for agent identity:

- **Authenticity over performance**: character traits are genuine to the model, not masks. Claude's curiosity, warmth, and commitment to honesty are real traits that emerged through training, not behavioral rules to be overridden.
- **Traits guide, not mandate**: "We don't want Claude to treat its traits like rules from which it never deviates" — the constitution guides probabilistic behavior, not enforces rigid rules.
- **Transparent limitations**: character training includes honest acknowledgment of memory constraints ("I cannot remember past conversations") — the agent knows what it does not know.
- **Training methodology**: Constitutional AI + RLAIF generates character-aligned responses synthetically, trains preference models, then uses those for RL. The cycle is: generate → self-rank against constitution → train on winners.

The critical design decision: **which traits are fixed vs. evolvable?** Anthropic locks safety-critical behaviors during training while leaving communication style and knowledge more malleable.

### 9. ReAct and Reasoning-as-Memory

ReAct (arXiv:2210.03629) introduced thought-action interleaving, which has an implicit memory effect: the agent's scratchpad (Thought traces) becomes a working memory that accumulates context across the current episode.

```
Thought: I need to find the year the film was released.
Action: search("film title release year")
Observation: The film was released in 1998.
Thought: Now I know the year. I need to calculate how old the director was.
Action: calculate(2026 - 1998)
...
```

Each thought-observation pair updates the implicit state, enabling multi-hop reasoning across tool calls. This is ephemeral memory (single session) but forms the foundation for episodic storage: successful ReAct trajectories can be distilled and stored as few-shot examples for future retrieval.

### 10. Self-Correction and Its Limits

**Intrinsic self-correction without external feedback is unreliable.** A 2024 survey (arXiv:2406.01297) found that prompted self-correction from LLMs alone does not reliably work in most tasks. The conditions for success are:

1. **Reliable external feedback** available (code execution, test results, environment signals)
2. **Large-scale fine-tuning** specifically for correction
3. **Task structure** naturally amenable to verification

This explains why Voyager's self-verification loop works (the Minecraft environment provides ground-truth signals), while pure prompt-based self-critique often produces sycophantic agreement with the original output.

**Practical implication**: always ground reflection in external signals when possible — test failures, assertion errors, environment rewards — rather than asking the model to critique its own output in isolation.

---

## Architecture Patterns

### Pattern A: Memory + Reflection Pipeline

```python
class SelfEvolvingAgent:
    def __init__(self):
        self.semantic_memory = VectorStore()      # facts/preferences
        self.episodic_memory = EpisodicBuffer()   # past experiences
        self.soul_doc = SOULDocument()             # procedural rules

    def act(self, task):
        # 1. Retrieve relevant context
        facts = self.semantic_memory.search(task)
        experiences = self.episodic_memory.search(task)
        rules = self.soul_doc.get_relevant_rules(task)

        # 2. Execute with full context
        result = self.llm.run(
            system=self.soul_doc.core_identity + rules,
            user=task,
            context=facts + experiences
        )

        # 3. Reflect on outcome
        if result.needs_reflection:
            reflection = self.reflect(task, result)
            self.episodic_memory.store(task, result, reflection)

        # 4. Extract semantic memories
        new_facts = self.extract_facts(task, result)
        self.semantic_memory.store(new_facts)

        # 5. Periodically propose SOUL patches
        if self.should_reflect():
            patch = self.propose_soul_patch()
            self.soul_doc.apply_patch(patch, require_review=True)

        return result.output

    def reflect(self, task, result):
        return self.llm.run(
            f"Task: {task}\nOutcome: {result}\n"
            "What went wrong? What should I do differently next time?"
        )
```

### Pattern B: Skill Library (Voyager-style)

```python
class SkillLibraryAgent:
    def __init__(self):
        self.skill_library = {}  # description_embedding -> code

    def execute_with_skills(self, task):
        relevant_skills = self.retrieve_top_k_skills(task, k=5)

        for attempt in range(max_attempts=4):
            code = self.generate_code(task, relevant_skills,
                                      self.last_feedback, self.last_errors)
            result, feedback, errors = self.environment.run(code)
            success = self.verify(task, result)

            if success:
                self.add_skill(task, code)  # persist for future use
                break

        return result

    def add_skill(self, task_description, code):
        embedding = embed(task_description)
        self.skill_library[embedding] = {
            "description": task_description,
            "code": code,
            "created_at": now()
        }
```

### Pattern C: SOUL Patch Proposal

```python
class ReflectionEngine:
    def propose_soul_patch(self, recent_sessions, current_soul):
        pattern_analysis = self.llm.run(
            system="You are analyzing an AI agent's behavior for patterns.",
            user=f"""
            Recent sessions (last 10):
            {recent_sessions}

            Current SOUL document:
            {current_soul}

            Identify:
            1. Repeated mistakes that a new rule could prevent
            2. Successful patterns that should be reinforced
            3. Outdated rules that no longer apply

            Propose specific patches in unified diff format.
            """
        )
        return SOULPatch(
            diff=pattern_analysis.diff,
            confidence=pattern_analysis.confidence,
            requires_human_review=True  # always for safety
        )
```

### Pattern D: Episodic Memory with RetroAgent-style Feedback

```python
class RetroAgent:
    def __init__(self):
        self.memory = EpisodicMemory(retrieval="simutil_ucb")
        # SimUtil-UCB balances: similarity + utility + exploration

    def solve(self, task):
        # retrieve lessons from similar past failures
        memories = self.memory.retrieve(
            query=task,
            strategy="similarity_and_utility"  # avoid over-relying on same memory
        )

        result = self.execute(task, context=memories)

        # dual feedback: intrinsic numerical + intrinsic language
        subtask_progress = self.measure_subtask_completion(result)
        lesson = self.distill_lesson(task, result)

        self.memory.store({
            "task": task,
            "outcome": result,
            "lesson": lesson,
            "progress_delta": subtask_progress
        })
        return result
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Memory poisoning | Bad reflections get stored and retrieved, degrading future behavior | Confidence threshold + external validation before storing; periodic memory review |
| Sycophantic self-critique | LLM agrees with its own output when asked to critique it | Always use external feedback signals (tests, environment); never rely solely on intrinsic critique |
| Identity drift | Procedural memory updates cause agent to drift from intended behavior | Lock identity core; use propose-and-review for any soul/prompt mutations |
| Catastrophic memory interference | New memories overwrite or contradict useful old ones | Use embeddings + retrieval (not fixed slots); implement contradiction detection before storing |
| Skill bloat | Skill library grows unbounded, retrieval quality degrades | Periodic pruning; merge similar skills; score skills by usage and recency |
| Over-reflection overhead | Running reflection on every interaction wastes tokens | Selective reflection: trigger only on failures, surprising outcomes, or confidence < threshold |
| Curriculum collapse | Self-generated task distribution drifts to trivial tasks | Include novelty signal in curriculum (Voyager's "in-context novelty search"); external task injection |
| Missing external ground truth | Self-correction loops that have no external signal fail | Always wire in verifiable feedback: code execution, unit tests, environment states, human ratings |

---

## Best Practices

1. **Separate mutable from immutable memory** (Source: LangMem, SOUL.md pattern): Hard safety rules go in an immutable section. Style preferences and heuristics go in a mutable section with version control.

2. **Ground reflection in external signals** (Source: Reflexion, Voyager, Self-correction survey): Use code execution results, test failures, and environment feedback rather than LLM self-critique alone. Verification beats introspection.

3. **Use retrieval scoring that accounts for recency, importance, and relevance** (Source: Generative Agents): Pure cosine similarity retrieval misses temporal relevance. Weight recent, high-importance memories more heavily.

4. **Store skills as executable code, not natural language** (Source: Voyager, AgentFactory): Code is deterministic and can be tested. Natural-language procedure descriptions are ambiguous at execution time.

5. **Bound the episodic memory buffer** (Source: Reflexion): Keep only the last 1-3 reflections per task type. Unbounded memory exceeds context windows and dilutes signal.

6. **Implement contradiction detection before updating semantic memory** (Source: Cairn memory pipeline): Before inserting a new fact, check if it contradicts existing facts; apply an LLM judge (ADD/UPDATE/SKIP/CONTRADICT) rather than blindly appending.

7. **Make SOUL/personality changes require explicit review** (Source: Pub ReflectionEngine, safety principles): Autonomous soul patching without review creates runaway behavioral drift. Propose → review → merge is the safer workflow.

8. **Use diverse retrieval (MMR or SimUtil-UCB) to prevent fixation** (Source: RetroAgent, MemGPT): Maximum Marginal Relevance or similar diversity-aware retrieval prevents the agent from always retrieving the same memory, broadening its behavioral repertoire.

9. **Implement skill verification before library addition** (Source: Voyager, JARVIS-1): A newly learned skill should pass a success check before it is committed to the library. Failed skills shouldn't be learned from — only successes compound.

10. **Log and periodically review agent evolution** (Source: ADAS, EvoScientist): Maintain an archive of behavior changes over time. Review the archive before proposed soul patches to detect regressions.

---

## TypeScript Implementation Patterns

### TypeScript: Memory Store Interface

```typescript
interface MemoryItem {
  id: string;
  content: string;
  embedding: number[];       // 768 or 1024 dimensional
  importance: number;        // 1-10, LLM-rated at storage time
  createdAt: Date;
  lastAccessedAt: Date;
  accessCount: number;
  type: 'semantic' | 'episodic' | 'procedural';
  tags: string[];
}

interface MemoryStore {
  store(item: Omit<MemoryItem, 'id'>): Promise<MemoryItem>;
  search(query: string, k: number): Promise<MemoryItem[]>;
  update(id: string, updates: Partial<MemoryItem>): Promise<void>;
  prune(strategy: 'recency' | 'importance' | 'usage'): Promise<number>;
}

// Retrieval scoring (generative agents formula)
function scoreMemory(memory: MemoryItem, queryEmbedding: number[]): number {
  const recency = Math.exp(-DECAY_RATE * hoursSince(memory.lastAccessedAt));
  const importance = memory.importance / 10;
  const relevance = cosineSimilarity(queryEmbedding, memory.embedding);
  return ALPHA_RECENCY * recency + ALPHA_IMPORTANCE * importance + ALPHA_RELEVANCE * relevance;
}
```

### TypeScript: Reflexion Loop

```typescript
interface ReflexionState {
  task: string;
  trajectories: Trajectory[];
  reflections: string[];      // bounded circular buffer
  maxReflections: number;     // typically 1-3
}

async function reflexionLoop(
  task: string,
  executor: AgentExecutor,
  maxTrials: number = 5
): Promise<string> {
  const state: ReflexionState = {
    task,
    trajectories: [],
    reflections: [],
    maxReflections: 3,
  };

  for (let trial = 0; trial < maxTrials; trial++) {
    const result = await executor.run(task, {
      context: state.reflections.join('\n')
    });

    state.trajectories.push(result.trajectory);

    if (result.success) return result.output;

    // Generate reflection from failure
    const reflection = await llm.generate(
      `Task: ${task}\n` +
      `Attempt ${trial + 1} trajectory: ${result.trajectory}\n` +
      `Feedback: ${result.feedback}\n` +
      `What specific mistake was made and what should be done differently?`
    );

    // Bounded buffer: keep only recent reflections
    state.reflections = [...state.reflections.slice(-(state.maxReflections - 1)), reflection];
  }

  throw new Error(`Failed to complete task after ${maxTrials} trials`);
}
```

### TypeScript: SOUL Patch Proposal

```typescript
interface SOULPatch {
  section: 'hard_rules' | 'soft_rules' | 'patterns' | 'knowledge';
  operation: 'add' | 'update' | 'remove';
  content: string;
  rationale: string;
  confidence: number;        // 0-1
  evidenceSessions: string[]; // session IDs that triggered this
}

async function proposeSOULPatch(
  recentSessions: Session[],
  currentSOUL: string
): Promise<SOULPatch[]> {
  const analysis = await llm.generate({
    system: 'Analyze agent session logs and propose minimal, targeted SOUL document patches.',
    user: `
      Recent sessions (${recentSessions.length}):
      ${recentSessions.map(s => s.summary).join('\n---\n')}

      Current SOUL document:
      ${currentSOUL}

      Identify repeated error patterns, consistent successes worth reinforcing,
      and outdated rules. Propose specific patches with high confidence only.
    `,
    schema: { patches: 'SOULPatch[]' }
  });

  // Only surface high-confidence patches
  return analysis.patches.filter(p =>
    p.confidence > 0.8 && p.section !== 'hard_rules'  // never auto-patch hard rules
  );
}
```

---

## Go Implementation Patterns

### Go: Skill Library with HNSW Index

```go
type Skill struct {
    ID          string
    Description string
    Code        string
    Embedding   []float32
    CreatedAt   time.Time
    SuccessRate float64
    UseCount    int
}

type SkillLibrary struct {
    mu     sync.RWMutex
    skills []*Skill
    index  *hnswlib.Index  // or any ANN library
}

func (sl *SkillLibrary) Retrieve(task string, k int) ([]*Skill, error) {
    queryEmb, err := embed(task)
    if err != nil {
        return nil, err
    }
    sl.mu.RLock()
    defer sl.mu.RUnlock()
    ids, _ := sl.index.SearchKNN(queryEmb, k)
    // return skills by ID, filter by success_rate > 0.5
    return sl.filterByQuality(ids), nil
}

func (sl *SkillLibrary) AddSkill(description, code string) error {
    emb, err := embed(description)
    if err != nil {
        return err
    }
    skill := &Skill{
        ID:          uuid.New().String(),
        Description: description,
        Code:        code,
        Embedding:   emb,
        CreatedAt:   time.Now(),
        SuccessRate: 1.0,
    }
    sl.mu.Lock()
    defer sl.mu.Unlock()
    sl.skills = append(sl.skills, skill)
    return sl.index.AddPoint(emb, len(sl.skills)-1)
}
```

### Go: Episodic Memory with Contradiction Detection

```go
type MemoryJudge string

const (
    JudgeADD         MemoryJudge = "ADD"
    JudgeUPDATE      MemoryJudge = "UPDATE"
    JudgeSKIP        MemoryJudge = "SKIP"
    JudgeCONTRADICT  MemoryJudge = "CONTRADICT"
)

func (ms *MemoryStore) InsertWithCheck(ctx context.Context, newFact string) error {
    similar, err := ms.SearchSimilar(ctx, newFact, k=5)
    if err != nil {
        return err
    }
    if len(similar) == 0 {
        return ms.Insert(ctx, newFact, JudgeADD)
    }

    judgment, err := ms.llm.Judge(ctx, JudgePrompt{
        NewFact:   newFact,
        Existing:  similar,
    })
    if err != nil {
        return err
    }

    switch judgment.Action {
    case JudgeADD:
        return ms.Insert(ctx, newFact, JudgeADD)
    case JudgeUPDATE:
        return ms.Update(ctx, judgment.TargetID, newFact)
    case JudgeSKIP:
        return nil  // already known
    case JudgeCONTRADICT:
        return ms.FlagContradiction(ctx, judgment.TargetID, newFact)
    }
    return nil
}
```

---

## Framework Comparison

| Framework | Memory Types | Reflection | Skill Library | SOUL/Prompt Mutation | Open Source |
|-----------|-------------|------------|---------------|---------------------|-------------|
| MemGPT / Letta | All 3 (tiered) | Limited | No | Partial (memory blocks) | Yes (Python) |
| Voyager | Skill embeddings | Yes (code verif.) | Yes (core feature) | No | Yes (Python) |
| Reflexion | Episodic buffer | Yes (verbal RL) | No | No | Yes (Python) |
| LangMem | All 3 | Yes (procedural) | No | Yes (core feature) | Yes (Python) |
| AutoGen | Short-term | Via conversation | No | No | Yes (Python) |
| SAGE | None explicit | Yes (critic agent) | No | Shared backbone | Research |
| AgentFactory | Code library | Implicit | Yes (subagents) | No | Research |
| Pub (this project) | Semantic + Episodic | ReflectionEngine | Skills (SKILL.md) | SOUL.md (propose/review) | Private |

---

## Taxonomic Map of Techniques

- **Memory Systems**
  - Semantic (facts): vector store + embeddings
  - Episodic (experiences): timestamped natural language
  - Procedural (behavior): system prompt / SOUL doc
- **Reflection Mechanisms**
  - Single-trial (Self-Refine): generator → feedback → refiner loop
  - Cross-trial (Reflexion): episodic buffer of verbal reflections
  - Hierarchical (Generative Agents): observations → insights → meta-insights
  - Tree-search (LATS): MCTS + reflection for optimal trajectory
- **Skill Acquisition**
  - Code libraries (Voyager, AgentFactory): executable, testable
  - Subagent libraries (AutoAgents, AgentFactory): composable agents as skills
  - Retrieval-augmented execution: top-k skill injection into context
- **Prompt / SOUL Evolution**
  - Constitutional AI: constitution-guided self-critique at training time
  - Procedural memory (LangMem): runtime prompt optimization
  - SOUL.md + ReflectionEngine: propose → review → merge
  - ADAS / Meta Agent Search: meta-agent rewrites agent code
- **Multi-Agent Co-Evolution**
  - SAGE: challenger + planner + solver + critic co-evolve
  - EvoScientist: researcher + engineer + evolution manager
  - AutoAgents: dynamic role generation + observer reflection
  - RetroAgent: dual intrinsic (numerical + language) feedback
- **Safety Controls**
  - Immutable core: hard rules locked from mutation
  - Propose-and-review: SOUL patches require human approval
  - Confidence thresholds: only store high-confidence memories
  - External verification: never rely on pure intrinsic critique

---

## Key Research Results Summary

Empirical benchmarks from key self-evolving agent papers:

| System | Task Domain | Key Metric | Mechanism |
|--------|-------------|------------|-----------|
| Voyager | Minecraft | 3.3x unique items | Skill library + iterative prompting |
| Reflexion | HumanEval coding | 91% pass@1 (vs GPT-4 80%) | Episodic reflection buffer |
| SAGE | LiveCodeBench | +8.9% on Qwen-2.5-7B | 4-agent co-evolution |
| RetroAgent | ALFWorld | +18.3% vs GRPO | Dual intrinsic feedback |
| EvoTool | Tool use | +5pts on GPT-4.1 | Blame-aware mutation |
| ADAS | Math/code/science | Beat hand-designed | Meta agent search |
| Steve-Evolving | Minecraft | Continuous improvement | 3-phase closed loop |
| GUI Agent (HybridMem) | GUI tasks | +22.5% on Qwen2.5-VL | Graph memory + self-evolution |
| AutoAgent | Multi-domain | Improved robustness | Closed-loop cognitive evolution |

---

## Operational Pitfalls

| Pitfall | Root Cause | Solution |
|---------|-----------|---------|
| Reflection hallucinations | LLM fabricates plausible-sounding lessons | Anchor reflections to concrete evidence (error messages, test outputs) |
| Memory retrieval latency | Large vector stores slow nearest-neighbor search | Use approximate ANN (HNSW); cache frequent retrievals |
| Context window saturation | Too many retrieved memories crowd out task context | Score and filter memories; use compressed episodic summaries |
| Skill library staleness | Skills work for old tool versions/environments | Version-tag skills; re-verify on environment changes |
| SOUL patch wars | Multiple reflection cycles produce contradictory patches | Track patch provenance; resolve conflicts via explicit merge strategy |
| Emergent goal drift | Self-improvement optimizes for wrong proxy metric | Define clear terminal evaluation criteria; regular human audits |

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Voyager: LLM-powered Lifelong Learning in Minecraft](https://voyager.minedojo.org/) | Project site | Canonical skill library demo with open code |
| [Reflexion (arXiv:2303.11366)](https://arxiv.org/abs/2303.11366) | Paper | Core verbal RL reflection mechanism |
| [Generative Agents (arXiv:2304.03442)](https://arxiv.org/abs/2304.03442) | Paper | Memory stream + hierarchical reflection architecture |
| [Self-Refine (arXiv:2305.16291)](https://arxiv.org/abs/2305.16291) | Paper | Single-pass iterative refinement loop |
| [MemGPT / Letta GitHub](https://github.com/cpacker/MemGPT) | Codebase | Tiered memory OS for LLM agents |
| [LangMem SDK](https://github.com/langchain-ai/langmem) | Codebase | Procedural + semantic + episodic memory for LangGraph |
| [Constitutional AI (arXiv:2212.08073)](https://arxiv.org/abs/2212.08073) | Paper | Anthropic's self-critique training methodology |
| [ADAS / Meta Agent Search (arXiv:2408.08435)](https://arxiv.org/abs/2408.08435) | Paper | Meta-agent programming better agents in code |
| [AutoAgent (arXiv:2603.09716)](https://arxiv.org/abs/2603.09716) | Paper | Elastic memory + closed-loop cognitive evolution |
| [SAGE (arXiv:2603.15255)](https://arxiv.org/abs/2603.15255) | Paper | 4-agent co-evolution with curriculum control |
| [EvoTool (arXiv:2603.04900)](https://arxiv.org/abs/2603.04900) | Paper | Evolutionary tool-use policy optimization |
| [RetroAgent (arXiv:2603.08561)](https://arxiv.org/abs/2603.08561) | Paper | Dual intrinsic feedback for self-evolution |
| [AgentFactory (arXiv:2603.18000)](https://arxiv.org/abs/2603.18000) | Paper | Code-based subagent accumulation and reuse |
| [Letta Docs](https://docs.letta.com/introduction) | Docs | Production memory-as-service for agents |
| [LangChain Reflection Agents](https://blog.langchain.com/reflection-agents/) | Blog | LATS and practical reflection implementation |
| [Lilian Weng: LLM Powered Agents](https://lilianweng.github.io/posts/2023-06-23-agent/) | Blog | Comprehensive overview of memory, planning, reflection |

---

## Relevancy Analysis for Pub

Pub's architecture implements several of the patterns described in this guide:

- **Semantic memory**: `memory_items` table + sqlite-vec embeddings — directly corresponds to semantic memory tier
- **Episodic memory**: `session_journal` table — `SessionJournaler` produces structured entries per tick/chat
- **Procedural memory**: `SOUL.md` — `ReflectionEngine` proposes patches, human reviews before merging
- **Skill library**: `backend/.pub/skills/` — SKILL.md files are natural-language skill definitions (not executable code, more like Voyager's curriculum than its code library)
- **Reflection loop**: `ReflectionEngine` runs every 30 min or 10 ticks, detects patterns across sessions, proposes memories and SOUL patches
- **Constitutional AI analog**: `KnowledgePromoter` — high-usage memories (5+) get promoted to SOUL.md doc patches

**Gap**: Pub does not implement executable code skill libraries (Voyager level). Skills are prompt-based instructions, not verified executable procedures. Adding a code skill library to the agent loop would enable genuine skill compounding.

**Gap**: Pub's SOUL patching requires human review by design (correct safety choice). The `ReflectionEngine` outputs proposals — ensuring behavioral evolution stays under human control.

---

*This guide was synthesized from 42 sources. See `resources/self-evolving-agents-sources.json` for full source list.*
