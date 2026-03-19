# Learning Guide: Active Learning and User Adaptation in AI Agents

**Generated**: 2026-03-19
**Sources**: 42 resources analyzed
**Depth**: deep

---

## Prerequisites

- Familiarity with LLMs and conversational AI basics
- Basic understanding of embeddings and vector similarity search
- Working knowledge of RAG (Retrieval-Augmented Generation)
- Exposure to reinforcement learning concepts is helpful but not required

---

## TL;DR

- Agents learn "who you are" through three memory layers: semantic (facts), episodic (sessions), and procedural (rules/patterns) — all three are required for genuine personalization
- Implicit feedback signals (dwell time, sentiment, acceptance rate, edit distance) are more scalable than explicit ratings but noisier; combining both is the state of the art
- The cold-start problem is addressed by meta-learning (rapid adaptation from few examples), cross-user similarity transfer, and proactive elicitation ("ask instead of guess")
- Contextual bandits provide the principled exploration-exploitation framework for deciding when to try new response styles vs. exploit known preferences
- Preference drift is real — user models must timestamp beliefs and decay stale ones rather than treating user state as static

---

## Core Concepts

### 1. The Three-Layer Memory Model for Personalization

Effective user adaptation requires three distinct memory types that work together (arXiv:2603.07670, LangChain blog):

**Semantic memory** stores factual user preferences as structured entities:
- "User prefers concise answers under 200 words"
- "User is an expert in TypeScript but a beginner in Rust"
- "User dislikes passive voice and em-dashes"

These are stored as key-value facts in a vector store and retrieved by semantic similarity to the current query. Mem0 implements this pattern with LLM-based extraction — after each conversation turn, an LLM (e.g., GPT-4 Nano) extracts preference facts and deduplicates them against existing memories.

**Episodic memory** records sequences of past sessions — what happened, what worked, what failed:
- "In session 2026-03-15, user asked for a Python script, rejected the first draft citing verbosity, accepted after 2 revisions"
- "User tends to re-engage 48h after a successful coding session"

These inform few-shot examples and behavioral calibration. MemGPT (arXiv:2310.08560) pioneered OS-inspired virtual context management: information is paged between fast (in-context) and slow (disk) tiers, with the LLM controlling what to move. This enables coherent multi-session interactions where "the agent remembers, reflects, and evolves dynamically."

**Procedural memory** encodes distilled behavioral rules — patterns abstracted from many episodes:
- "Always ask for clarification before writing a >100-line function"
- "User responds better to numbered lists than prose for technical content"

Procedural updates are the highest-trust form of learning and should require pattern detection across multiple sessions before being written. In Pub's architecture this maps to SOUL.md patches proposed by ReflectionEngine.

**Key insight**: Personalization permeates the entire agent pipeline, not just output formatting. A 2026 survey on Personalized LLM-Powered Agents (arXiv:2602.22680) found that "user signals must be represented, propagated, and utilized across profile modeling, memory, planning, and action execution."

---

### 2. Active Learning Patterns for Agents

Active learning is the practice of strategically selecting which information to acquire rather than passively absorbing everything. For agents, this means:

**Query strategies for preference elicitation:**

*Uncertainty sampling* — ask when the agent is least certain. If the reward model assigns similar scores to two response styles, proactively ask: "I can answer this technically or conversationally — which do you prefer?" (arXiv:2306.08954)

*Expected model change* — ask questions that will produce the largest update to the user model. A question that confirms something already known has low value; a question about an unexplored dimension has high value.

*Diversity sampling* — ensure queries cover different dimensions of preference (formality, verbosity, technicality, format) rather than repeatedly asking about the same axis.

The CPER framework (arXiv:2503.12556) demonstrated this most directly in dialogue — "from guessing to asking." Instead of LLMs inferring unstated preferences, the system proactively solicits user information via targeted questioning. On movie recommendation this achieved +42% preference alignment improvement; on mental health support, +27% improvement. The approach was particularly effective in extended dialogues (12+ turns).

The PPP framework (arXiv:2511.02208) formalized this as multi-objective RL: Productivity (task success) + Proactivity (strategic questioning) + Personalization (preference adaptation). Agents trained with PPP achieved +21.6 average improvement over GPT-4 baselines on software engineering tasks while developing the capacity to adapt to previously unseen user preferences.

**Proactive preference discovery via information theory:**

BED-LLM (arXiv:2508.21184) uses Bayesian Experimental Design to compute the expected information gain of each potential question, selecting queries that maximally reduce uncertainty about latent user attributes. The key insight: treat preference inference as a scientific experiment — design each interaction turn to falsify hypotheses about the user.

**Key insight**: A benchmark study (arXiv:2510.17132) found that LLM preference inference success varies from 32% to 98% depending on context — "effective preference inference remains an open frontier." This variance argues for hybrid approaches: active elicitation for high-stakes preferences, passive inference for low-stakes ones.

