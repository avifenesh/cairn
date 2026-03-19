package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

type mockConfigService struct {
	config map[string]any
}

func (m *mockConfigService) PatchConfig(_ context.Context, changes map[string]any) (map[string]any, error) {
	for k, v := range changes {
		m.config[k] = v
	}
	return m.config, nil
}

func (m *mockConfigService) GetConfig(_ context.Context) (map[string]any, error) {
	return m.config, nil
}

func TestGetConfig(t *testing.T) {
	svc := &mockConfigService{config: map[string]any{"ghOwner": "avi", "rssEnabled": true}}
	ctx := &tool.ToolContext{Cancel: context.Background(), Config: svc}

	result, err := getConfig.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestPatchConfig(t *testing.T) {
	svc := &mockConfigService{config: map[string]any{"ghOwner": "old"}}
	ctx := &tool.ToolContext{Cancel: context.Background(), Config: svc}

	changes := map[string]any{"ghOwner": "new", "rssEnabled": true}
	data, _ := json.Marshal(changes)
	args, _ := json.Marshal(map[string]string{"changes": string(data)})

	result, err := patchConfig.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}
	if svc.config["ghOwner"] != "new" {
		t.Errorf("expected ghOwner=new, got %v", svc.config["ghOwner"])
	}
}

func TestPatchConfigInvalidJSON(t *testing.T) {
	svc := &mockConfigService{config: map[string]any{}}
	ctx := &tool.ToolContext{Cancel: context.Background(), Config: svc}

	args, _ := json.Marshal(map[string]string{"changes": "not json"})
	result, err := patchConfig.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPatchConfigNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"changes": `{"foo":"bar"}`})
	result, err := patchConfig.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}
