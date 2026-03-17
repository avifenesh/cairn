# Pub v2 — Implementation Phases

> Five phases from foundation to production. Each phase produces a working increment.

## Phase Dependency Graph

```
Phase 1: Foundation (Event Bus + LLM Client + SQLite)
    │
    ├── Phase 2a: Tool System ──────────────┐
    │                                        │
    ├── Phase 2b: Task Engine ──────────┐    │
    │                                   │    │
    └── Phase 2c: Memory System ───┐    │    │
                                   │    │    │
                                   ▼    ▼    ▼
                            Phase 3: Agent Core
                            (ReAct loop wires all together)
                                   │
                    ┌──────────────┼──────────────┐
                    ▼              ▼              ▼
            Phase 4a:       Phase 4b:       Phase 4c:
            Server +        Signal Plane    Plugin & Skills
            Protocols
                    │              │              │
                    └──────────────┼──────────────┘
                                   ▼
                            Phase 5: Integration
                            (Frontend migration, always-on,
                             open-source release)
```

## Phase 1: Foundation

**Goal:** Binary that connects to GLM-5 Turbo, streams a response, and stores it in SQLite.

**Duration estimate:** 1 week

**Can be parallelized:** 1.1 and 1.2 and 1.3 are independent.

| Subphase | From Piece | Description | Parallel? |
|----------|-----------|-------------|-----------|
| 1.1 | Piece 1 (1.1-1.2) | Event bus core + event types | ✅ Independent |
| 1.2 | Piece 2 (2.1-2.3) | LLM types + SSE parser + GLM provider | ✅ Independent |
| 1.3 | SQLite setup | Database connection, migrations, base tables | ✅ Independent |
| 1.4 | Piece 2 (2.5-2.6) | Retry/fallback + budget tracker | Needs 1.2 |
| 1.5 | Integration test | CLI binary: `pub-go chat "hello"` streams GLM response | Needs 1.1-1.4 |

**Deliverable:** `pub-go chat "hello"` → streams GLM-5 Turbo response to stdout.

---

## Phase 2: Core Systems (parallel tracks)

**Goal:** Tools execute, tasks queue and isolate, memory stores and retrieves.

**Duration estimate:** 2 weeks (3 tracks in parallel)

### Phase 2a: Tool System

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 2a.1 | Piece 3 (3.1) | Tool interface + Define[P] helper | Nothing |
| 2a.2 | Piece 3 (3.2) | Registry with mode filtering | Phase 3 |
| 2a.3 | Piece 3 (3.3) | Permission engine (wildcard rules) | Phase 3 |
| 2a.4 | Piece 3 (3.4) | Built-in tools: readFile, writeFile, editFile, shell, git | Phase 3 |
| 2a.5 | Piece 3 (3.5) | MCP toolset adapter (mcp-go) | Phase 4c |
| 2a.6 | Piece 3 (3.6-3.7) | Result formatting + tests | — |

### Phase 2b: Task Engine

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 2b.1 | Piece 5 (5.1) | Task types + SQLite store | Nothing |
| 2b.2 | Piece 5 (5.2) | Priority queue (in-memory heap) | Phase 3 |
| 2b.3 | Piece 5 (5.3) | Worktree manager (git operations) | Phase 3 |
| 2b.4 | Piece 5 (5.4) | Lease-based claiming + reaper | 2b.1, 2b.2 |
| 2b.5 | Piece 5 (5.5) | Dedup guard | 2b.1 |
| 2b.6 | Piece 5 (5.6-5.8) | Worker pool + tests | 2b.1-2b.5 |

### Phase 2c: Memory System

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 2c.1 | Piece 6 (6.1) | Memory store (SQLite) | Nothing |
| 2c.2 | Piece 6 (6.2) | Embedding service | 2c.3 |
| 2c.3 | Piece 6 (6.3) | RAG search with MMR | Phase 3 |
| 2c.4 | Piece 6 (6.5) | Soul loader + hot-reload | Phase 3 |
| 2c.5 | Piece 6 (6.6) | Context builder (token-budgeted) | 2c.1-2c.4, Phase 3 |
| 2c.6 | Piece 6 (6.8-6.9) | Compaction + decay + tests | 2c.1 |

**Deliverables:**
- Tools can be registered, filtered by mode, executed with permission checks
- Tasks queue, claim via lease, execute in isolated worktrees
- Memories store, embed, search via RAG

---