---

### 3. User Modeling and Profile Construction

A user model is a structured representation of who the user is, what they want, and how they behave.

**Approaches to building user profiles:**

*Explicit profiling*: Ask the user directly at onboarding. Simple and high-fidelity but creates friction. Best for high-impact, stable preferences (expertise level, communication style).

*Implicit inference from behavior*: Learn from what users do, not what they say. Key signals:
- **Edit distance** on agent outputs: user heavily edited → output was wrong; minor edits → close but not perfect; no edit → accepted
- **Regeneration requests**: user asked for a different response → explicit rejection
- **Dwell time and engagement**: user spent 3 minutes reading → useful; closed immediately → not useful
- **Follow-up questions**: "Can you make that shorter?" → preference for brevity
- **Acceptance rate** (in coding): GitHub Copilot uses this as the primary implicit signal

*Decomposition into atomic units*: Rather than one monolithic user profile, break it into independent dimensions. The STEAM framework (arXiv:2601.16872) decomposes user preferences into "atomic memory units, each capturing a distinct interest dimension." This enables granular updates — changing a preference in one dimension doesn't corrupt others.

**Profile organization patterns:**

The PersonaTree (arXiv:2601.05171) uses a tree structure: trunk = stable core identity, branches = interest domains, leaves = specific preferences. A lightweight RL-trained MemListener model manages {ADD, UPDATE, DELETE, NO_OP} operations. Remarkably, the small MemListener model achieved "performance comparable to DeepSeek-R1-0528 and Gemini-3-Pro" on memory operation decisions.

PersonaAgent with GraphRAG (arXiv:2511.17467) uses knowledge graph + community detection: individual user behaviors become graph nodes, community detection finds clusters of related preferences, and both individual + collective patterns feed into personalized prompts. This achieved 11.1% improvement in news categorization, 56.1% in movie tagging.

**Knowledge graph for temporal user modeling (Zep pattern):**

Zep's temporal context graph assigns `valid_at` and `invalid_at` timestamps to each extracted fact. When a preference changes — e.g., user switches from Adidas to Nike — the system captures both the new preference AND the historical context. This enables the agent to reason about how preferences evolved, not just what they currently are.

---

### 4. Preference Inference from Interaction History

**The preference following gap:**

PrefEval (arXiv:2502.09597) found that in zero-shot settings, preference following accuracy falls below 10% after just 10 turns (~3k tokens) across most evaluated LLMs. This is the fundamental problem: raw LLMs are stateless and forget. The solution is explicit memory extraction and injection.

**RLHF Lite — learning from human feedback without full PPO:**

Full RLHF requires: pretraining → supervised fine-tuning → reward model training → PPO optimization. This is expensive. Lite alternatives:

*Direct Preference Optimization (DPO)*: Skips the explicit reward model step. Given preference pairs (preferred / rejected), DPO directly optimizes the policy to prefer the preferred output. Requires less infrastructure than PPO. PersoDPO (arXiv:2602.04493) applies DPO to persona-grounded dialogue.

*Reward factorization* (arXiv:2503.06358): Represent user-specific rewards as a linear combination of shared basis functions. With only ~10 user responses, the system infers individual preference weights. Achieved 67% win rate over GPT-4o defaults in human evaluation. This is the most practical "lite" approach.

*Low-rank reward modeling — LoRe* (arXiv:2504.14439): Decompose preference space into a low-dimensional basis. Individual users live in low-dimensional subspace. Enables few-shot generalization to unseen users. The key assumption: "most preference variation occurs along a smaller number of meaningful dimensions."

*Meta reward modeling* (arXiv:2601.18731): Use MAML-style meta-learning to "learn the process of preference adaptation" rather than fitting individual preference data. Cold-start is addressed by the Robust Personalization Objective (RPO), which emphasizes hard-to-learn users during meta-optimization. Achieves few-shot personalization — new users need minimal feedback.

**Personalized RLHF with user summarization (PLUS)** (arXiv:2507.13579):
Learns to produce text-based summaries of each user's preferences, characteristics, and past conversations. The summarizer and reward model co-train in an online loop. Results: 11-77% improvement over standard approaches, 25% better zero-shot performance on new users, 72% win rate with GPT-4o.

**MTRec — beyond observable signals** (arXiv:2509.22807):
A critical insight: implicit feedback signals are noisy proxies. "A user might click on a news article because of its attractive headline but end up feeling uncomfortable after reading." MTRec uses distributional inverse reinforcement learning to model the latent "mental reward" — what users truly prefer beyond what they click. Deployed on a short video platform, achieved 7% increase in viewing time.

---

### 5. Contextual Bandits for Agent Decisions

