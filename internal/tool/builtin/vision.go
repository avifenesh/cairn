package builtin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/avifenesh/cairn/internal/tool"
)

// visionProcess manages a long-lived Z.ai Vision MCP subprocess (stdio transport).
var visionProc struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
	alive  bool
}

// callVisionMCP sends a tool call to the Vision MCP subprocess.
// It lazily spawns the process on first use and respawns on errors.
func callVisionMCP(ctx context.Context, toolName string, args map[string]any) (string, error) {
	if !VisionEnabled() {
		return "", fmt.Errorf("vision: not configured")
	}

	visionProc.mu.Lock()
	defer visionProc.mu.Unlock()

	if err := ensureVisionRunning(); err != nil {
		return "", err
	}

	text, err := visionCallTool(ctx, toolName, args)
	if err != nil {
		visionCleanup()
		return "", err
	}
	return text, nil
}

// CloseVision shuts down the vision subprocess. Call on server shutdown.
func CloseVision() {
	visionProc.mu.Lock()
	defer visionProc.mu.Unlock()
	visionCleanup()
}

func ensureVisionRunning() error {
	if visionProc.alive && visionProc.cmd != nil && visionProc.cmd.Process != nil {
		if err := visionProc.cmd.Process.Signal(syscall.Signal(0)); err == nil {
			return nil
		}
		visionCleanup()
	}
	return visionSpawn()
}

func visionSpawn() error {
	cmd := exec.Command(visionConfig.npxPath, "-y", "@z_ai/mcp-server")
	cmd.Env = append(os.Environ(),
		"Z_AI_API_KEY="+visionConfig.apiKey,
		"Z_AI_MODE=ZAI",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("vision: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("vision: stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("vision: start process: %w", err)
	}

	visionProc.cmd = cmd
	visionProc.stdin = stdin
	visionProc.stdout = bufio.NewReaderSize(stdout, 256*1024)
	visionProc.nextID = 1
	visionProc.alive = true

	if err := visionInitialize(); err != nil {
		visionCleanup()
		return fmt.Errorf("vision: handshake failed: %w", err)
	}

	return nil
}

func visionInitialize() error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      visionProc.nextID,
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "cairn", "version": "0.1.0"},
		},
	}
	visionProc.nextID++

	resp, err := visionRoundTrip(req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	// Send initialized notification (MCP protocol requirement, no ID field).
	notif := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}{JSONRPC: "2.0", Method: "notifications/initialized"}
	return visionWriteJSON(notif)
}

func visionCallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      visionProc.nextID,
		Params:  map[string]any{"name": toolName, "arguments": args},
	}
	visionProc.nextID++

	if err := visionWriteJSON(req); err != nil {
		return "", err
	}

	// Read with context cancellation support.
	type readResult struct {
		resp *jsonRPCResponse
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		resp, err := visionReadResponse()
		ch <- readResult{resp, err}
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("vision: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		if r.resp.Error != nil {
			return "", fmt.Errorf("vision: RPC error %d: %s", r.resp.Error.Code, r.resp.Error.Message)
		}
		var mcpCheck struct {
			IsError bool `json:"isError"`
		}
		if json.Unmarshal(r.resp.Result, &mcpCheck) == nil && mcpCheck.IsError {
			return "", fmt.Errorf("vision: %s", extractMCPText(r.resp.Result))
		}
		return extractMCPText(r.resp.Result), nil
	}
}

func visionWriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("vision: marshal: %w", err)
	}
	data = append(data, '\n')
	_, err = visionProc.stdin.Write(data)
	return err
}

func visionReadResponse() (*jsonRPCResponse, error) {
	for {
		line, err := visionProc.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("vision: read stdout: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue // skip non-JSON lines (npm logs, etc.)
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // skip malformed lines
		}
		return &resp, nil
	}
}

func visionRoundTrip(req jsonRPCRequest) (*jsonRPCResponse, error) {
	if err := visionWriteJSON(req); err != nil {
		return nil, err
	}
	return visionReadResponse()
}

func visionCleanup() {
	visionProc.alive = false
	if visionProc.stdin != nil {
		visionProc.stdin.Close()
	}
	if visionProc.cmd != nil && visionProc.cmd.Process != nil {
		_ = syscall.Kill(-visionProc.cmd.Process.Pid, syscall.SIGKILL)
		_ = visionProc.cmd.Wait()
	}
	visionProc.cmd = nil
	visionProc.stdin = nil
	visionProc.stdout = nil
}

// --- Vision tool parameter structs ---
// All tools use "image_source" (local path or URL) + "prompt" (required).
// Verified via tools/list on @z_ai/mcp-server v0.1.2.

type visionImageParams struct {
	ImageSource string `json:"image_source" desc:"Local file path or URL of the image"`
	Prompt      string `json:"prompt" desc:"What to analyze or extract from the image"`
}

type visionUIToArtifactParams struct {
	ImageSource string `json:"image_source" desc:"Local file path or URL of the UI screenshot"`
	OutputType  string `json:"output_type" desc:"Type of output: code, prompt, spec, or description"`
	Prompt      string `json:"prompt" desc:"Instructions for what to generate from this UI"`
}