## Phase 3: Agent Core

**Goal:** Full ReAct agent loop that takes user input, uses LLM + tools, manages sessions.

**Duration estimate:** 1.5 weeks

**Requires:** Phase 1 + Phase 2 (all three tracks)

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 3.1 | Piece 4 (4.1) | Agent interface + event model | Nothing |
| 3.2 | Piece 4 (4.2) | ReAct loop implementation | 3.1 |
| 3.3 | Piece 4 (4.3) | Session store (SQLite) | 3.1 |
| 3.4 | Piece 4 (4.5) | Agent modes (talk/work/coding) + system prompt | 3.2 |
| 3.5 | Piece 4 (4.6) | Sub-agent spawning via task engine | 3.2, Phase 2b |
| 3.6 | Piece 4 (4.4) | Session compaction (LLM summarization) | 3.2, 3.3 |
| 3.7 | Piece 4 (4.7) | Checkpoint/resume | 3.2, 3.3 |
| 3.8 | Integration test | Full loop: user → LLM → tool → LLM → response | All |

**Deliverable:** `pub-go chat "read package.json and tell me the version"` → agent reads file via tool, responds.

---

## Phase 4: Server & Extensions (parallel tracks)

**Goal:** HTTP API, SSE streaming, signal ingestion, skills, plugins.

**Duration estimate:** 2 weeks (3 tracks in parallel)

### Phase 4a: Server + Protocols

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 4a.1 | Piece 9 (9.1-9.2) | HTTP server + auth middleware | Nothing |
| 4a.2 | Piece 9 (9.3) | REST routes (feed, tasks, memories, sessions) | 4a.1 |
| 4a.3 | Piece 9 (9.4) | SSE broadcaster + replay buffer | 4a.1, Phase 1 (bus) |
| 4a.4 | Piece 9 (9.5) | Assistant message endpoint → agent | 4a.1, Phase 3 |
| 4a.5 | Piece 9 (9.6) | Voice endpoints (whisper + Polly) | 4a.1 |
| 4a.6 | Piece 9 (9.7) | MCP server | 4a.1, Phase 2a |
| 4a.7 | Piece 9 (9.8) | A2A server | 4a.1, Phase 2b |
| 4a.8 | Piece 9 (9.9-9.11) | Static files, rate limiting, CORS, tests | 4a.1 |

### Phase 4b: Signal Plane

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 4b.1 | Piece 7 (7.1) | Event store + dedup | Nothing |
| 4b.2 | Piece 7 (7.2) | Poll scheduler | 4b.1 |
| 4b.3 | Piece 7 (7.3) | GitHub poller | 4b.1, 4b.2 |
| 4b.4 | Piece 7 (7.4) | Gmail poller | 4b.1, 4b.2 |
| 4b.5 | Piece 7 (7.5) | Generic pollers (Reddit, HN, npm, crates) | 4b.1, 4b.2 |
| 4b.6 | Piece 7 (7.6) | Webhook handler | 4b.1 |
| 4b.7 | Piece 7 (7.7-7.8) | Digest runner + tests | 4b.1, Phase 1 (LLM) |

### Phase 4c: Plugin & Skill System

| Subphase | From Piece | Description | Blocks |
|----------|-----------|-------------|--------|
| 4c.1 | Piece 8 (8.1-8.2) | Skill types + discovery | Nothing |
| 4c.2 | Piece 8 (8.3) | Skill hot-reload | 4c.1 |
| 4c.3 | Piece 8 (8.4) | Skill injection into system prompt | 4c.1, Phase 3 |
| 4c.4 | Piece 8 (8.5) | Plugin interface + hook system | Phase 1 (bus) |
| 4c.5 | Piece 8 (8.6) | Built-in plugins | 4c.4 |
| 4c.6 | Piece 8 (8.7) | Plugin loading from config | 4c.4 |
| 4c.7 | Piece 8 (8.8-8.9) | ClawHub client + tests | 4c.1, 4c.2 |

**Deliverables:**
- Full HTTP API compatible with current frontend
- SSE streaming to web client
- GitHub/Gmail/HN polling + digest generation
- Skills loaded from SKILL.md files
- Plugins extend behavior via hooks

---

## Phase 5: Integration & Release

**Goal:** Replace Node.js backend, always-on agent, open-source release.

**Duration estimate:** 2 weeks

