package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

type mockSkillService struct {
	skills map[string]*tool.SkillItem
}

func newMockSkillService() *mockSkillService {
	return &mockSkillService{skills: map[string]*tool.SkillItem{
		"web-search": {
			Name:        "web-search",
			Description: "Search the web",
			Inclusion:   "on-demand",
			Content:     "# Web Search\n\nSearch and summarize.",
		},
		"code-review": {
			Name:        "code-review",
			Description: "Review code for bugs",
			Inclusion:   "on-demand",
			Content:     "# Code Review\n\nCheck for bugs.",
		},
	}}
}

func (m *mockSkillService) Get(name string) *tool.SkillItem {
	return m.skills[name]
}

func (m *mockSkillService) List() []*tool.SkillItem {
	out := make([]*tool.SkillItem, 0, len(m.skills))
	for _, sk := range m.skills {
		out = append(out, sk)
	}
	return out
}

func (m *mockSkillService) Create(name, description, content, inclusion string, allowedTools []string) error {
	m.skills[name] = &tool.SkillItem{Name: name, Description: description, Content: content, Inclusion: inclusion, AllowedTools: allowedTools}
	return nil
}

func (m *mockSkillService) Update(name, description, content, inclusion string, allowedTools []string) error {
	if _, ok := m.skills[name]; !ok {
		return fmt.Errorf("not found")
	}
	m.skills[name] = &tool.SkillItem{Name: name, Description: description, Content: content, Inclusion: inclusion, AllowedTools: allowedTools}
	return nil
}

func (m *mockSkillService) Delete(name string) error {
	if _, ok := m.skills[name]; !ok {
		return fmt.Errorf("not found")
	}
	delete(m.skills, name)
	return nil
}

func (m *mockSkillService) InstallDir() string { return "/tmp/test-skills" }
func (m *mockSkillService) Refresh() error     { return nil }

func toolCtxWithSkills(svc tool.SkillService) *tool.ToolContext {
	return &tool.ToolContext{
		SessionID: "test",
		AgentMode: tool.ModeTalk,
		Cancel:    context.Background(),
		Skills:    svc,
	}
}

func TestLoadSkill(t *testing.T) {
	ctx := toolCtxWithSkills(newMockSkillService())
	args, _ := json.Marshal(map[string]string{"name": "web-search"})

	result, err := loadSkill.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["name"].(string) != "web-search" {
		t.Fatalf("expected web-search, got %v", result.Metadata["name"])
	}
	if result.Output == "" {
		t.Fatal("expected skill content in output")
	}
}

func TestLoadSkillActivatesCallback(t *testing.T) {
	ctx := toolCtxWithSkills(newMockSkillService())
	var activated string
	ctx.ActivateSkill = func(name, content string, allowedTools []string) {
		activated = name
	}

	args, _ := json.Marshal(map[string]string{"name": "web-search"})
	result, err := loadSkill.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if activated != "web-search" {
		t.Fatalf("expected activation callback with 'web-search', got %q", activated)
	}
	if result.Metadata["activated"] != true {
		t.Fatal("expected activated=true in metadata")
	}
}

func TestLoadSkillOutputFormat(t *testing.T) {
	ctx := toolCtxWithSkills(newMockSkillService())
	args, _ := json.Marshal(map[string]string{"name": "web-search"})

	result, err := loadSkill.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "<skill_content") {
		t.Fatal("expected <skill_content> tag in output")
	}
	if !strings.Contains(result.Output, "</skill_content>") {
		t.Fatal("expected closing </skill_content> tag")
	}
}

func TestLoadSkillNotFound(t *testing.T) {
	ctx := toolCtxWithSkills(newMockSkillService())
	args, _ := json.Marshal(map[string]string{"name": "nonexistent"})

	result, err := loadSkill.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for missing skill")
	}
}

func TestLoadSkillNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"name": "test"})

	result, err := loadSkill.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}

func TestListSkills(t *testing.T) {
	ctx := toolCtxWithSkills(newMockSkillService())

	result, err := listSkills.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["count"].(int) != 2 {
		t.Fatalf("expected 2 skills, got %v", result.Metadata["count"])
	}
}

func TestListSkillsEmpty(t *testing.T) {
	ctx := toolCtxWithSkills(&mockSkillService{skills: map[string]*tool.SkillItem{}})

	result, err := listSkills.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "No skills available." {
		t.Fatalf("expected empty message, got: %s", result.Output)
	}
}

func TestListSkillsNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}

	result, err := listSkills.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}