type visionDiffParams struct {
	ExpectedImageSource string `json:"expected_image_source" desc:"Local path or URL of the expected/reference UI"`
	ActualImageSource   string `json:"actual_image_source" desc:"Local path or URL of the actual UI"`
	Prompt              string `json:"prompt" desc:"What aspects to compare"`
}

type visionVideoParams struct {
	VideoSource string `json:"video_source" desc:"Local file path or URL of the video (MP4/MOV/M4V, max 8MB)"`
	Prompt      string `json:"prompt" desc:"What to analyze or extract from the video"`
}

// --- Vision tool definitions ---
// Tool names and param schemas verified via tools/list on the actual MCP server.

func visionTool(name, desc, mcpTool string) tool.Tool {
	return tool.Define(name, desc,
		[]tool.Mode{tool.ModeTalk, tool.ModeWork},
		func(ctx *tool.ToolContext, p visionImageParams) (*tool.ToolResult, error) {
			if p.ImageSource == "" {
				return &tool.ToolResult{Error: "image_source is required (local path or URL)"}, nil
			}
			if p.Prompt == "" {
				return &tool.ToolResult{Error: "prompt is required"}, nil
			}
			args := map[string]any{"image_source": p.ImageSource, "prompt": p.Prompt}
			text, err := callVisionMCP(safeCtx(ctx.Cancel), mcpTool, args)
			if err != nil {
				return &tool.ToolResult{Error: fmt.Sprintf("%s failed: %v", name, err)}, nil
			}
			return &tool.ToolResult{Output: text, Metadata: map[string]any{"provider": "zai-vision"}}, nil
		},
	)
}

var (
	visionImageAnalysis  = visionTool("cairn.imageAnalysis", "Analyze an image using Z.ai Vision (general understanding, Q&A).", "analyze_image")
	visionExtractText    = visionTool("cairn.extractText", "Extract text from a screenshot (OCR for code, terminals, docs) using Z.ai Vision.", "extract_text_from_screenshot")
	visionDiagnoseError  = visionTool("cairn.diagnoseError", "Diagnose an error from a screenshot with fix recommendations using Z.ai Vision.", "diagnose_error_screenshot")
	visionAnalyzeDiagram = visionTool("cairn.analyzeDiagram", "Understand a technical diagram (architecture, flow, UML) using Z.ai Vision.", "understand_technical_diagram")
	visionAnalyzeChart   = visionTool("cairn.analyzeChart", "Analyze a data visualization (chart, dashboard, graph) using Z.ai Vision.", "analyze_data_visualization")
)

var visionUIToArtifact = tool.Define("cairn.uiToArtifact",
	"Convert a UI screenshot into code, prompts, specs, or descriptions using Z.ai Vision.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p visionUIToArtifactParams) (*tool.ToolResult, error) {
		if p.ImageSource == "" {
			return &tool.ToolResult{Error: "image_source is required"}, nil
		}
		if p.OutputType == "" {
			p.OutputType = "code"
		}
		if p.Prompt == "" {
			return &tool.ToolResult{Error: "prompt is required"}, nil
		}
		args := map[string]any{
			"image_source": p.ImageSource,
			"output_type":  p.OutputType,
			"prompt":       p.Prompt,
		}
		text, err := callVisionMCP(safeCtx(ctx.Cancel), "ui_to_artifact", args)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("ui to artifact failed: %v", err)}, nil
		}
		return &tool.ToolResult{Output: text, Metadata: map[string]any{"provider": "zai-vision"}}, nil
	},
)

var visionUIDiffCheck = tool.Define("cairn.uiDiffCheck",
	"Compare two UI screenshots and identify visual differences using Z.ai Vision.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p visionDiffParams) (*tool.ToolResult, error) {
		if p.ExpectedImageSource == "" || p.ActualImageSource == "" {
			return &tool.ToolResult{Error: "both expected_image_source and actual_image_source are required"}, nil
		}
		if p.Prompt == "" {
			return &tool.ToolResult{Error: "prompt is required"}, nil
		}
		args := map[string]any{
			"expected_image_source": p.ExpectedImageSource,
			"actual_image_source":   p.ActualImageSource,
			"prompt":                p.Prompt,
		}
		text, err := callVisionMCP(safeCtx(ctx.Cancel), "ui_diff_check", args)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("ui diff check failed: %v", err)}, nil
		}
		return &tool.ToolResult{Output: text, Metadata: map[string]any{"provider": "zai-vision"}}, nil
	},
)

var visionVideoAnalysis = tool.Define("cairn.videoAnalysis",
	"Analyze a video file (MP4/MOV/M4V, max 8MB) using Z.ai Vision.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p visionVideoParams) (*tool.ToolResult, error) {
		if p.VideoSource == "" {
			return &tool.ToolResult{Error: "video_source is required (local path or URL)"}, nil
		}
		if p.Prompt == "" {
			return &tool.ToolResult{Error: "prompt is required"}, nil
		}
		args := map[string]any{"video_source": p.VideoSource, "prompt": p.Prompt}
		text, err := callVisionMCP(safeCtx(ctx.Cancel), "video_analysis", args)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("video analysis failed: %v", err)}, nil
		}
		return &tool.ToolResult{Output: text, Metadata: map[string]any{"provider": "zai-vision"}}, nil
	},
)
