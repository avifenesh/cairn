# Phase 10.5: Agent Management UI & Orchestration

> Frontend pages and backend features for managing agent types, assigning tasks to specific agents, scheduling agent runs, and chatting with specialist agents directly.
> Depends on: Phase 10 (AGENT.md infrastructure) — completed.

## Motivation

Phase 10 built the backend: AGENT.md files, discovery service, hot-reload, SubagentRunner resolution, REST endpoints. But there's no way for users to:
1. See what agent types exist and their capabilities
2. Create or edit custom agent types
3. Manually assign a task to a specific agent type
4. Schedule recurring agent runs via cron
5. Chat directly with a specialist agent (e.g., open a "researcher" session)

## Current State (after Phase 10)

| Feature | Backend | Frontend | Status |
|---------|---------|----------|--------|
| List agent types | `GET /v1/agent-types` | None | Backend only |
| Get agent type detail | `GET /v1/agent-types/{name}` | None | Backend only |
| Create custom type | `agenttype.Service.Create()` | None | No REST endpoint yet |
| Delete custom type | `agenttype.Service.Delete()` | None | No REST endpoint yet |
| Spawn agent on task | `cairn.spawnSubagent` tool | None | LLM-only (no UI) |
| Schedule agent run | None | None | Not implemented |
| Chat with agent type | None | None | Not implemented |

## Tasks (6 items)

---

### Task 1: Agent Types Frontend Page

**Priority:** High | **Effort:** Medium | **Impact:** Core discoverability

**What:** A `/agents` page (already in routes as stub) that lists all discovered agent types with their name, description, mode, max rounds, tool count, and worktree status.

**Implementation:**

1. **Page: `/agents/+page.svelte`** — Grid/list of agent type cards
   - Each card shows: name, description, mode badge, round limit, tool count
   - Click → detail page
   - "Create Custom Agent" button → create dialog

2. **Detail page: `/agents/[name]/+page.svelte`**
   - Full AGENT.md content rendered as markdown
   - Frontmatter displayed as metadata badges
   - Allowed tools list with descriptions
   - "Edit" button (opens markdown editor, like Soul page)
   - "Delete" button (only for user-created, not bundled)
   - "Run Now" button → task assignment dialog (Task 3)
   - "Chat" button → specialist chat (Task 5)

3. **API client:** `getAgentTypes()`, `getAgentType(name)`, `createAgentType()`, `updateAgentType()`, `deleteAgentType()`

4. **REST endpoints (add to routes.go):**
   - `POST /v1/agent-types` — create custom type
   - `PUT /v1/agent-types/{name}` — update type content
   - `DELETE /v1/agent-types/{name}` — delete user-created type

**Files:**
- New: `frontend/src/routes/agents/+page.svelte`, `frontend/src/routes/agents/[name]/+page.svelte`
- Modify: `frontend/src/lib/api/client.ts`, `internal/server/routes.go`

---

### Task 2: Agent Type Editor (Create/Edit Dialog)

**Priority:** High | **Effort:** Medium | **Impact:** Customization

**What:** A dialog or page for creating/editing AGENT.md files with form fields + markdown editor.

**Implementation:**

1. **Form fields:**
   - Name (only on create, read-only on edit)
   - Description (text input)
   - Mode (select: talk / work / coding)
   - Max rounds (number input, with mode-default hint)
   - Allowed tools (multi-select from available tools, or "all")
   - Worktree isolation (checkbox, only relevant for coding mode)

2. **Markdown editor:**
   - Full AGENT.md body (system prompt content)
   - Preview panel with markdown rendering
   - Same editor component as Soul page

3. **Validation:**
   - Name: lowercase, alphanumeric + hyphens, 1-64 chars
   - Description: required, min 10 chars
   - Max rounds: 1-400 range

4. **Save:** Serializes form → frontmatter + body → PUT/POST to API

**Files:**
- New: `frontend/src/lib/components/agents/AgentTypeEditor.svelte`
- Modify: agents page components

---

### Task 3: Manual Task Assignment to Agent Type

**Priority:** High | **Effort:** Small | **Impact:** Direct user control

**What:** From the UI, manually spawn a specific agent type on a task with custom instructions.

**Implementation:**

1. **"Run Agent" dialog** — Accessible from:
   - Agent type detail page ("Run Now" button)
   - Task detail page (dropdown: "Run with [type]")
   - Command palette / quick action

2. **Dialog fields:**
   - Agent type (pre-selected or dropdown)
   - Instruction (textarea — what the agent should do)
   - Context (optional textarea — parent context)
   - Exec mode (foreground / background toggle)

3. **Backend:** `POST /v1/agent-types/{name}/run`
   ```json
   {
     "instruction": "Review the latest PR for security issues",
     "context": "PR #197 adds new authentication endpoints",
     "execMode": "background"
   }
   ```
   Server calls `SubagentRunner.Spawn()` directly.