Contextual bandits formalize the exploration-exploitation tradeoff for personalized decisions. The agent must decide: exploit known preferences (give the response style the user usually likes) or explore new ones (try something different that might be better).

**Framework:**
- **Context** = user profile + current query + conversation history
- **Actions** = discrete choices (formal/informal, short/long, code/prose, proactive/reactive)
- **Reward** = implicit or explicit feedback signal

**LinUCB** (classic): Maintains a linear model per action, selects action with highest UCB = estimated reward + uncertainty bonus. Uncertainty bonus decays as more data is observed for that action.

**Neural contextual bandits** (arXiv:2312.14037): Replace linear model with neural network. Key insight: exploration strategy must account for the "Matthew Effect" — popular/default actions accumulate more data, biasing toward them. Hierarchical approaches help by first exploring broadly (exploration) then refining to user-specific preferences (exploitation).

**Collaborative bandits** (arXiv:2201.13395): Users are not independent. Group similar users into clusters, share bandit updates within clusters. This solves cold start — a new user immediately benefits from the cluster's learned preferences. The MACF framework (arXiv:2511.18413) extends this to multi-agent settings: user agents and item agents collaborate dynamically via an orchestrator.

**Hierarchical bandits for cold start** (arXiv:2601.14333): Start with broad system-level policies (high exploration), gradually refine to user-specific contexts (high exploitation). The contextual similarity-based policy transfer achieves faster cold-start adaptation. Deployed at production (Pinterest), showed 0.4% revenue improvement in A/B testing + 0.5% post-deployment.

**Key insight**: Bandit algorithms and LLMs are bidirectionally beneficial (arXiv:2601.12945). MABs provide principled adaptive decision-making for LLM personalization; LLMs improve arm definition and environment modeling for MABs.

---

### 6. Memory-Based Personalization: Mem0 and Zep Patterns

**Mem0 architecture:**

```python
from mem0 import MemoryClient

client = MemoryClient()

# Add a memory after conversation turn
client.memory.add(messages, user_id=user_id)

# Retrieve relevant memories before responding
relevant_memories = client.memory.search(
    query=current_message,
    user_id=user_id,
    limit=3
)

# Inject into system prompt
system_prompt = f"""You are a helpful assistant.
Previous memories about this user:
{', '.join([m['memory'] for m in relevant_memories])}
Answer based on query and memories."""
```

Key metrics (from Mem0 benchmarks):
- +26% accuracy vs OpenAI Memory
- 91% faster than full-context approaches
- 90% fewer tokens than context-full systems

The efficiency gains come from extracting and indexing facts rather than replaying full conversation history. A user preference stored as "dislikes verbose code comments" occupies ~10 tokens; the conversation where that preference emerged might occupy 2,000 tokens.

**Zep's temporal knowledge graph:**

```python
from zep_cloud.client import Zep

client = Zep(api_key=API_KEY)

# Add messages to session
client.memory.add(session_id, messages=messages)

# Get assembled context (already formatted for LLM)
memory = client.memory.get(session_id)
# Returns: relevant_facts, entity_summary, dialogue_summary
```

Zep's distinguishing feature: temporal reasoning. Each extracted relationship is timestamped. When user preferences change, both the old and new values are preserved with validity windows. This enables the agent to say "you used to prefer X but switched to Y in March" — conversational continuity that feels genuinely attentive.

**Memoria's hybrid architecture** (arXiv:2512.12686):
Combines session-level summarization (for recency) with a weighted knowledge graph (for persistent patterns). The knowledge graph captures "user traits, preferences, and behavioral patterns as structured entities and relationships." The key insight: short-term coherence and long-term personalization require different storage strategies.

**SuperLocalMemory's privacy-first pattern** (arXiv:2603.02240):
For agents that can't send data to cloud services: SQLite + FTS5 for storage, Leiden-based knowledge graph clustering for organization, adaptive learning-to-rank for retrieval. Achieved 104% improvement in ranking quality (NDCG@5). Built-in Bayesian trust defense against memory poisoning attacks (72% trust degradation for malicious memories).

---

### 7. Session Journaling → Pattern Detection → Behavioral Adaptation

This is the self-improvement loop that turns raw interactions into lasting behavioral changes:

**Step 1 — Session journaling:**
After each session, run an async LLM call to produce a structured entry:
```json
{
  "session_id": "...",
  "summary": "User asked for refactoring help, rejected 2 approaches citing overengineering, accepted simple extract-method solution",
  "decisions": ["User prefers minimal changes", "User values simplicity over DRY"],
  "errors": ["Initially proposed Strategy pattern — user found it over-engineered"],
  "learnings": ["This user's aesthetic: pragmatic, readable, minimal"],
  "user_signals": {
    "explicit_rejections": 2,
    "acceptance_turn": 3,
    "estimated_satisfaction": "medium-high"
  }
}
```

