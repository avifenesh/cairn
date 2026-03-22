package rules

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/avifenesh/cairn/internal/signal"
)

// Template is a pre-built rule pattern that users can instantiate with minimal input.
type Template struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`         // "signal", "task", "memory", "scheduled"
	Source      string          `json:"source,omitempty"` // source this applies to ("" = any)
	Params      []TemplateParam `json:"params"`
	factory     func(params map[string]string) (*Rule, error)
}

// TemplateParam defines a user-fillable parameter for a template.
type TemplateParam struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // "text", "select", "number"
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"` // for "select" type
}

// bundledTemplates is the authoritative list of rule templates.
var bundledTemplates = []Template{
	// --- Signal Monitoring ---
	{
		ID:          "notify-github-pr",
		Name:        "Notify on GitHub PRs",
		Description: "Send a notification when a new GitHub pull request is detected.",
		Category:    "signal",
		Source:      "github",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Notify on GitHub PRs",
				Description: "Fires when a GitHub PR event is ingested.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": "github", "kind": "pr"},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "New PR: {{.title}} — {{.url}}", "priority": "2"},
				}},
				ThrottleMs: 60000,
			}, nil
		},
	},
	{
		ID:          "notify-github-issue",
		Name:        "Notify on GitHub Issues",
		Description: "Send a notification when a new GitHub issue is detected.",
		Category:    "signal",
		Source:      "github",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Notify on GitHub Issues",
				Description: "Fires when a GitHub issue event is ingested.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": "github", "kind": "issue"},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "New issue: {{.title}} — {{.url}}", "priority": "1"},
				}},
				ThrottleMs: 60000,
			}, nil
		},
	},
	{
		ID:          "notify-github-release",
		Name:        "Notify on New Releases",
		Description: "Send a notification when a new GitHub release is published.",
		Category:    "signal",
		Source:      "github",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Notify on New Releases",
				Description: "Fires when a GitHub release event is ingested.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": "github", "kind": "release"},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "New release: {{.title}} — {{.url}}", "priority": "2"},
				}},
				ThrottleMs: 300000,
			}, nil
		},
	},
	{
		ID:          "notify-hn-mentions",
		Name:        "Notify on HN Stories",
		Description: "Send a notification when a Hacker News story matches a keyword.",
		Category:    "signal",
		Source:      "hn",
		Params: []TemplateParam{{
			Key:   "keyword",
			Label: "Keyword (optional)",
			Type:  "text",
		}},
		factory: func(params map[string]string) (*Rule, error) {
			r := &Rule{
				Name:        "Notify on HN Stories",
				Description: "Fires when a Hacker News story is ingested.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": "hn"},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "HN: {{.title}} — {{.url}}", "priority": "1"},
				}},
				ThrottleMs: 60000,
			}
			if kw := strings.TrimSpace(params["keyword"]); kw != "" {
				r.Condition = fmt.Sprintf("title contains %q", kw)
				r.Name = fmt.Sprintf("Notify on HN: %s", kw)
			}
			return r, nil
		},
	},
	{
		ID:          "notify-reddit-post",
		Name:        "Notify on Reddit Posts",
		Description: "Send a notification when a new Reddit post is detected.",
		Category:    "signal",
		Source:      "reddit",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Notify on Reddit Posts",
				Description: "Fires when a Reddit post is ingested.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": "reddit"},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "Reddit: {{.title}} — {{.url}}", "priority": "1"},
				}},
				ThrottleMs: 60000,
			}, nil
		},
	},
	{
		ID:          "notify-email",
		Name:        "Notify on New Emails",
		Description: "Send a notification when a new email is received via Gmail.",
		Category:    "signal",
		Source:      "gmail",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Notify on New Emails",
				Description: "Fires when a Gmail email event is ingested.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": "gmail"},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "New email: {{.title}}", "priority": "2"},
				}},
				ThrottleMs: 30000,
			}, nil
		},
	},
	{
		ID:          "notify-any-signal",
		Name:        "Notify on Any Signal",
		Description: "Send a notification when any event arrives from a chosen source.",
		Category:    "signal",
		Params: []TemplateParam{{
			Key:      "source",
			Label:    "Source",
			Type:     "select",
			Required: true,
			Options:  allSourceNames(),
		}},
		factory: func(params map[string]string) (*Rule, error) {
			source := strings.TrimSpace(params["source"])
			if source == "" {
				return nil, fmt.Errorf("source parameter is required")
			}
			return &Rule{
				Name:        fmt.Sprintf("Notify on %s events", source),
				Description: fmt.Sprintf("Fires when any event from %s is ingested.", source),
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "EventIngested",
					Filter:    map[string]string{"sourceType": source},
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": fmt.Sprintf("[%s] {{.title}} — {{.url}}", source), "priority": "1"},
				}},
				ThrottleMs: 60000,
			}, nil
		},
	},

	// --- Task Automation ---
	{
		ID:          "alert-task-failure",
		Name:        "Alert on Task Failures",
		Description: "Send an urgent notification when any task fails.",
		Category:    "task",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Alert on Task Failures",
				Description: "Fires when a task fails.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "TaskFailed",
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "Task failed: {{.error}} (task {{.taskId}})", "priority": "3"},
				}},
				ThrottleMs: 10000,
			}, nil
		},
	},
	{
		ID:          "log-task-completed",
		Name:        "Log Completed Tasks",
		Description: "Send a notification when a task completes successfully.",
		Category:    "task",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Log Completed Tasks",
				Description: "Fires when a task completes.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "TaskCompleted",
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "Task completed: {{.taskId}}", "priority": "1"},
				}},
				ThrottleMs: 5000,
			}, nil
		},
	},

	// --- Memory Management ---
	{
		ID:          "review-memory-proposal",
		Name:        "Notify on Memory Proposals",
		Description: "Send a notification when the agent proposes a new memory for review.",
		Category:    "memory",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Notify on Memory Proposals",
				Description: "Fires when a memory is proposed.",
				Enabled:     true,
				Trigger: Trigger{
					Type:      TriggerEvent,
					EventType: "MemoryProposed",
				},
				Actions: []Action{{
					Type:   ActionNotify,
					Params: map[string]string{"message": "New memory proposed: {{.content}}", "priority": "1"},
				}},
				ThrottleMs: 10000,
			}, nil
		},
	},

	// --- Scheduled ---
	{
		ID:          "daily-digest-reminder",
		Name:        "Daily Digest Reminder",
		Description: "Create a task to build the daily digest at a specified hour.",
		Category:    "scheduled",
		Params: []TemplateParam{{
			Key:     "hour",
			Label:   "Hour (0-23)",
			Type:    "number",
			Default: "9",
		}},
		factory: func(params map[string]string) (*Rule, error) {
			hour := params["hour"]
			if hour == "" {
				hour = "9"
			}
			h, err := strconv.Atoi(hour)
			if err != nil || h < 0 || h > 23 {
				return nil, fmt.Errorf("hour must be an integer between 0 and 23, got %q", hour)
			}
			return &Rule{
				Name:        "Daily Digest Reminder",
				Description: "Submits a digest task daily.",
				Enabled:     true,
				Trigger: Trigger{
					Type:     TriggerCron,
					Schedule: fmt.Sprintf("0 %s * * *", hour),
				},
				Actions: []Action{{
					Type:   ActionTask,
					Params: map[string]string{"description": "Build and send daily digest", "type": "digest", "priority": "2"},
				}},
			}, nil
		},
	},
	{
		ID:          "weekly-report",
		Name:        "Weekly Summary Report",
		Description: "Create a task to generate a weekly summary every Monday morning.",
		Category:    "scheduled",
		factory: func(_ map[string]string) (*Rule, error) {
			return &Rule{
				Name:        "Weekly Summary Report",
				Description: "Submits a weekly report task every Monday at 9 AM.",
				Enabled:     true,
				Trigger: Trigger{
					Type:     TriggerCron,
					Schedule: "0 9 * * 1",
				},
				Actions: []Action{{
					Type:   ActionTask,
					Params: map[string]string{"description": "Generate weekly summary report", "type": "report", "priority": "2"},
				}},
			}, nil
		},
	},
}

