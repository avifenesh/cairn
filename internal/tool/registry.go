package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/avifenesh/cairn/internal/llm"
)

// Registry holds registered tools and provides mode-based filtering and execution.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds one or more tools to the registry. If a tool with the same name
// already exists, it is replaced.
func (r *Registry) Register(tools ...Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// ForMode returns all tools available in the given mode, sorted by name.
func (r *Registry) ForMode(mode Mode) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Tool
	for _, t := range r.tools {
		if hasMode(t.Modes(), mode) {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// ForLLM converts all tools available in the given mode to the LLM ToolDef format.
func (r *Registry) ForLLM(mode Mode) []llm.ToolDef {
	return r.ForLLMFiltered(mode, nil)
}

// ForLLMFiltered converts tools available in the given mode to LLM format,
// optionally filtering to only the named tools. When allowedTools is nil or
// empty, all mode-available tools are returned.
func (r *Registry) ForLLMFiltered(mode Mode, allowedTools []string) []llm.ToolDef {
	tools := r.ForMode(mode)

	if len(allowedTools) > 0 {
		allowed := make(map[string]bool, len(allowedTools))
		for _, name := range allowedTools {
			allowed[name] = true
		}
		var filtered []Tool
		for _, t := range tools {
			if allowed[t.Name()] {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	defs := make([]llm.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		}
	}
	return defs
}

// Execute runs a tool by name with the given arguments.
func (r *Registry) Execute(ctx *ToolContext, name string, args json.RawMessage) (*ToolResult, error) {
	t, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, args)
}

// All returns every registered tool, sorted by name.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

func hasMode(modes []Mode, target Mode) bool {
	for _, m := range modes {
		if m == target {
			return true
		}
	}
	return false
}