This is what Pub's `SessionJournaler` does — fire-and-forget after each task, stored in `session_journal` table.

**Step 2 — Pattern detection (reflection):**
Periodically (every N sessions or T minutes), scan journal entries for recurring patterns:
- 3+ sessions where user rejected verbose responses → propose memory: "User prefers concise responses"
- 5+ sessions where user asked follow-up about implementation details → propose memory: "User wants implementation context upfront"
- Pattern of session success correlating with code examples → procedural update: "Include runnable examples by default"

Pub's `ReflectionEngine` implements this: reads journal + memories + SOUL.md, uses LLM to detect patterns, proposes memories and SOUL patches.

**Step 3 — Behavioral adaptation:**
Apply learned patterns at three levels:
1. **In-prompt injection** (fast, no training): Inject preference summaries into system prompt. "This user prefers bullet points over prose, technical depth over simplicity."
2. **Memory-based context** (medium): Retrieve relevant past interactions and inject as few-shot examples.
3. **Policy update** (slow, high-impact): Update SOUL.md / procedural rules with confirmed patterns.

**The STEAM framework's adaptive evolution** (arXiv:2601.16872) provides the clearest pattern for step 2: memory consolidation (refining existing memories) + memory formation (capturing new interest dimensions) applied to atomic memory units organized in communities.

---

### 8. Recommendation System Patterns Applied to Agents

Recommendation systems have decades of research on user modeling. Key transferable patterns:

**Matrix factorization applied to agent decisions:**
Think of agent "actions" (response styles, topics, formats) as items, and user preferences as the latent factors. Weighted Matrix Factorization handles unobserved interactions (turns where no explicit feedback was given) by down-weighting rather than ignoring them.

**Collaborative filtering for cross-user signals:**
New users share preferences with similar existing users. The STEAM framework organizes memories across users into communities and generates prototype memories for signal propagation — addressing the fundamental problem that individual interaction data is sparse.

**Two-stage candidate generation + ranking:**
Stage 1: Broad candidate generation from all possible response strategies using embedding similarity.
Stage 2: Personalized ranking using the full user profile + contextual signals.
This is how modern recommendation systems work (Google's ML recommendation course) and maps naturally to agent response generation.

**Generative recommendations** (arXiv:2510.27157):
LLMs reconceptualize recommendation as a generation task. Instead of ranking pre-defined items, the model generates personalized responses/items directly. This unifies the recommendation and generation steps, enabling more natural personalization.

**Addressing popularity bias:**
Default response strategies accumulate more training signal, creating a feedback loop that biases agents toward "average" responses. Contextual bandits with uncertainty bonuses (UCB) explicitly counteract this by exploring undersampled strategies.

---

### 9. The Cold-Start Problem and How to Solve It

New users have no history. Without history, there is no personalization. Approaches:

**Onboarding elicitation (ask directly):**
A minimal onboarding questionnaire (3-5 questions) covering the highest-variance preference dimensions. Calibrate question value using information-theoretic measures — ask about dimensions where the model is most uncertain. BED-LLM (arXiv:2508.21184) formalizes this as Bayesian Experimental Design.

The VARK profiling approach (arXiv:2603.03309) categorizes users by cognitive style (Visual/Auditory/Reading/Kinesthetic) from minimal initial data, enabling cold-start recommendations even before extensive interaction history.

**Cross-domain transfer:**
If the user exists in another context (different product, previous account), transfer preference signals. The S2CDR framework (arXiv:2603.02725) demonstrates cross-domain preference transfer for cold-start users.

**Meta-learning for rapid adaptation:**
MetaDrug's dual-adaptation (arXiv:2601.22820) — self-adaptation (from user's own limited history) + peer-adaptation (from similar users) with uncertainty-aware filtering — provides the clearest pattern. Meta-learning pre-trains the agent to learn quickly from few examples, so even 2-3 interactions provide meaningful personalization signal.

**Hierarchical policy transfer:**
Start with group-level policies (inferred from user demographics or context), then refine as individual data accumulates. The hierarchical contextual bandit approach (arXiv:2601.14333) provides the formal framework.

**Using LLM prior knowledge:**
LLMs have absorbed vast amounts of human preference patterns. For cold-start users, lean on LLM priors ("typical expert users prefer X") and correct toward individual preferences as data accumulates. The PLUS framework (arXiv:2507.13579) demonstrated 25% better zero-shot performance on new users through better user summarization.

**Proactive questioning:**
Once the agent identifies a preference gap (topic where it has no user-specific data), use active elicitation. The CPER framework's "from guessing to asking" pattern is the most direct implementation: identify gaps, generate targeted questions, update user model.

**Cold start timeline:**
- Turn 0: Use LLM priors + onboarding data
- Turn 1-5: Heavy active elicitation, uncertainty sampling
- Turn 5-20: Implicit signal accumulation, collaborative transfer from similar users
- Turn 20+: Individual model becomes reliable, shift to exploitation

