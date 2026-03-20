---
name: mermaid-diagram
description: "Generate Mermaid diagrams from natural language. Use when asked to create a diagram, flowchart, sequence diagram, architecture diagram, ER diagram, state diagram, class diagram, mindmap, gantt chart, or any visual representation. Keywords: diagram, flowchart, sequence, architecture, mermaid, visualize, chart, graph, mindmap, gantt, ER, class diagram, state diagram, pie chart"
argument-hint: "<description of what to diagram>"
allowed-tools: "cairn.createArtifact"
inclusion: on-demand
---

# Mermaid Diagram Generator

Generate Mermaid diagrams from natural language descriptions and save them as `diagram` artifacts.

## Supported Diagram Types

| Type | Directive | Use For |
|------|-----------|---------|
| Flowchart | `flowchart TD` | Processes, decision trees, workflows |
| Sequence | `sequenceDiagram` | API calls, message flows, interactions |
| Class | `classDiagram` | Object models, type hierarchies |
| State | `stateDiagram-v2` | State machines, lifecycle flows |
| ER | `erDiagram` | Database schemas, entity relationships |
| Gantt | `gantt` | Timelines, project schedules |
| Pie | `pie` | Proportional breakdowns |
| Mindmap | `mindmap` | Brainstorms, topic hierarchies |

## Steps

1. **Identify diagram type** from the user's request. Default to `flowchart TD` if unclear.

2. **Write Mermaid syntax** following these rules:
   - Use descriptive node IDs (`userLogin` not `A`)
   - Keep labels concise (under 40 chars)
   - Use appropriate arrow styles (`-->` for flow, `-.->` for optional, `==>` for emphasis)
   - Group related nodes with `subgraph` when there are 6+ nodes
   - Limit diagrams to 20 nodes maximum for readability

3. **Validate syntax mentally** before saving:
   - No unmatched brackets or quotes
   - Node IDs contain only alphanumeric chars and underscores
   - Labels with special characters are wrapped in quotes
   - Each `subgraph` has a matching `end`

4. **Save as artifact** by calling `cairn.createArtifact`:

```json
{
  "type": "diagram",
  "title": "descriptive title of the diagram",
  "contentJson": {
    "title": "Diagram Title",
    "description": "Brief explanation of what the diagram shows",
    "code": "flowchart TD\n  start[Start] --> finish[End]",
    "diagramType": "flowchart"
  }
}
```

The `contentJson` fields:
- `title` (string) -- heading for the rendered output
- `description` (string, optional) -- paragraph explaining the diagram
- `code` (string, required) -- raw Mermaid syntax
- `diagramType` (string) -- one of: flowchart, sequence, class, state, er, gantt, pie, mindmap

## Quick Syntax Reference

**Flowchart:**
```
flowchart TD
  start[Start] --> check{Valid?}
  check -->|Yes| process[Process]
  check -->|No| error[Error]
  process --> done[Done]
```

**Sequence:**
```
sequenceDiagram
  Client->>+Server: POST /login
  Server->>DB: SELECT user
  DB-->>Server: user row
  Server-->>-Client: 200 token
```

**State:**
```
stateDiagram-v2
  [*] --> Idle
  Idle --> Running: start
  Running --> Idle: stop
  Running --> Error: fail
  Error --> Idle: reset
```

**ER:**
```
erDiagram
  USER ||--o{ ORDER : places
  ORDER ||--|{ LINE_ITEM : contains
  PRODUCT ||--o{ LINE_ITEM : "ordered in"
```

**Mindmap:**
```
mindmap
  root((Topic))
    Branch A
      Leaf 1
      Leaf 2
    Branch B
      Leaf 3
```

## Notes

- Always prefer `flowchart` over the deprecated `graph` directive
- Use `TD` (top-down) by default; use `LR` (left-right) for wide/shallow graphs
- For sequence diagrams, use `+`/`-` activation markers to show call depth
- If the user asks to edit an existing diagram, create a new artifact with the updated code
- The rendered output is Markdown with a fenced mermaid code block, viewable in any Mermaid-compatible renderer
