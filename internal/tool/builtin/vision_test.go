package builtin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVisionEnabled(t *testing.T) {
	// Save and restore state.
	origKey := visionConfig.apiKey
	origPath := visionConfig.npxPath
	origEnabled := visionConfig.enabled.Load()
	defer func() {
		visionConfig.apiKey = origKey
		visionConfig.npxPath = origPath
		visionConfig.enabled.Store(origEnabled)
	}()

	SetVisionConfig("", "")
	if VisionEnabled() {
		t.Error("expected disabled when no key/path")
	}

	SetVisionConfig("test-key", "/usr/bin/npx")
	if !VisionEnabled() {
		t.Error("expected enabled when key and path set")
	}

	SetVisionConfig("test-key", "")
	if VisionEnabled() {
		t.Error("expected disabled when no npx path")
	}
}

func TestVisionToolCount(t *testing.T) {
	// Save and restore Z.ai and vision state.
	origKey := zaiConfig.APIKey
	origEnabled := zaiConfig.enabled.Load()
	origVKey := visionConfig.apiKey
	origVPath := visionConfig.npxPath
	origVEnabled := visionConfig.enabled.Load()
	defer func() {
		zaiConfig.APIKey = origKey
		zaiConfig.enabled.Store(origEnabled)
		visionConfig.apiKey = origVKey
		visionConfig.npxPath = origVPath
		visionConfig.enabled.Store(origVEnabled)
	}()

	// Z.ai enabled, vision disabled: 35 tools (30 base + 5 Z.ai HTTP).
	SetZaiConfig("test-key", "https://api.z.ai/api/mcp")
	SetVisionConfig("", "")
	tools := All()
	if len(tools) != 41 {
		t.Errorf("expected 41 tools (zai without vision), got %d", len(tools))
	}

	// Z.ai enabled, vision enabled: 43 tools (30 base + 5 Z.ai HTTP + 8 vision).
	SetVisionConfig("test-key", "/usr/bin/npx")
	tools = All()
	if len(tools) != 49 {
		t.Errorf("expected 49 tools (zai with vision), got %d", len(tools))
	}
}

func TestVisionToolNames(t *testing.T) {
	origKey := zaiConfig.APIKey
	origEnabled := zaiConfig.enabled.Load()
	origVKey := visionConfig.apiKey
	origVPath := visionConfig.npxPath
	origVEnabled := visionConfig.enabled.Load()
	defer func() {
		zaiConfig.APIKey = origKey
		zaiConfig.enabled.Store(origEnabled)
		visionConfig.apiKey = origVKey
		visionConfig.npxPath = origVPath
		visionConfig.enabled.Store(origVEnabled)
	}()

	SetZaiConfig("test-key", "https://api.z.ai/api/mcp")
	SetVisionConfig("test-key", "/usr/bin/npx")

	tools := All()
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		names[t.Name()] = true
	}

	expected := []string{
		"cairn.imageAnalysis", "cairn.extractText", "cairn.diagnoseError",
		"cairn.analyzeDiagram", "cairn.analyzeChart", "cairn.uiToArtifact",
		"cairn.uiDiffCheck", "cairn.videoAnalysis",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected tool %q not found in All()", name)
		}
	}
}

func TestCallVisionMCP_MockProcess(t *testing.T) {
	// Create a mock script that acts as a fake MCP server on stdio.
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-mcp.sh")
	err := os.WriteFile(script, []byte(`#!/bin/bash
# Read initialize request
read -r line
# Respond with initialize result
echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{"listChanged":true}},"serverInfo":{"name":"mock","version":"0.1.0"}}}'
# Read initialized notification
read -r line
# Read tools/call request
read -r line
# Respond with tool result
echo '{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"Mock analysis: image contains a cat."}],"isError":false}}'
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Save and restore state.
	origKey := visionConfig.apiKey
	origPath := visionConfig.npxPath
	origEnabled := visionConfig.enabled.Load()
	defer func() {
		CloseVision()
		visionConfig.apiKey = origKey
		visionConfig.npxPath = origPath
		visionConfig.enabled.Store(origEnabled)
	}()

	// Point vision to the mock script instead of npx.
	// We override the spawn to use bash directly.
	visionConfig.apiKey = "test-key"
	visionConfig.npxPath = "/bin/bash"
	visionConfig.enabled.Store(true)

	// Override: we need to call the script, not npx.
	// Easiest: temporarily swap npxPath to bash and pass script as arg.
	// But callVisionMCP uses visionConfig.npxPath with args "-y", "@z_ai/mcp-server".
	// We need a wrapper script that ignores args and runs the mock.
	wrapper := filepath.Join(dir, "npx-mock")
	err = os.WriteFile(wrapper, []byte("#!/bin/bash\nexec "+script+"\n"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	visionConfig.npxPath = wrapper

	text, err := callVisionMCP(t.Context(), "analyze_image", map[string]any{"image_source": "/tmp/test.png", "prompt": "What is in this image?"})
	if err != nil {
		t.Fatalf("callVisionMCP failed: %v", err)
	}
	if text != "Mock analysis: image contains a cat." {
		t.Errorf("unexpected result: %q", text)
	}
}