---

### 10. Communication Style Adaptation

Beyond factual preferences, agents should adapt how they communicate:

**Style dimensions to track:**
- Formality (casual ↔ professional)
- Verbosity (terse ↔ elaborate)
- Technicality (layman ↔ expert)
- Format preference (prose ↔ lists ↔ tables ↔ code)
- Tone (warm/empathetic ↔ clinical/neutral)
- Proactivity (answer-only ↔ suggest related things)

**Implicit style signals:**
- User uses casual language → they're comfortable with casual responses
- User writes in short sentences → they prefer concise responses
- User always asks for code examples → default to providing them
- User never reads long explanations (evidenced by follow-ups) → shorten

**HumAIne-Chatbot's dual-signal approach** (arXiv:2509.04303):
Pre-trains on diverse virtual personas to establish a broad prior, then refines per-user using:
- Implicit signals: typing speed, sentiment analysis, engagement duration
- Explicit signals: direct likes/dislikes

The key architecture: user profile conditions both content and style of dialogue policy decisions.

**Bidirectional persona modeling** (arXiv:2204.07372):
Traditional dialogue systems model the agent's persona but ignore the user's. The implicit persona detection paper showed that human-like conversation emerges when agents infer and respond to the user's conversational characteristics — not just express their own persona.

---

### 11. Handling Preference Drift

User preferences change over time. A static user model becomes misleading.

**Types of drift:**
- *Abrupt drift*: User changes job, now needs expert-level content in a new domain
- *Gradual drift*: User skill increases over months, needs less basic explanation
- *Seasonal drift*: User wants brief responses during busy periods, detailed ones when relaxed
- *Contextual drift*: Same user prefers different styles for different task types

**Detection approaches:**
Temporal Matrix Factorization (arXiv:1510.05263) tracks each user's latent preference vector across time steps using a linear transition model. Key finding: "performance gains are mostly from users who indeed have concept drift in their latent vectors" — the system automatically identifies users experiencing genuine preference shifts.

**Memory invalidation patterns:**
Zep's approach: every extracted fact has `valid_at` and `invalid_at` timestamps. New contradicting information doesn't overwrite old facts — it invalidates them and creates new ones with the current timestamp. This preserves preference history while ensuring current preferences dominate retrieval.

Cairn's contradiction engine uses an LLM judge to classify contradictions as ADD/UPDATE/SKIP/CONTRADICT, ensuring new information is processed relative to existing knowledge rather than blindly accumulated.

**Recency weighting:**
Apply exponential decay to older preference signals. Recent interactions get higher weight in the user model. The decay rate should match the expected drift rate for the user/domain.

---

## Code Examples

### Basic Memory-Augmented Agent Pattern (Python)

```python
import json
from datetime import datetime
from typing import Optional

class PersonalizedAgent:
    def __init__(self, user_id: str, llm_client, memory_client):
        self.user_id = user_id
        self.llm = llm_client
        self.memory = memory_client
        self.session_log = []

    def respond(self, user_message: str) -> str:
        # 1. Retrieve relevant memories
        memories = self.memory.search(
            query=user_message,
            user_id=self.user_id,
            limit=5
        )
        memory_context = "\n".join([m["memory"] for m in memories])

        # 2. Build personalized system prompt
        system_prompt = f"""You are a personalized assistant.
What you know about this user:
{memory_context}

Apply these preferences naturally in your response."""

        # 3. Generate response
        response = self.llm.chat(
            system=system_prompt,
            messages=self.session_log + [{"role": "user", "content": user_message}]
        )

        # 4. Log turn for memory extraction
        self.session_log.append({"role": "user", "content": user_message})
        self.session_log.append({"role": "assistant", "content": response})

        # 5. Extract and store new memories (async / background)
        self.memory.add(
            messages=[
                {"role": "user", "content": user_message},
                {"role": "assistant", "content": response}
            ],
            user_id=self.user_id
        )

        return response

    def reflect_on_session(self) -> dict:
        """Extract structured insights from this session for episodic memory."""
        reflection_prompt = """Analyze this conversation and extract:
1. User preferences revealed (explicit or implicit)
2. What worked well
3. What the user rejected or corrected
4. Inferred expertise level
5. Communication style signals

Return as JSON."""

        return json.loads(self.llm.chat(
            system=reflection_prompt,
            messages=self.session_log
        ))
```

### Contextual Bandit for Response Style Selection (Python)