4. **Result display:**
   - Foreground: stream events to chat-like panel
   - Background: redirect to task list with status tracking

**Files:**
- New: `frontend/src/lib/components/agents/RunAgentDialog.svelte`
- Modify: `internal/server/routes.go` (new endpoint), agent type pages

---

### Task 4: Cron-to-Agent Binding

**Priority:** Medium | **Effort:** Medium | **Impact:** Recurring automation

**What:** Schedule agent runs via cron. "Every morning at 9am, run researcher to check for new PRs waiting for review."

**Implementation:**

1. **Cron job type extension:**
   Add `agentType` and `agentInstruction` fields to cron job schema:
   ```sql
   ALTER TABLE cron_jobs ADD COLUMN agent_type TEXT DEFAULT '';
   ALTER TABLE cron_jobs ADD COLUMN agent_instruction TEXT DEFAULT '';
   ```

2. **Cron execution handler:**
   In the cron tick handler, if `agent_type` is set, spawn a subagent instead of submitting a generic task:
   ```go
   if job.AgentType != "" {
       subagentRunner.Spawn(ctx, "cron:"+job.ID, &tool.SubagentSpawnRequest{
           Type:        job.AgentType,
           Instruction: job.AgentInstruction,
           ExecMode:    "background",
       })
   }
   ```

3. **REST endpoints:**
   - Update `POST /v1/crons` to accept `agentType` + `agentInstruction`
   - Update `PUT /v1/crons/{id}` similarly

4. **Frontend:**
   - Cron creation dialog: add "Agent Type" dropdown + "Instruction" textarea
   - Cron list: show agent type badge on agent-bound crons

**Files:**
- Modify: `internal/cron/store.go` (schema), `internal/agent/loop.go` (cron execution), `internal/server/routes.go` (REST), cron frontend components

---

### Task 5: Chat with Specialist Agent

**Priority:** Medium | **Effort:** Medium | **Impact:** Direct interaction

**What:** Open a chat session that uses a specific agent type's system prompt and tool set, instead of the default main agent.

**Implementation:**

1. **Session mode override:**
   Add optional `agentType` field to chat session creation:
   ```json
   POST /v1/sessions
   {
     "agentType": "researcher",
     "message": "What new Go features landed in 1.25?"
   }
   ```

2. **InvocationContext override:**
   In `runAgent()`, if session has `agentType`:
   - Resolve type from `agenttype.Service`
   - Override mode, allowed tools, and SubagentSystemHint from the type definition
   - The agent runs with the specialist's constraints

3. **Frontend chat UI:**
   - Agent type selector in chat header (dropdown or pills)
   - Show active agent type badge
   - Tool restrictions visually indicated
   - "Switch to [type]" command in chat

4. **Session metadata:**
   Store `agentType` in session state so it persists across page reloads and compaction.

**Files:**
- Modify: `internal/server/routes.go` (session creation), `internal/agent/react.go` (mode override), chat frontend components

---

### Task 6: Agent Activity Dashboard

**Priority:** Low | **Effort:** Medium | **Impact:** Observability

**What:** Show running and completed agent sessions with their type, status, duration, and output summary.

**Implementation:**

1. **Activity list on /agents page:**
   - Recent agent runs (from ActivityStore)
   - Columns: type, status, started, duration, rounds, instruction preview
   - Filter by type, status
   - Click → session detail with full output

2. **Per-type stats:**
   - On agent type detail page: "Recent Runs" section
   - Success rate, average rounds, average duration

3. **Backend:**
   - `GET /v1/agent-types/{name}/activity` — list recent runs for a type
   - Filters: status, limit, before

**Files:**
- Modify: `internal/server/routes.go`, agent frontend pages

---

## Dependency Graph

```
Task 1 (frontend page)      ── standalone
Task 2 (editor)             ── depends on 1
Task 3 (manual assignment)  ── depends on 1
Task 4 (cron binding)       ── standalone (backend-heavy)
Task 5 (specialist chat)    ── depends on 1
Task 6 (activity dashboard) ── depends on 1
```

Tasks 1 and 4 can start in parallel.

## Suggested Commit Order

1. Task 1 — Agent types page + CRUD endpoints (enables everything else)
2. Task 2 — Editor (natural extension of page)
3. Task 3 — Manual assignment (most impactful user feature)
4. Task 5 — Specialist chat (deepest integration)
5. Task 4 — Cron binding (requires schema migration)
6. Task 6 — Activity dashboard (polish)

## Verification

1. `pnpm check && pnpm test` — frontend clean
2. `go vet ./... && go test -race ./...` — backend clean
3. Manual: browse /agents, see 4 bundled types
4. Manual: create custom agent type, verify it appears in list
5. Manual: edit agent type content, verify hot-reload
6. Manual: "Run Now" on a type with custom instruction, see result
7. Manual: create cron with agent type, verify it runs on schedule
8. Manual: open specialist chat, verify tool restrictions apply
9. Manual: check activity dashboard shows agent run history