// allSourceNames returns source names from the signal registry for template options.
func allSourceNames() []string {
	sources := signal.AllSourceInfo()
	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, s.Name)
	}
	return names
}

// templateIndex builds a lookup map on first access (thread-safe via sync.Once).
var (
	templateIndex map[string]*Template
	indexOnce     sync.Once
)

func ensureIndex() {
	indexOnce.Do(func() {
		templateIndex = make(map[string]*Template, len(bundledTemplates))
		for i := range bundledTemplates {
			templateIndex[bundledTemplates[i].ID] = &bundledTemplates[i]
		}
	})
}

// ListTemplates returns all bundled templates sorted by category then name.
func ListTemplates() []Template {
	ensureIndex()
	result := make([]Template, len(bundledTemplates))
	copy(result, bundledTemplates)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// ListTemplatesForSource returns templates that match a specific source (or have no source restriction).
func ListTemplatesForSource(source string) []Template {
	ensureIndex()
	var result []Template
	for _, t := range bundledTemplates {
		if t.Source == "" || t.Source == source {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// GetTemplate returns a template by ID, or nil if not found.
func GetTemplate(id string) *Template {
	ensureIndex()
	return templateIndex[id]
}

// Instantiate creates a Rule from a template with the given parameters.
// The returned Rule has no ID — the caller should save it to the store.
func Instantiate(id string, params map[string]string) (*Rule, error) {
	ensureIndex()
	tmpl := templateIndex[id]
	if tmpl == nil {
		return nil, fmt.Errorf("template %q not found", id)
	}

	// Validate required params.
	for _, p := range tmpl.Params {
		if p.Required {
			v := strings.TrimSpace(params[p.Key])
			if v == "" {
				// Try default.
				if p.Default != "" {
					if params == nil {
						params = make(map[string]string)
					}
					params[p.Key] = p.Default
				} else {
					return nil, fmt.Errorf("required parameter %q is missing", p.Key)
				}
			}
		}
	}

	// Apply defaults for non-required params.
	if params == nil {
		params = make(map[string]string)
	}
	for _, p := range tmpl.Params {
		if _, ok := params[p.Key]; !ok && p.Default != "" {
			params[p.Key] = p.Default
		}
	}

	return tmpl.factory(params)
}