| Subphase | Description | Blocks |
|----------|-------------|--------|
| 5.1 | API compatibility layer — ensure Go server matches Node API contract | Phase 4a |
| 5.2 | Frontend migration — test web client against Go backend | Phase 4a |
| 5.3 | Always-on agent loop (idle mode, proactive behavior) | Phase 3, 4b, 6 (memory) |
| 5.4 | Session migration — import SQLite data from v1 | Phase 3.3 |
| 5.5 | Episodic memory: session journaler | Phase 3, 6 |
| 5.6 | Reflection engine (pattern detection → memories + soul patches) | Phase 6.7 |
| 5.7 | Performance testing (30-day soak test) | All |
| 5.8 | Documentation (README, CONTRIBUTING, architecture guide) | All |
| 5.9 | CI/CD (GitHub Actions: build, test, release binaries) | All |
| 5.10 | Open-source release (LICENSE, cleanup, public repo) | All |

**Deliverable:** Single Go binary replaces Node.js backend. `curl -L pub.dev/install | sh`.

---

## What Can Run In Parallel (Summary)

```
Week 1:   [1.1 Event Bus] | [1.2 LLM Client] | [1.3 SQLite]
          └─────────────────────┬───────────────────────┘
                                │ 1.4, 1.5
Week 2-3: [2a Tool System] | [2b Task Engine] | [2c Memory]
          └─────────────────────┬───────────────────────┘
                                │
Week 4:                  [3. Agent Core]
                                │
Week 5-6: [4a Server] | [4b Signal Plane] | [4c Plugins/Skills]
          └─────────────────────┬───────────────────────┘
                                │
Week 7-8:              [5. Integration & Release]
```

**Maximum parallelism:** 3 tracks simultaneously (Weeks 2-3 and 5-6).

---

## Wiring: How Everything Connects to the Agent

```go
func main() {
    // 1. Foundation
    bus := eventbus.New()
    db := sqlite.Open("pub.db")
    llmClient := llm.NewRegistry(config.Providers).Default()

    // 2. Core systems
    toolRegistry := tool.NewRegistry()
    toolRegistry.Register(builtin.AllTools()...)
    taskEngine := task.NewEngine(db, bus)
    memoryService := memory.NewService(db, embedder, bus)
    soul := memory.LoadSoul("SOUL.md")

    // 3. Agent
    contextBuilder := memory.NewContextBuilder(memoryService, soul)
    agent := agent.NewReAct(agent.Config{
        LLM:            llmClient,
        Tools:          toolRegistry,
        Tasks:          taskEngine,
        Memory:         memoryService,
        ContextBuilder: contextBuilder,
        Bus:            bus,
    })

    // 4. Server
    server := server.New(server.Config{
        Agent:   agent,
        Tasks:   taskEngine,
        Bus:     bus,
        DB:      db,
    })

    // 4b. Signal plane
    signalPlane := signal.New(db, bus, config.Sources)
    signalPlane.StartPolling()

    // 4c. Skills
    skillService := skills.NewService(config.SkillDirs)
    skillService.Watch()

    // 5. Always-on agent loop
    agentLoop := agentloop.New(agent, taskEngine, memoryService, bus, config)
    agentLoop.Start() // background goroutine: tick → decide → act → learn

    // Start server
    server.Start(":8787")
}
```

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Go SQLite performance vs Node better-sqlite3 | Benchmark early (Phase 1.3). WAL mode + connection pooling. |
| LLM provider quirks (GLM network_error) | Port existing retry logic verbatim. Test with real API. |
| Frontend API incompatibility | Write API contract test suite against both Node and Go servers. |
| Worktree corruption on crash | Worktree cleanup on startup (reap orphans). Periodic health check. |
| Skill ecosystem compatibility | Test top 20 ClawHub skills during Phase 4c. |
| Memory growth over months | Compaction + decay from day one (Phase 2c). Soak test in Phase 5. |

---

## Success Criteria (per phase)

| Phase | "Done" means |
|-------|-------------|
| 1 | `pub-go chat "hello"` streams response from GLM-5 Turbo |
| 2 | Tools execute in worktrees. Tasks queue and lease. Memories RAG-search. |
| 3 | Full ReAct loop: user → tools → response. Sessions persist. Modes work. |
| 4 | HTTP API serves frontend. SSE streams. Polls GitHub/Gmail. Skills load. |
| 5 | Node.js backend fully replaced. Binary runs 30 days stable. Open-sourced. |