```python
import numpy as np
from dataclasses import dataclass, field
from typing import Dict

@dataclass
class StyleArm:
    name: str
    n_tries: int = 0
    total_reward: float = 0.0
    sum_sq: float = 0.0  # for variance tracking

class ResponseStyleBandit:
    """LinUCB-inspired bandit for selecting response style."""

    def __init__(self, alpha: float = 1.0):
        self.alpha = alpha  # exploration coefficient
        self.arms: Dict[str, StyleArm] = {
            "concise": StyleArm("concise"),
            "detailed": StyleArm("detailed"),
            "code_first": StyleArm("code_first"),
            "explanation_first": StyleArm("explanation_first"),
            "bulleted": StyleArm("bulleted"),
        }

    def select(self, context: dict) -> str:
        """Select style with highest UCB score."""
        best_arm = None
        best_ucb = -float("inf")
        total_tries = sum(a.n_tries for a in self.arms.values())

        for name, arm in self.arms.items():
            if arm.n_tries == 0:
                return name  # explore untried arms first

            mean_reward = arm.total_reward / arm.n_tries
            # UCB bonus: decays as more data is collected
            ucb_bonus = self.alpha * np.sqrt(
                np.log(total_tries + 1) / arm.n_tries
            )
            ucb = mean_reward + ucb_bonus

            if ucb > best_ucb:
                best_ucb = ucb
                best_arm = name

        return best_arm

    def update(self, arm_name: str, reward: float):
        """Update arm stats with observed reward signal."""
        arm = self.arms[arm_name]
        arm.n_tries += 1
        arm.total_reward += reward

    def reward_from_signals(self, signals: dict) -> float:
        """Convert implicit feedback signals to reward scalar."""
        reward = 0.0
        # Positive signals
        if signals.get("no_edit"): reward += 0.3
        if signals.get("no_regenerate"): reward += 0.2
        if signals.get("high_dwell_time"): reward += 0.2
        if signals.get("explicit_positive"): reward += 1.0
        # Negative signals
        if signals.get("heavy_edit"): reward -= 0.3
        if signals.get("regenerated"): reward -= 0.5
        if signals.get("explicit_negative"): reward -= 1.0
        return reward
```

### User Profile with Temporal Tracking

```python
from datetime import datetime, timedelta
from dataclasses import dataclass, field
from typing import List, Optional
import json

@dataclass
class PreferenceFact:
    key: str
    value: str
    confidence: float
    created_at: datetime
    last_confirmed_at: datetime
    valid_until: Optional[datetime] = None  # None = still valid

    def is_stale(self, decay_days: int = 90) -> bool:
        """Preference decays if not confirmed within decay window."""
        return (datetime.now() - self.last_confirmed_at).days > decay_days

    def effective_confidence(self) -> float:
        """Decay confidence over time."""
        days_old = (datetime.now() - self.last_confirmed_at).days
        decay_factor = max(0.1, 1.0 - (days_old / 180))
        return self.confidence * decay_factor

class UserProfile:
    def __init__(self, user_id: str):
        self.user_id = user_id
        self.facts: List[PreferenceFact] = []

    def upsert_fact(self, key: str, value: str, confidence: float):
        """Add new fact or update if key exists and new confidence is higher."""
        existing = next((f for f in self.facts if f.key == key), None)
        if existing:
            if value != existing.value:
                # Preference changed: invalidate old, add new
                existing.valid_until = datetime.now()
            else:
                # Preference confirmed: update confidence and timestamp
                existing.last_confirmed_at = datetime.now()
                existing.confidence = max(existing.confidence, confidence)
                return
        self.facts.append(PreferenceFact(
            key=key, value=value, confidence=confidence,
            created_at=datetime.now(), last_confirmed_at=datetime.now()
        ))

    def get_active_facts(self) -> List[PreferenceFact]:
        """Return non-stale, non-invalidated facts sorted by confidence."""
        active = [
            f for f in self.facts
            if f.valid_until is None and not f.is_stale()
        ]
        return sorted(active, key=lambda f: f.effective_confidence(), reverse=True)

    def to_prompt_context(self, top_k: int = 10) -> str:
        facts = self.get_active_facts()[:top_k]
        return "\n".join([
            f"- {f.key}: {f.value} (confidence: {f.effective_confidence():.2f})"
            for f in facts
        ])
```

### Active Elicitation with Information Gain Estimation

