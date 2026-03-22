package rules

import (
	"testing"

	"github.com/expr-lang/expr"
)

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	if len(templates) == 0 {
		t.Fatal("ListTemplates returned empty")
	}
	if len(templates) != len(bundledTemplates) {
		t.Errorf("got %d templates, want %d", len(templates), len(bundledTemplates))
	}

	// Verify sorted by category.
	for i := 1; i < len(templates); i++ {
		if templates[i].Category < templates[i-1].Category {
			t.Errorf("not sorted by category: %q before %q", templates[i-1].Category, templates[i].Category)
		}
	}
}

func TestListTemplatesForSource(t *testing.T) {
	github := ListTemplatesForSource("github")
	if len(github) == 0 {
		t.Fatal("no templates for github")
	}
	for _, tmpl := range github {
		if tmpl.Source != "" && tmpl.Source != "github" {
			t.Errorf("template %q has source %q, want github or empty", tmpl.ID, tmpl.Source)
		}
	}

	// Source-less templates (task, memory, scheduled) should always appear.
	all := ListTemplatesForSource("nonexistent")
	for _, tmpl := range all {
		if tmpl.Source != "" {
			t.Errorf("template %q has source %q, should not appear for nonexistent", tmpl.ID, tmpl.Source)
		}
	}
}

func TestGetTemplate(t *testing.T) {
	tmpl := GetTemplate("notify-github-pr")
	if tmpl == nil {
		t.Fatal("GetTemplate(notify-github-pr) returned nil")
	}
	if tmpl.Name != "Notify on GitHub PRs" {
		t.Errorf("name = %q", tmpl.Name)
	}

	if GetTemplate("nonexistent") != nil {
		t.Error("GetTemplate(nonexistent) should return nil")
	}
}

func TestInstantiateAllTemplates(t *testing.T) {
	for _, tmpl := range bundledTemplates {
		t.Run(tmpl.ID, func(t *testing.T) {
			// Build params with defaults.
			params := make(map[string]string)
			for _, p := range tmpl.Params {
				if p.Required && p.Default == "" {
					// Use first option for select, or a placeholder for text.
					if len(p.Options) > 0 {
						params[p.Key] = p.Options[0]
					} else {
						params[p.Key] = "test-value"
					}
				}
			}

			rule, err := Instantiate(tmpl.ID, params)
			if err != nil {
				t.Fatalf("Instantiate failed: %v", err)
			}
			if rule == nil {
				t.Fatal("Instantiate returned nil rule")
			}
			if rule.Name == "" {
				t.Error("rule has empty name")
			}
			if rule.Trigger.Type == "" {
				t.Error("rule has empty trigger type")
			}
			if len(rule.Actions) == 0 {
				t.Error("rule has no actions")
			}

			// Verify condition compiles with expr-lang if present.
			if rule.Condition != "" {
				_, err := expr.Compile(rule.Condition, expr.AsBool())
				if err != nil {
					t.Errorf("condition %q failed to compile: %v", rule.Condition, err)
				}
			}
		})
	}
}

func TestInstantiateNotFound(t *testing.T) {
	_, err := Instantiate("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestInstantiateMissingRequiredParam(t *testing.T) {
	_, err := Instantiate("notify-any-signal", nil)
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
}

func TestInstantiateWithKeyword(t *testing.T) {
	rule, err := Instantiate("notify-hn-mentions", map[string]string{"keyword": "Go"})
	if err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}
	if rule.Condition == "" {
		t.Error("expected condition with keyword")
	}
	// Verify condition compiles.
	_, err = expr.Compile(rule.Condition, expr.AsBool())
	if err != nil {
		t.Errorf("keyword condition %q failed to compile: %v", rule.Condition, err)
	}
}

func TestInstantiateWithoutKeyword(t *testing.T) {
	rule, err := Instantiate("notify-hn-mentions", nil)
	if err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}
	if rule.Condition != "" {
		t.Errorf("expected empty condition without keyword, got %q", rule.Condition)
	}
}

func TestTemplateIDsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, tmpl := range bundledTemplates {
		if seen[tmpl.ID] {
			t.Errorf("duplicate template ID: %q", tmpl.ID)
		}
		seen[tmpl.ID] = true
	}
}