```python
class ActiveElicitationManager:
    """Decides when and what to ask users to fill preference gaps."""

    PREFERENCE_DIMENSIONS = [
        "verbosity",      # concise vs detailed
        "formality",      # casual vs professional
        "technicality",   # beginner vs expert
        "format",         # prose vs lists vs code
        "proactivity",    # answer-only vs suggest-more
    ]

    def __init__(self, user_profile: UserProfile, threshold: float = 0.3):
        self.profile = user_profile
        self.threshold = threshold  # ask when confidence below this

    def find_gaps(self) -> List[str]:
        """Identify preference dimensions with low or missing confidence."""
        known_dims = {f.key: f.effective_confidence()
                      for f in self.profile.get_active_facts()}
        gaps = []
        for dim in self.PREFERENCE_DIMENSIONS:
            conf = known_dims.get(dim, 0.0)
            if conf < self.threshold:
                gaps.append(dim)
        return gaps

    def should_ask(self, turn_number: int, gaps: List[str]) -> bool:
        """Only ask during natural pauses, not on every turn."""
        if not gaps:
            return False
        # Ask at onboarding (turn 1), then only occasionally
        if turn_number == 1:
            return True
        # Ask at natural checkpoints, not too frequently
        if turn_number > 5 and turn_number % 10 == 0 and gaps:
            return True
        # Ask after completing a task (lower friction moment)
        return False

    def generate_elicitation_question(self, gap_dimension: str) -> str:
        templates = {
            "verbosity": "For responses like this, would you prefer I keep them brief or go into more detail?",
            "technicality": "How familiar are you with this topic? Should I explain concepts or assume background knowledge?",
            "format": "Do you find bullet-point lists or flowing prose easier to read for this kind of content?",
            "proactivity": "Should I stick to exactly what you asked, or would it be helpful if I flagged related things you might want to know?",
            "formality": "Would you prefer I communicate more formally or keep things casual?",
        }
        return templates.get(gap_dimension, "What format works best for you?")
```

---

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Forgetting across sessions | LLMs are stateless; naive implementations replay full context | Extract + store semantic facts; inject summaries, not raw history |
| Overfitting to recent feedback | Latest interaction dominates the user model | Apply recency weighting with decay; don't discard older confirmed preferences |
| Confusing user and agent persona | Dialogue systems model agent personality but ignore user's | Model both: agent style + user characteristics |
| Implicit signal noise | Clicks, dwell time reflect curiosity not satisfaction | Use mental reward models; combine implicit + explicit signals |
| Homogeneous recommendations | Popularity bias in training data | Contextual bandit UCB bonuses force exploration of underrepresented styles |
| Memory poisoning | Malicious or accidental bad data corrupts user model | Bayesian trust scores; don't update on single contradictory signals |
| Static cold start priors | LLM defaults feel generic | Use hierarchical priors (population → cohort → individual) that refine with data |
| Asking too much / onboarding friction | Over-eager elicitation | Use information gain to select only high-value questions; ask at natural pauses |
| Preference conflation during update | One update overwrites related-but-distinct preferences | STEAM-style atomic memory units; one unit per preference dimension |
| Ignoring preference drift | User model becomes stale over months | Timestamp all preferences; decay confidence; detect concept drift in interaction patterns |

---

## Best Practices

1. **Extract facts, not transcripts**: Store compressed semantic memories rather than raw conversation history. A 200-token preference summary replaces a 10,000-token conversation replay. (Mem0 benchmarks: 90% fewer tokens, 91% faster)

2. **Combine explicit and implicit signals**: Neither alone is sufficient. Explicit feedback (likes/dislikes) is high-fidelity but sparse. Implicit signals (edit distance, dwell time, regeneration) are dense but noisy. The optimal reward signal combines both.

3. **Use low-rank preference decomposition**: User preferences live in a low-dimensional space. Reward factorization with ~10 user responses can significantly personalize behavior. Full per-user models are unnecessary and don't generalize.

4. **Timestamp every preference fact**: User preferences change. Every stored preference should have a `created_at` and `last_confirmed_at` timestamp. Apply confidence decay for facts not recently confirmed.

5. **Prefer "ask over guess" for high-stakes preferences**: Active elicitation outperforms passive inference for important dimensions. Use information-theoretic measures to identify which questions have highest value.

6. **Apply the session → reflect → adapt loop**: Don't only update in real-time. Run periodic reflection across session journal entries to detect patterns that single sessions cannot reveal.

7. **Model user AND agent persona**: Effective personalization requires understanding the user's communication characteristics, not just maintaining the agent's personality.

8. **Handle cold start with meta-learning priors**: Use MAML-style meta-learning so the agent can adapt to new users from 2-3 interactions. Supplement with collaborative transfer from similar users.

9. **Guard against memory poisoning**: Apply trust scoring to incoming preference signals. Single contradicting data points should reduce confidence, not immediately overwrite existing knowledge. Use Bayesian trust defense for multi-user shared memory systems.

10. **Separate memory tiers by write frequency**: Core stable facts (expertise level, communication style) change rarely — verify before updating. Session-level context changes every conversation — update freely. Pattern-level procedural rules change slowly — require multi-session evidence.

---

## Relevancy Analysis for Pub / Cairn

### Pub (Signal-to-Action OS)

The existing architecture already has the right primitives:
- `memory_items` table + RAG = semantic memory
- `session_journal` + `ReflectionEngine` = episodic → procedural loop
- SOUL.md = procedural memory

**Gaps and recommendations:**
- **Communication style tracking**: Add style dimensions (verbosity, technicality, format) as explicit semantic memory keys. Inject into system prompt alongside factual memories.
- **Implicit feedback capture**: Current system uses explicit feedback. Adding passive signals (response acceptance rate, edit detection, follow-up question detection) would improve personalization signal density.
- **Contextual bandit for response strategies**: The existing tool-loop decisions could benefit from a lightweight bandit tracking which response formats the user engages with most.
- **Preference timestamps**: Add `last_confirmed_at` to `memory_items`. Run weekly decay job to reduce confidence on stale preferences.
- **Active elicitation**: After detecting low-confidence preference dimensions in a conversation, surface a single targeted question at the end of the task.

### Cairn

Cairn's memory pipeline (extraction → embedding → contradiction judgment → ADD/UPDATE/SKIP/CONTRADICT) implements the core loop well.

**Recommendations:**
- **STEAM-style atomic memory units**: Ensure preferences are stored at the right granularity — not one monolithic "user profile" but separate facts per dimension
- **Meta reward modeling**: When implementing RLHF for Cairn's GLM-5 loop, prefer reward factorization (arXiv:2503.06358) over full per-user reward models — achieves personalization with ~10 user responses
- **Bandit for active elicitation**: Track which preference dimensions have low confidence, use information gain to select elicitation questions

---

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Toward Personalized LLM-Powered Agents](https://arxiv.org/abs/2602.22680) | Survey paper | Most comprehensive recent survey of personalized agent architecture |
| [Memory for Autonomous LLM Agents](https://arxiv.org/abs/2603.07670) | Survey paper | Definitive treatment of memory mechanisms, evaluation, emerging frontiers |
| [RLHF Tutorial (HuggingFace)](https://huggingface.co/blog/rlhf) | Blog post | Best practical RLHF introduction with open-source tooling (TRL, TRLX) |
| [LangChain Memory for Agents](https://blog.langchain.com/memory-for-agents) | Blog post | Concrete implementation patterns for procedural/semantic/episodic memory |
| [Mem0 GitHub](https://github.com/mem0ai/mem0) | Code repo | Production-ready memory layer; +26% accuracy vs OpenAI Memory |
| [Zep GitHub](https://github.com/getzep/zep) | Code repo | Temporal knowledge graph; temporal reasoning about preference evolution |
| [MemGPT paper](https://arxiv.org/abs/2310.08560) | Research paper | OS-inspired virtual context management for persistent agent memory |
| [Latent Preference Discovery Benchmark](https://arxiv.org/abs/2510.17132) | Research paper | Rigorous evaluation of LLM preference inference; 32-98% success variance |
| [PLUS: User Summarization for RLHF](https://arxiv.org/abs/2507.13579) | Research paper | Best approach for scalable personalized RLHF with interpretable user models |
| [Reward Factorization](https://arxiv.org/abs/2503.06358) | Research paper | Lite RLHF personalization with ~10 user responses; 67% GPT-4o win rate |
| [Neural Contextual Bandits Tutorial](https://arxiv.org/abs/2312.14037) | Tutorial | Comprehensive contextual bandit coverage including exploration bias |
| [Meta Reward Modeling](https://arxiv.org/abs/2601.18731) | Research paper | MAML-based cold-start personalization; few-shot adaptation |
| [PersonaAgent with GraphRAG](https://arxiv.org/abs/2511.17467) | Research paper | Knowledge graph + community detection for persona-aligned prompts |
| [CPER: From Guessing to Asking](https://arxiv.org/abs/2503.12556) | Research paper | Active elicitation that outperforms passive inference |
| [MTRec: Mental Reward Models](https://arxiv.org/abs/2509.22807) | Research paper | Beyond implicit feedback noise: modeling true user satisfaction |
| [Temporal Matrix Factorization](https://arxiv.org/abs/1510.05263) | Research paper | Concept drift detection and handling in user preference models |
| [Inside Out Memory Trees](https://arxiv.org/abs/2601.05171) | Research paper | RL-driven memory operations (ADD/UPDATE/DELETE) for persistent dialogue |
| [PrefEval Benchmark](https://arxiv.org/abs/2502.09597) | Research paper | Measures preference following failure modes (< 10% at 10 turns) |

---

## Self-Evaluation

```json
{
  "coverage": 9,
  "diversity": 8,
  "examples": 8,
  "accuracy": 8,
  "gaps": [
    "Multimodal preference signals (image/voice) not covered",
    "Privacy-preserving personalization (federated learning) mentioned briefly",
    "Evaluation metrics for personalization quality not deeply covered",
    "Online vs offline learning tradeoffs could be more detailed"
  ]
}
```

---

*Generated by /learn from 42 sources analyzed on 2026-03-19.*
*See `resources/active-learning-user-adaptation-sources.json` for full source metadata.*
